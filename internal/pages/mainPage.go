// Package pages, it implements main routing to different pages.
package pages

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/state"
	"github.com/ktails/ktails/internal/tui/cmds"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/ktails/ktails/internal/tui/views"
)

type focusTarget int

const (
	focusLeftPane focusTarget = iota
	focusTabs
)

type MainPage struct {
	// dimensions
	width  int
	height int

	// tabs
	tabs       []string
	tabContent string
	activeTab  int

	// App state
	appState       *state.AppState
	appStateLoaded bool

	// base models
	contextList      *models.ContextsInfo
	deploymentList   *models.DeploymentPage
	podList          *models.PodPage
	svcList          *models.ServicePage
	deploymentDetail *models.ResourceDetailPage
	focus            focusTarget

	// k8s client
	Client *k8s.Client

	// UI overlays
	errorMessage string
	showHelp     bool

	// Auto-refresh — a self-rescheduling tick that reruns refreshActiveTab on
	// an interval. Paused (tick still reschedules, but the refresh is skipped)
	// while the Detail or Log pane is open, since a background reorder under a
	// pinned pane is more disruptive than helpful.
	autoRefresh     bool
	refreshInterval time.Duration

	// Detail pane — a cross-cutting bottom split opened by Enter from
	// Deployments or Pods. It's not a peer tab: it stays put, splitting
	// whichever top tab's content area you're currently on.
	showDetail    bool
	detailFocused bool

	// Log pane — a second cross-cutting bottom split, reachable with `l` on
	// one or more checked Pods rows (or the row under the cursor, if none
	// are checked). Mutually exclusive with the Detail pane: opening one
	// closes the other. Every checked pod's containers become one source
	// each, merged into a single scrollback; logStreams holds the live
	// stream/scanner/generation per source, keyed the same way as
	// podLogs' sources. Generation guards against messages from a
	// since-superseded stream for that specific source (old pod switched
	// out, or the whole pane closed) without affecting other open sources.
	podLogs     *models.LogPage
	showLogs    bool
	logsFocused bool
	logStreams  map[string]*logStreamState

	tableW, tableH int
}

// logStreamState is the live stream-plumbing state for one open log
// source, keyed by the same context/namespace/pod/container key used in
// models.LogPage.
type logStreamState struct {
	stream     io.ReadCloser
	scanner    *bufio.Scanner
	generation int
}

// NewMainPageModel builds the top-level page model. refreshIntervalSeconds is
// config.Preferences.RefreshInterval — the auto-refresh tick period; values
// below 1 fall back to 5s (the same default as config.DefaultConfig).
func NewMainPageModel(c *k8s.Client, refreshIntervalSeconds int) *MainPage {
	ctxInfo := models.NewContextInfo(c)
	depList := models.NewDeploymentPage(c)
	pList := models.NewPodPageModel(c)
	svcList := models.NewServicePageModel(c)
	detailPage := models.NewResourceDetailPage()
	logPage := models.NewLogPage()
	tabs := styles.DefaultTabs
	tabs = append(tabs, "svc")

	if refreshIntervalSeconds < 1 {
		refreshIntervalSeconds = 5
	}

	m := &MainPage{
		Client:           c,
		appState:         state.NewAppState(),
		tabs:             tabs,
		tabContent:       "",
		contextList:      ctxInfo,
		deploymentList:   depList,
		podList:          pList,
		svcList:          svcList,
		deploymentDetail: detailPage,
		podLogs:          logPage,
		logStreams:       make(map[string]*logStreamState),
		appStateLoaded:   false,
		focus:            focusLeftPane,
		errorMessage:     "",
		showHelp:         false,
		autoRefresh:      true,
		refreshInterval:  time.Duration(refreshIntervalSeconds) * time.Second,
	}

	m.updateFocusStates()
	return m
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return m.refreshTickCmd()
}

// refreshTickCmd schedules the next RefreshTickMsg one refreshInterval from
// now — the standard bubbletea self-rescheduling tick pattern. Scheduled
// unconditionally, even while auto-refresh is paused or toggled off, so it's
// always running in the background and ready to pick refreshing back up.
func (m *MainPage) refreshTickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return msgs.RefreshTickMsg{}
	})
}

func (m *MainPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	defer m.logSlowUpdate(start)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		keypress := msg.String()

		// Help overlay is modal — only ? and esc pass through
		if m.showHelp {
			if keypress == "?" || keypress == "esc" {
				m.showHelp = false
			}
			return m, nil
		}

		// Global keys
		switch keypress {
		case "ctrl+c", "q":
			m.stopLogStream()
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "esc":
			// Peel dismissals one at a time: unfocus the detail/log pane, then
			// close it, then inline error, then context errors. Detail and Logs
			// are mutually exclusive, so only one of their branches is ever live.
			if m.detailFocused {
				m.detailFocused = false
				m.updateFocusStates()
			} else if m.logsFocused {
				m.logsFocused = false
				m.updateFocusStates()
			} else if m.showDetail {
				m.showDetail = false
				m.applyContentSizes()
			} else if m.showLogs {
				m.closeLogs()
				m.applyContentSizes()
			} else if m.errorMessage != "" {
				m.errorMessage = ""
			} else {
				m.appState.ClearErrors()
			}
			return m, nil
		case "?":
			m.showHelp = true
			return m, nil
		case "R":
			m.autoRefresh = !m.autoRefresh
			return m, nil
		}

		// Context list keys
		if m.focus == focusLeftPane {
			cmd := m.contextList.Update(msg)
			return m, cmd
		}

		// While the detail pane has keyboard focus, it captures everything
		// (arrows/j-k/pgup/pgdn/g/G) until Esc hands focus back to the list.
		if m.detailFocused {
			cmd := m.deploymentDetail.Update(msg)
			return m, cmd
		}

		// While the log pane has keyboard focus, it captures everything except
		// 'c', which MainPage intercepts to isolate/return-to-merged a single
		// source — a pure view toggle, no stream side effects.
		if m.logsFocused {
			if keypress == "c" {
				m.podLogs.CycleIsolation()
				return m, nil
			}
			cmd := m.podLogs.Update(msg)
			return m, cmd
		}

		// Ctrl+R always jumps straight back into an already-open pane — unlike
		// Enter, it never fetches, no matter where the list cursor now sits.
		if keypress == "ctrl+r" && m.showDetail {
			m.detailFocused = true
			m.applyContentSizes()
			m.updateFocusStates()
			return m, nil
		}

		// Tab navigation (tabs focused) — switching tabs while the detail pane
		// is open (but unfocused) is allowed; the pane is cross-cutting and
		// stays put beneath whichever tab you land on.
		switch keypress {
		case "right", "]":
			next := m.activeTab + 1
			if next >= len(m.tabs) {
				return m, nil
			}
			nextTab := m.tabs[next]
			if nextTab == "Deployments" || nextTab == "Pods" || nextTab == "svc" {
				snapshot := m.appState.Snapshot()
				if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
					return m, nil
				}
			}
			m.activeTab = next
			m.updateFocusStates()
			return m, nil
		case "left", "[":
			prev := m.activeTab - 1
			if prev < 0 {
				return m, nil
			}
			m.activeTab = prev
			m.updateFocusStates()
			return m, nil
		}

		// Enter on a selected Deployment/Pod/Service row (re)loads the detail
		// pane for that row and gives it keyboard focus for scrolling. Detail
		// and Logs share the same bottom slot and are mutually exclusive.
		if m.appStateLoaded && keypress == "enter" &&
			(m.tabs[m.activeTab] == "Deployments" || m.tabs[m.activeTab] == "Pods" || m.tabs[m.activeTab] == "svc") {
			m.closeLogs()
			if cmd := m.openResourceDetail(m.tabs[m.activeTab]); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Space toggles the row under the cursor for inclusion in the next
		// merged log stream; Ctrl+X clears all checkmarks. Pods-tab only.
		if m.appStateLoaded && m.tabs[m.activeTab] == "Pods" {
			switch keypress {
			case " ":
				m.podList.ToggleChecked(models.PodRowKey(m.podList.SelectedRow()))
				return m, nil
			case "ctrl+x":
				m.podList.ClearChecked()
				return m, nil
			}
		}

		// l reconciles the merged log pane to whatever's currently checked in
		// the Pods tab (or the row under the cursor, if nothing's checked).
		if m.appStateLoaded && keypress == "l" && m.tabs[m.activeTab] == "Pods" {
			if cmd := m.openPodLogs(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// r re-fetches only the active tab's resource type, across every
		// selected context — not all three resource types, to avoid tripling
		// API load on tabs the user isn't even looking at. Table cursor is
		// untouched: SetRows reuses the same table.Model, it doesn't reset it.
		if m.appStateLoaded && keypress == "r" {
			if cmd := m.refreshActiveTab(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Ctrl+W toggles wide mode on the active tab's table (sticky per tab,
		// reset on resize); Shift+Left/Right scroll one column at a time while
		// wide mode is on. Both are a no-op outside the three resource tabs.
		if m.appStateLoaded {
			switch keypress {
			case "ctrl+w":
				if t := m.activeResourceTable(); t != nil {
					t.ToggleWideMode()
				}
				return m, nil
			case "shift+left":
				if t := m.activeResourceTable(); t != nil && t.WideMode() {
					t.ScrollLeft()
				}
				return m, nil
			case "shift+right":
				if t := m.activeResourceTable(); t != nil && t.WideMode() {
					t.ScrollRight()
				}
				return m, nil
			}
		}

		// Content keys forwarded to the active tab
		if m.appStateLoaded {
			switch m.tabs[m.activeTab] {
			case "Deployments":
				cmd := m.deploymentList.Update(msg)
				return m, cmd
			case "Pods":
				cmd := m.podList.Update(msg)
				return m, cmd
			case "svc":
				cmd := m.svcList.Update(msg)
				return m, cmd
			}
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		ctxW, ctxH := getContextPaneDimensions(m.width, m.height)
		ctxMsg := tea.WindowSizeMsg{Width: ctxW, Height: ctxH}

		leftW := leftPaneWidthFor(m.width)
		tableW := m.width - leftW - 12
		tableH := m.height - 16
		if tableH < 1 {
			tableH = 1
		}
		m.tableW, m.tableH = tableW, tableH
		m.applyContentSizes()

		return m, m.contextList.Update(ctxMsg)

	case msgs.ResourceDetailMsg:
		if msg.Err != nil {
			m.deploymentDetail.SetError(msg.Err.Error())
			return m, nil
		}
		m.deploymentDetail.SetDetail(msg.Detail)
		return m, nil

	case msgs.LogStreamOpenedMsg:
		// Stale — this source has since been restarted or closed. Close the
		// stream rather than adopting it; other open sources are unaffected.
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			msg.Stream.Close()
			return m, nil
		}
		st.stream = msg.Stream
		st.scanner = cmds.NewLogScanner(msg.Stream)
		return m, cmds.WaitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner)

	case msgs.LogLineMsg:
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return m, nil
		}
		m.podLogs.AppendLine(msg.SourceKey, msg.Line)
		return m, cmds.WaitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner)

	case msgs.LogStreamClosedMsg:
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return m, nil
		}
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, msg.SourceKey)
		m.podLogs.SetStreamEnded(msg.SourceKey, msg.Err)
		return m, nil

	case msgs.DeploymentTableMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load deployments for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoading(msg.Context, false)
			{
				s := m.appState.Snapshot()
				m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
			}
			return m, nil
		}

		m.appState.SetDeployments(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.deploymentList.SetRows(snapshot.Deployments)
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)
		m.updateFocusStates()
		return m, nil

	case msgs.ContextsStateMsg:
		m.errorMessage = ""

		// Snapshot before mutations so we know which contexts were already present
		prevSelected := m.appState.Snapshot().SelectedContexts

		for _, contextName := range msg.Deselected {
			m.appState.RemoveContext(contextName)
		}

		for _, ms := range msg.Selected {
			m.appState.AddContext(ms.ContextName, ms.DefaultNamespace)
		}

		snapshot := m.appState.Snapshot()
		m.deploymentList.SetRows(snapshot.Deployments)
		m.podList.SetRows(snapshot.Pods)
		m.svcList.SetRows(snapshot.Services)
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)

		if len(snapshot.SelectedContexts) == 0 {
			m.appStateLoaded = false
			m.deploymentList.SetRows([]msgs.RowData{})
			m.podList.SetRows([]msgs.RowData{})
			m.svcList.SetRows([]msgs.RowData{})
			m.contextList.SetContextStates(nil, nil, nil)
			m.updateFocusStates()
			return m, nil
		}

		for i, t := range m.tabs {
			if t == "Deployments" {
				m.activeTab = i
				break
			}
		}

		// Only load contexts that are genuinely new (not previously selected).
		// Previously selected contexts that failed stay failed until the user
		// explicitly deselects and re-selects them — that removes them from
		// prevSelected and they appear here as new on the next Enter press.
		cmdSequence := []tea.Cmd{}
		for context, namespace := range snapshot.SelectedContexts {
			if _, alreadyPresent := prevSelected[context]; alreadyPresent {
				continue
			}
			m.appState.SetLoading(context, true)
			m.appState.SetLoadingPods(context, true)
			m.appState.SetLoadingServices(context, true)
			cmdSequence = append(cmdSequence,
				cmds.LoadDeploymentInfoCmd(m.Client, context, namespace),
				cmds.LoadPodInfoCmd(m.Client, context, namespace),
				cmds.LoadServiceInfoCmd(m.Client, context, namespace),
			)
		}

		{
			s := m.appState.Snapshot()
			m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
		}
		m.appStateLoaded = true
		m.updateFocusStates()

		if len(cmdSequence) == 0 {
			return m, nil
		}

		return m, tea.Batch(cmdSequence...)

	case msgs.PodTableMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load pods for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoadingPods(msg.Context, false)
			{
				s := m.appState.Snapshot()
				m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
			}
			return m, nil
		}

		m.appState.SetPods(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.podList.SetRows(snapshot.Pods)
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)
		return m, nil

	case msgs.ServiceTableMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load services for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoadingServices(msg.Context, false)
			{
				s := m.appState.Snapshot()
				m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
			}
			return m, nil
		}

		m.appState.SetServices(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.svcList.SetRows(snapshot.Services)
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)
		return m, nil

	case msgs.ErrorMsg:
		m.errorMessage = fmt.Sprintf("%s: %v", msg.Title, msg.Err)
		if msg.Context != "" {
			m.appState.SetError(msg.Context, m.errorMessage)
			{
				s := m.appState.Snapshot()
				m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
			}
		}
		return m, nil

	case msgs.RefreshTickMsg:
		// Always reschedule, even when auto-refresh is off or paused, so it
		// resumes on its own the moment the pane closes / it's toggled back on.
		next := m.refreshTickCmd()
		if !m.autoRefresh || m.showDetail || m.showLogs || !m.appStateLoaded {
			return m, next
		}
		if cmd := m.refreshActiveTab(); cmd != nil {
			return m, tea.Batch(cmd, next)
		}
		return m, next
	}

	// Forward non-key messages to the focused component(s)
	if m.focus == focusTabs && m.appStateLoaded {
		var forwardCmds []tea.Cmd
		switch m.tabs[m.activeTab] {
		case "Deployments":
			forwardCmds = append(forwardCmds, m.deploymentList.Update(msg))
		case "Pods":
			forwardCmds = append(forwardCmds, m.podList.Update(msg))
		case "svc":
			forwardCmds = append(forwardCmds, m.svcList.Update(msg))
		}
		if m.showDetail {
			forwardCmds = append(forwardCmds, m.deploymentDetail.Update(msg))
		}
		if m.showLogs {
			forwardCmds = append(forwardCmds, m.podLogs.Update(msg))
		}
		if len(forwardCmds) > 0 {
			return m, tea.Batch(forwardCmds...)
		}
	}

	if m.focus == focusLeftPane {
		cmd := m.contextList.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *MainPage) logSlowUpdate(start time.Time) {
	elapsed := time.Since(start)
	if elapsed > 16*time.Millisecond {
		log.Printf("Slow update: %v", elapsed)
	}
}

func (m *MainPage) toggleFocus() {
	if m.focus == focusLeftPane {
		m.focus = focusTabs
	} else {
		m.focus = focusLeftPane
	}
	m.updateFocusStates()
}

func (m *MainPage) updateFocusStates() {
	m.contextList.SetFocused(m.focus == focusLeftPane)
	listActive := m.focus == focusTabs && !m.detailFocused && !m.logsFocused
	shouldFocusDeployments := listActive && m.tabs[m.activeTab] == "Deployments" && m.appStateLoaded
	m.deploymentList.SetFocused(shouldFocusDeployments)
	shouldFocusPods := listActive && m.tabs[m.activeTab] == "Pods" && m.appStateLoaded
	m.podList.SetFocused(shouldFocusPods)
	shouldFocusSvc := listActive && m.tabs[m.activeTab] == "svc" && m.appStateLoaded
	m.svcList.SetFocused(shouldFocusSvc)
	m.deploymentDetail.SetFocused(m.focus == focusTabs && m.detailFocused)
	m.podLogs.SetFocused(m.focus == focusTabs && m.logsFocused)
}

// detailPaneHeightPercent is the share of the tab content area given to the
// detail pane, out of the remainder after list rows.
const detailPaneHeightPercent = 45

// applyContentSizes resizes the Deployments/Pods lists and the bottom pane
// (Detail or Logs — mutually exclusive) to split the tab content area in two
// whenever either is open.
func (m *MainPage) applyContentSizes() {
	listH := m.tableH
	detailH := 0
	if m.showDetail || m.showLogs {
		detailH = m.tableH * detailPaneHeightPercent / 100
		if detailH < 6 {
			detailH = 6
		}
		listH = m.tableH - detailH - 2 // 2 lines reserved: the divider and the pane header
		if listH < 3 {
			listH = 3
		}
	}
	m.deploymentList.SetSize(m.tableW, listH)
	m.podList.SetSize(m.tableW, listH)
	m.svcList.SetSize(m.tableW, listH)
	m.deploymentDetail.SetSize(m.tableW, detailH)
	m.podLogs.SetSize(m.tableW, detailH)
}

// openResourceDetail loads detail for the currently selected row on the given
// top tab ("Deployments", "Pods", or "svc") into the shared bottom detail pane.
// Returns nil if there's no valid selection.
func (m *MainPage) openResourceDetail(sourceTab string) tea.Cmd {
	var kind, name, ctxName, namespace string

	switch sourceTab {
	case "Deployments":
		row := m.deploymentList.SelectedRow()
		if row == nil {
			return nil
		}
		kind = "Deployment"
		name, _ = row[msgs.DeployKeyName].(string)
		ctxName, _ = row[msgs.DeployKeyContext].(string)
		namespace, _ = row[msgs.DeployKeyNamespace].(string)
	case "Pods":
		row := m.podList.SelectedRow()
		if row == nil {
			return nil
		}
		kind = "Pod"
		name, _ = row[msgs.PodKeyName].(string)
		namespace, _ = row[msgs.PodKeyNamespace].(string)
		ctxName, _ = row[msgs.PodKeyContext].(string)
	case "svc":
		row := m.svcList.SelectedRow()
		if row == nil {
			return nil
		}
		kind = "Service"
		name, _ = row[msgs.SvcKeyName].(string)
		namespace, _ = row[msgs.SvcKeyNamespace].(string)
		ctxName, _ = row[msgs.SvcKeyContext].(string)
	default:
		return nil
	}

	// Re-entering the row already shown in the pane just refocuses it instead
	// of re-fetching — e.g. after Esc dropped back to the list to scroll/pick
	// a row, Enter on that same row jumps straight back in.
	if m.showDetail && m.deploymentDetail.Matches(kind, name, ctxName) {
		m.detailFocused = true
		m.applyContentSizes()
		m.updateFocusStates()
		return nil
	}

	m.deploymentDetail.StartLoading(kind, name, ctxName)
	m.showDetail = true
	m.detailFocused = true
	m.applyContentSizes()
	m.updateFocusStates()

	switch kind {
	case "Pod":
		return cmds.LoadPodDetailCmd(m.Client, ctxName, namespace, name)
	case "Service":
		return cmds.LoadServiceDetailCmd(m.Client, ctxName, namespace, name)
	default:
		return cmds.LoadDeploymentDetailCmd(m.Client, ctxName, namespace, name)
	}
}

// wideModeTable is implemented identically by DeploymentPage/PodPage/
// ServicePage — the Ctrl+W wide-mode toggle and Shift+Left/Right column
// scroll operate on whichever of the three is the active tab.
type wideModeTable interface {
	ToggleWideMode()
	WideMode() bool
	ScrollLeft()
	ScrollRight()
	ScrollStatus() (offset, total int, ok bool)
}

// activeResourceTable returns the active tab's table as a wideModeTable, or
// nil if the active tab isn't one of the three resource tables.
func (m *MainPage) activeResourceTable() wideModeTable {
	switch m.tabs[m.activeTab] {
	case "Deployments":
		return m.deploymentList
	case "Pods":
		return m.podList
	case "svc":
		return m.svcList
	}
	return nil
}

// refreshActiveTab re-fetches the active tab's resource type for every
// selected context, batched with tea.Batch the same way ContextsStateMsg
// kicks off the initial load. Setting the same per-context loading flag used
// on initial load means the existing "⏳ N loading" status bar hint just
// picks the refresh up automatically.
func (m *MainPage) refreshActiveTab() tea.Cmd {
	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		return nil
	}

	var cmdSequence []tea.Cmd
	switch m.tabs[m.activeTab] {
	case "Deployments":
		for context, namespace := range snapshot.SelectedContexts {
			m.appState.SetLoading(context, true)
			cmdSequence = append(cmdSequence, cmds.LoadDeploymentInfoCmd(m.Client, context, namespace))
		}
	case "Pods":
		for context, namespace := range snapshot.SelectedContexts {
			m.appState.SetLoadingPods(context, true)
			cmdSequence = append(cmdSequence, cmds.LoadPodInfoCmd(m.Client, context, namespace))
		}
	case "svc":
		for context, namespace := range snapshot.SelectedContexts {
			m.appState.SetLoadingServices(context, true)
			cmdSequence = append(cmdSequence, cmds.LoadServiceInfoCmd(m.Client, context, namespace))
		}
	}

	if len(cmdSequence) == 0 {
		return nil
	}

	s := m.appState.Snapshot()
	m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
	return tea.Batch(cmdSequence...)
}

// podLogTarget identifies one pod/container source to be tailed.
type podLogTarget struct {
	key                            string // context/namespace/pod/container
	context, namespace, pod, cntnr string
}

// podLogTargets expands the given raw Pods-table rows into one target per
// container (all containers of each pod are tailed — decision #4).
func podLogTargets(rows []msgs.RowData) []podLogTarget {
	var targets []podLogTarget
	for _, row := range rows {
		containers, _ := row[msgs.PodKeyContainers].(string)
		if containers == "" {
			continue
		}
		name, _ := row[msgs.PodKeyName].(string)
		namespace, _ := row[msgs.PodKeyNamespace].(string)
		ctxName, _ := row[msgs.PodKeyContext].(string)
		for _, container := range strings.Split(containers, ",") {
			targets = append(targets, podLogTarget{
				key:       ctxName + "/" + namespace + "/" + name + "/" + container,
				context:   ctxName,
				namespace: namespace,
				pod:       name,
				cntnr:     container,
			})
		}
	}
	return targets
}

// openPodLogs reconciles the merged log pane to whatever's currently checked
// in the Pods tab — or, if nothing's checked, the single row under the
// cursor (preserving the original single-pod behavior). Sources newly
// present are opened, sources no longer targeted are closed, and unchanged
// sources are left running untouched. An empty target set closes the pane.
func (m *MainPage) openPodLogs() tea.Cmd {
	var rows []msgs.RowData
	if keys := m.podList.CheckedKeys(); len(keys) > 0 {
		for _, key := range keys {
			if row := m.podList.CheckedRow(key); row != nil {
				rows = append(rows, row)
			}
		}
	} else if row := m.podList.SelectedRow(); row != nil {
		rows = append(rows, row)
	}

	targets := podLogTargets(rows)
	if len(targets) == 0 {
		m.closeLogs()
		m.applyContentSizes()
		return nil
	}

	targetSet := make(map[string]podLogTarget, len(targets))
	for _, t := range targets {
		targetSet[t.key] = t
	}

	// Close sources no longer targeted.
	for key := range m.logStreams {
		if _, wanted := targetSet[key]; !wanted {
			m.closeLogSource(key)
		}
	}

	// Open sources newly targeted; unchanged ones are left running.
	var openCmds []tea.Cmd
	for key, t := range targetSet {
		if _, exists := m.logStreams[key]; exists {
			continue
		}
		m.podLogs.AddSource(key, t.pod, t.namespace, t.context, t.cntnr)
		m.logStreams[key] = &logStreamState{generation: 1}
		openCmds = append(openCmds, cmds.OpenPodLogStreamCmd(m.Client, t.context, t.namespace, t.pod, t.cntnr, key, 1))
	}

	m.closeDetail()
	m.showLogs = true
	m.logsFocused = true
	m.applyContentSizes()
	m.updateFocusStates()

	if len(openCmds) == 0 {
		return nil
	}
	return tea.Batch(openCmds...)
}

// closeLogSource stops one source's stream (if any) and removes it from
// both the stream registry and the render model.
func (m *MainPage) closeLogSource(key string) {
	if st, ok := m.logStreams[key]; ok {
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, key)
	}
	m.podLogs.RemoveSource(key)
}

// closeDetail closes the Detail pane, if open. It holds no external
// resources, so there's nothing to release beyond the flags themselves.
func (m *MainPage) closeDetail() {
	m.showDetail = false
	m.detailFocused = false
}

// closeLogs closes the Log pane, if open, stopping every open source's
// underlying stream.
func (m *MainPage) closeLogs() {
	m.stopLogStream()
	m.podLogs.Clear()
	m.showLogs = false
	m.logsFocused = false
}

// stopLogStream closes every currently open log source's stream. Safe to
// call when nothing is streaming.
func (m *MainPage) stopLogStream() {
	for key, st := range m.logStreams {
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, key)
	}
}

func (m *MainPage) View() string {
	if m.width < views.MinContentWidth || m.height < views.MinHeight {
		return m.renderTooSmallOverlay()
	}

	snapshot := m.appState.Snapshot()

	leftPaneWidth := leftPaneWidthFor(m.width)
	leftPane := ""
	tabBlur := false
	tabBottom := styles.WindowStyle

	// Starting guess for the left pane's height; reconciled exactly against
	// the right side's actual rendered height below (see rightLines/leftLines).
	leftPaneHeight := m.height - 5

	switch m.focus {
	case focusLeftPane:
		leftPane = views.RenderLeftPane(m.contextList.View(), leftPaneWidth, leftPaneHeight)
		tabBlur = true
		tabBottom = styles.WindowBlurStyle

	case focusTabs:
		leftPane = views.RenderLeftPaneBlur(m.contextList.View(), leftPaneWidth, leftPaneHeight)
		tabBlur = false
		tabBottom = styles.WindowStyle
	}

	// Build tab content
	tabs := strings.Builder{}
	tabWidth := m.width - leftPaneWidth - 8

	emptyMsg := "No contexts selected\n\nPress Tab to focus contexts\nSpace to select • Enter to load"
	switch m.tabs[m.activeTab] {
	case "Deployments":
		if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
			m.tabContent = styles.HelpBoxStyle().Render(emptyMsg)
		} else {
			m.tabContent = m.deploymentList.View()
		}
	case "Pods":
		if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
			m.tabContent = styles.HelpBoxStyle().Align(lipgloss.Center).Render(emptyMsg)
		} else {
			m.tabContent = m.podList.View()
		}
	case "svc":
		if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
			m.tabContent = styles.HelpBoxStyle().Align(lipgloss.Center).Render(emptyMsg)
		} else {
			m.tabContent = m.svcList.View()
		}
	default:
		m.tabContent = styles.HelpBoxStyle().Render(emptyMsg)
	}

	// Loading indicator (inline — it's brief and doesn't break layout)
	if hasLoading(snapshot.LoadingStates) {
		m.tabContent = m.renderLoadingIndicator(snapshot.LoadingStates) + "\n\n" + m.tabContent
	}

	// The bottom pane (Detail or Logs — mutually exclusive) is cross-cutting:
	// it splits whichever top tab's content area is active in two, rather
	// than being a peer tab of its own.
	if m.showDetail || m.showLogs {
		p := styles.CatppuccinMocha()
		// RenderTabHeaders divides tabWidth by len(tabs) with integer
		// division, so the box's real rendered width can be up to
		// len(tabs)-1 characters narrower than tableW depending on the exact
		// window width. Pad target width safely under that so this block
		// never gets word-wrapped by the outer container — a wrap here (not
		// just a truncation) adds a physical line, which is what threw the
		// left/right pane heights out of sync at some window widths.
		const tabRoundingSafetyMargin = 2 // len(m.tabs)-1, the max rounding loss from tabWidth/len(tabs)
		dividerW := m.tableW - tabRoundingSafetyMargin
		if dividerW < 1 {
			dividerW = 1
		}
		divider := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", dividerW))

		var header, body string
		if m.showDetail {
			header = m.deploymentDetail.Header(dividerW)
			body = m.deploymentDetail.View()
		} else {
			header = m.podLogs.Header(dividerW)
			body = m.podLogs.View()
		}

		joined := lipgloss.JoinVertical(lipgloss.Left,
			m.tabContent,
			divider,
			header,
			body,
		)
		// Right-pad every line up to a uniform minimum width so the outer
		// container's Align(Center) shifts the whole block by one constant
		// amount instead of centering each line individually — the latter is
		// what made YAML/status lines of varying length look raggedly
		// centered. Only ever pads (never truncates/wraps) — table rows carry
		// per-cell padding that already makes them wider than dividerW, and
		// forcing a hard Width() there would word-wrap the header row.
		m.tabContent = padLinesToMinWidth(joined, dividerW)
	}

	tabHeaders := views.RenderTabHeaders(m.activeTab, m.tabs, tabWidth, tabBlur)
	tabs.WriteString(tabHeaders)
	tabs.WriteString("\n")
	tabs.WriteString(tabBottom.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Height(m.height - 8).Align(lipgloss.Center).Render(m.tabContent))

	// lipgloss's Height()+Border()+Padding() frame math doesn't add up to a
	// fixed constant across every width/content combination — real content
	// (wrapped context names, table rows) can shift each side's natural
	// rendered height by a line or two in either direction depending on the
	// exact terminal size. Rather than trust a derived constant, measure both
	// blocks and, if they differ, bump the shorter one's declared height by
	// the exact gap and re-render it — guaranteeing the two borders land on
	// the same row regardless of any wrapping quirk on either side.
	rightLines := lineCount(tabs.String())
	leftLines := lineCount(leftPane)
	if gap := rightLines - leftLines; gap > 0 {
		leftPaneHeight += gap
		switch m.focus {
		case focusLeftPane:
			leftPane = views.RenderLeftPane(m.contextList.View(), leftPaneWidth, leftPaneHeight)
		case focusTabs:
			leftPane = views.RenderLeftPaneBlur(m.contextList.View(), leftPaneWidth, leftPaneHeight)
		}
	} else if gap < 0 {
		tabs.Reset()
		tabs.WriteString(tabHeaders)
		tabs.WriteString("\n")
		tabs.WriteString(tabBottom.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Height(m.height - 8 - gap).Align(lipgloss.Center).Render(m.tabContent))
	}

	fullView := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, leftPane, tabs.String()),
		m.renderStatusBar(snapshot),
	)

	// Overlays rendered on top of the full view (help > error)
	if m.showHelp {
		return m.renderHelpOverlay()
	}
	if m.errorMessage != "" {
		return m.renderErrorOverlay(m.errorMessage)
	}
	if len(snapshot.Errors) > 0 {
		return m.renderErrorSummaryOverlay(snapshot.Errors)
	}

	return fullView
}

func (m *MainPage) renderStatusBar(snapshot state.Snapshot) string {
	p := styles.CatppuccinMocha()
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Padding(0, 1)

	selectedCtx := len(snapshot.SelectedContexts)
	errCount := len(snapshot.Errors)
	loadingCount := 0
	for _, l := range snapshot.LoadingStates {
		if l {
			loadingCount++
		}
	}
	activeTabName := m.tabs[m.activeTab]
	var activeCount int
	switch activeTabName {
	case "Deployments":
		activeCount = len(snapshot.Deployments)
	case "Pods":
		activeCount = len(snapshot.Pods)
	case "svc":
		activeCount = len(snapshot.Services)
	}

	focusStr := "Left Pane"
	if m.focus == focusTabs {
		focusStr = "Tabs"
	}

	left := leftStyle.Render(fmt.Sprintf("Contexts: %d", selectedCtx))
	mid := midStyle.Render(fmt.Sprintf("Tab: %s | Focus: %s", activeTabName, focusStr))

	// Dynamic status bits (loading / count / errors) — count reflects
	// whichever tab currently has focus, not always Deployments.
	var statusBits []string
	if loadingCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⏳ %d loading", loadingCount))
	} else if activeCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("%s: %d", activeTabName, activeCount))
	}
	if errCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⚠ %d error(s)", errCount))
	}
	if activeTabName == "Pods" {
		if checkedCount := len(m.podList.CheckedKeys()); checkedCount > 0 {
			statusBits = append(statusBits, fmt.Sprintf("☑ %d checked · l: open merged · Ctrl+X: clear", checkedCount))
		}
	}
	if t := m.activeResourceTable(); t != nil {
		if offset, total, ok := t.ScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ col %d/%d ▸", offset, total))
		}
	}
	if len(statusBits) == 0 {
		statusBits = append(statusBits, "Ready")
	}
	status := rightStyle.Render(strings.Join(statusBits, "  |  "))

	// Hints are a fixed, separate element anchored to the far right
	hints := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Tab:focus  [ ]:tabs  ?:help  q:quit")

	barWidth := m.width - 2
	leftMid := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", mid)
	rightSection := lipgloss.JoinHorizontal(lipgloss.Top, status, "   ", hints)
	spacerWidth := barWidth - lipgloss.Width(leftMid) - lipgloss.Width(rightSection)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	line := leftMid + strings.Repeat(" ", spacerWidth) + rightSection

	return styles.StatusBar.Width(barWidth).Render(line)
}

func (m *MainPage) renderHelpOverlay() string {
	p := styles.CatppuccinMocha()

	titleStyle := lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(p.Blue).Bold(true).Width(22)
	descStyle := lipgloss.NewStyle().Foreground(p.Text)
	sepStyle := lipgloss.NewStyle().Foreground(p.Overlay0)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Mauve).
		Background(p.Mantle).
		Padding(1, 3)

	type binding struct{ key, desc string }
	bindings := []binding{
		{"Tab / Shift+Tab", "Switch pane focus"},
		{"[ / ]", "Navigate tabs"},
		{"← / →", "Navigate tabs (alias)"},
		{"↑ / ↓   j / k", "Move up / down"},
		{"Space", "Toggle context selection / check a Pods row for log tailing"},
		{"Enter", "Confirm selection & load / open + focus detail pane (refocuses instantly if already loaded)"},
		{"l (Pods tab)", "Open/reconcile the merged log pane for checked rows (or the row under the cursor)"},
		{"Ctrl+X (Pods tab)", "Clear all checked rows"},
		{"r", "Refresh the active tab's resource list across all selected contexts"},
		{"c (log pane focused)", "Isolate one source's view, or return to the full merge"},
		{"Ctrl+R", "Jump back into an open detail pane without changing its resource"},
		{"R", "Toggle auto-refresh on/off"},
		{"↑/↓ j/k PgUp/PgDn", "Scroll detail/log pane (while it has focus)"},
		{"Home / End", "Jump to top / bottom of detail/log pane"},
		{"Esc", "Unfocus detail/log pane, then close it / overlay / dismiss error"},
		{"?", "Toggle this help"},
		{"q / Ctrl+C", "Quit"},
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Keybindings"))
	lines = append(lines, sepStyle.Render(strings.Repeat("─", 38)))
	for _, b := range bindings {
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(b.key),
			descStyle.Render(b.desc),
		))
	}

	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box)
}

// renderTooSmallOverlay replaces the whole TUI with a plain message when the
// terminal is below views.MinContentWidth x views.MinHeight — below that, the
// real layout doesn't have room to render without breaking, so we don't try.
func (m *MainPage) renderTooSmallOverlay() string {
	p := styles.CatppuccinMocha()
	msg := fmt.Sprintf(
		"Terminal window is too small\n\nCurrent size: %d x %d\nMinimum size: %d x %d\n\nPlease resize your terminal",
		m.width, m.height, views.MinContentWidth, views.MinHeight,
	)
	body := lipgloss.NewStyle().Foreground(p.Text).Align(lipgloss.Center).Render(msg)

	// Below a certain point there isn't even room for the box border/padding
	// — fall back to bare text rather than let lipgloss mangle it further.
	if m.width < 20 || m.height < 6 {
		return body
	}

	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Yellow).
		Padding(1, 3).
		Align(lipgloss.Center).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MainPage) renderErrorOverlay(msg string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Error")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", maxW-2))
	body := lipgloss.NewStyle().Foreground(p.Text).Render(msg)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *MainPage) renderErrorSummaryOverlay(errors map[string]string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Errors encountered")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", maxW-2))
	var bodyLines []string
	for ctx, err := range errors {
		bodyLines = append(bodyLines, fmt.Sprintf("• %s: %s", ctx, err))
	}
	body := lipgloss.NewStyle().Foreground(p.Text).Render(strings.Join(bodyLines, "\n"))
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *MainPage) renderLoadingIndicator(loading map[string]bool) string {
	p := styles.CatppuccinMocha()
	loadingStyle := lipgloss.NewStyle().
		Foreground(p.Blue).
		Background(p.Surface0).
		Padding(0, 1)

	var loadingContexts []string
	for ctx, isLoading := range loading {
		if isLoading {
			loadingContexts = append(loadingContexts, ctx)
		}
	}

	if len(loadingContexts) == 0 {
		return ""
	}

	return loadingStyle.Render(fmt.Sprintf("⏳ Loading: %s...", strings.Join(loadingContexts, ", ")))
}

// padLinesToMinWidth right-pads every line of content with spaces so it is at
// least width columns wide, without ever truncating or wrapping lines that
// are already wider.
func padLinesToMinWidth(content string, width int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		switch pad := width - lipgloss.Width(line); {
		case pad > 0:
			lines[i] = line + strings.Repeat(" ", pad)
		case pad < 0:
			// Safety net: a line snuck past width (e.g. table cell padding
			// overhead) — truncate rather than let lipgloss word-wrap it,
			// which is what produced the ragged/centered look before.
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// lineCount returns the number of lines in a rendered lipgloss block.
func lineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

func hasLoading(loading map[string]bool) bool {
	for _, isLoading := range loading {
		if isLoading {
			return true
		}
	}
	return false
}

func getContextPaneDimensions(w, h int) (cW, cH int) {
	cW = leftPaneWidthFor(w)
	cH = h - 10
	return cW, cH
}

// leftPaneWidthFor computes the left (context list) pane's width: a quarter
// of the terminal width, capped at views.MaxLeftPaneWidth. Context names are
// short, so on wide terminals a fixed fraction wastes space that the tab
// area could otherwise use.
func leftPaneWidthFor(w int) int {
	lw := w / 4
	if lw > views.MaxLeftPaneWidth {
		lw = views.MaxLeftPaneWidth
	}
	return lw
}
