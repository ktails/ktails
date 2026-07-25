// Package pages, it implements main routing to different pages.
package pages

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/state"
	"github.com/ktails/ktails/internal/tui/cmds"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/ktails/ktails/internal/tui/views"
	"github.com/ktails/ktails/internal/tui/watch"
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

	// tabs — one per resource kind, in msgs.Kinds() order
	tabs      []msgs.ResourceKind
	activeTab int

	// App state — per-context UI status (selection, loading, errors). Rows
	// live in the watch Supervisor's caches, not here.
	appState       *state.AppState
	appStateLoaded bool

	// base models
	contextList *models.ContextsInfo
	// tables holds the one resource table per tab; all three share the
	// models.ResourceTable implementation and differ only by spec.
	tables           map[msgs.ResourceKind]*models.ResourceTable
	deploymentDetail *models.ResourceDetailPage
	focus            focusTarget

	// k8s client
	Client *k8s.Client

	// watchSup owns every watch stream, cache, and reconnect decision —
	// MainPage only starts/stops contexts, forwards watch messages to
	// Handle, and reads rows back out.
	watchSup *watch.Supervisor

	// UI overlays
	errorMessage string
	showHelp     bool

	// Auto-refresh — a self-rescheduling tick. Table data itself is kept
	// current by the watch streams; the tick's only remaining job is to
	// re-render Age text from the local watch caches (no API calls). Paused
	// (tick still reschedules, but the re-render is skipped) while the Detail
	// or Log pane is open, since a background reorder under a pinned pane is
	// more disruptive than helpful.
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
	if refreshIntervalSeconds < 1 {
		refreshIntervalSeconds = 5
	}

	tables := make(map[msgs.ResourceKind]*models.ResourceTable)
	for _, kind := range msgs.Kinds() {
		tables[kind] = models.NewResourceTable(kind)
	}

	m := &MainPage{
		Client:           c,
		appState:         state.NewAppState(),
		tabs:             msgs.Kinds(),
		contextList:      models.NewContextInfo(c),
		tables:           tables,
		deploymentDetail: models.NewResourceDetailPage(),
		podLogs:          models.NewLogPage(),
		logStreams:       make(map[string]*logStreamState),
		watchSup:         watch.NewSupervisor(c),
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

// activeKind returns the resource kind of the active tab.
func (m *MainPage) activeKind() msgs.ResourceKind {
	return m.tabs[m.activeTab]
}

// activeTable returns the active tab's resource table.
func (m *MainPage) activeTable() *models.ResourceTable {
	return m.tables[m.activeKind()]
}

// renderTabTitle renders the compact tab strip embedded in the Tab Area
// box's top border — "Deployments · Pods · svc" with the active tab
// accented (or merely brightened, when the Tab Area isn't focused).
func (m *MainPage) renderTabTitle(focused bool) string {
	p := styles.CatppuccinMocha()
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(" · ")
	parts := make([]string, 0, len(m.tabs))
	for i, kind := range m.tabs {
		st := lipgloss.NewStyle().Foreground(p.Overlay1)
		if i == m.activeTab {
			st = st.Bold(true).Foreground(p.Subtext1)
			if focused {
				st = st.Foreground(p.Mauve)
			}
		}
		parts = append(parts, st.Render(kind.Title()))
	}
	return strings.Join(parts, sep)
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return tea.Batch(m.refreshTickCmd(), recheckStartupSizeCmd())
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

// startupResizeCheckDelay is how long after Init() to re-verify the real
// terminal size against whatever WindowSizeMsg bubbletea delivered at
// startup.
const startupResizeCheckDelay = 250 * time.Millisecond

// recheckStartupSizeCmd re-queries the terminal's actual size shortly after
// startup and delivers it as a fresh WindowSizeMsg. bubbletea reads the
// initial size via one synchronous ioctl before the first frame renders,
// then only ever corrects it again on a real SIGWINCH. Some terminal
// emulators (observed with Ghostty on macOS, launched directly into a
// tiled/half-screen layout) haven't finished settling their own window
// geometry at that instant, so the ioctl reads a stale size and — because
// no further physical resize happens on its own — the layout stays wrong
// until the user manually triggers one. This fires once to catch that race;
// if the size didn't actually change, the resulting WindowSizeMsg is a
// harmless no-op.
func recheckStartupSizeCmd() tea.Cmd {
	return tea.Tick(startupResizeCheckDelay, func(time.Time) tea.Msg {
		w, h, err := term.GetSize(os.Stdout.Fd())
		if err != nil {
			return nil
		}
		return tea.WindowSizeMsg{Width: w, Height: h}
	})
}

func (m *MainPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	defer m.logSlowUpdate(start)

	// Watch messages are the Supervisor's to interpret; MainPage only
	// applies the resulting row/error updates to the UI.
	if upd, cmd, handled := m.watchSup.Handle(msg); handled {
		m.applyWatchUpdate(upd)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		keypress := msg.String()

		// Help overlay is modal — only ? and esc pass through
		if m.showHelp {
			if keypress == "?" || keypress == "esc" {
				m.showHelp = false
			}
			return m, nil
		}

		// While a resource table is actively capturing filter text (see
		// rowFilter in models/table.go), every keypress must reach it
		// untouched — otherwise single-letter global shortcuts like "r"
		// (refresh) or "l" (open logs) below would get swallowed into a
		// command instead of becoming part of the filter query.
		if m.focus == focusTabs && !m.detailFocused && !m.logsFocused {
			if _, _, typing, ok := m.activeTable().FilterStatus(); ok && typing {
				return m, m.activeTable().Update(msg)
			}
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
		// 'c' and 'w', which MainPage intercepts directly — both are pure view
		// toggles with no stream side effects (isolate/return-to-merged a
		// single source, and soft-wrap on/off).
		if m.logsFocused {
			switch keypress {
			case "c":
				m.podLogs.CycleIsolation()
				return m, nil
			case "w":
				m.podLogs.ToggleWrap()
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
		// stays put beneath whichever tab you land on. Every tab needs loaded
		// data, so navigation is gated on contexts being selected and loaded.
		switch keypress {
		case "right", "]":
			next := m.activeTab + 1
			if next >= len(m.tabs) {
				return m, nil
			}
			if !m.appStateLoaded || len(m.appState.Snapshot().SelectedContexts) == 0 {
				return m, nil
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

		// Enter on a selected row (re)loads the detail pane for that row and
		// gives it keyboard focus for scrolling. Detail and Logs share the
		// same bottom slot and are mutually exclusive.
		if m.appStateLoaded && keypress == "enter" {
			m.closeLogs()
			if cmd := m.openResourceDetail(m.activeKind()); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Space toggles the row under the cursor for inclusion in the next
		// merged log stream; Ctrl+X clears all checkmarks. Pods-tab only.
		if m.appStateLoaded && m.activeKind() == msgs.KindPods {
			switch keypress {
			case "space":
				pods := m.tables[msgs.KindPods]
				pods.ToggleChecked(models.PodRowKey(pods.SelectedRow()))
				return m, nil
			case "ctrl+x":
				m.tables[msgs.KindPods].ClearChecked()
				return m, nil
			}
		}

		// l reconciles the merged log pane to whatever's currently checked in
		// the Pods tab (or the row under the cursor, if nothing's checked).
		if m.appStateLoaded && keypress == "l" && m.activeKind() == msgs.KindPods {
			if cmd := m.openPodLogs(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// r force-restarts the watch(es) for only the active tab's resource
		// kind, across every selected context — not all three kinds, to
		// avoid tripling API load on tabs the user isn't even looking at.
		// Table cursor is untouched: SetRows reuses the same table.Model, it
		// doesn't reset it.
		if m.appStateLoaded && keypress == "r" {
			if cmd := m.restartActiveTabWatch(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Ctrl+W toggles wide mode on the active tab's table (sticky per tab,
		// reset on resize); Shift+Left/Right scroll one column at a time while
		// wide mode is on.
		if m.appStateLoaded {
			switch keypress {
			case "ctrl+w":
				t := m.activeTable()
				wasWide := t.WideMode()
				t.ToggleWideMode()
				if !wasWide && t.WideMode() && m.activeKind() == msgs.KindServices {
					return m, m.fetchServiceEndpointsIfNeeded()
				}
				return m, nil
			case "shift+left":
				if t := m.activeTable(); t.WideMode() {
					t.ScrollLeft()
				}
				return m, nil
			case "shift+right":
				if t := m.activeTable(); t.WideMode() {
					t.ScrollRight()
				}
				return m, nil
			}
		}

		// Content keys forwarded to the active tab
		if m.appStateLoaded {
			return m, m.activeTable().Update(msg)
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// views.Solve is the single owner of the layout budget: every pane
		// gets exactly its solved content rectangle.
		r := views.Solve(m.width, m.height)
		m.tableW, m.tableH = r.RightContentW, r.RightContentH
		m.applyContentSizes()

		return m, m.contextList.Update(tea.WindowSizeMsg{Width: r.LeftContentW, Height: r.LeftContentH})

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

	case msgs.ContextsStateMsg:
		return m, m.applyContextsState(msg)

	case msgs.ServiceEndpointsMsg:
		if msg.Err != nil {
			// Allow a later Ctrl+W toggle to retry instead of getting stuck
			// showing the "…" placeholder forever.
			m.watchSup.ClearEndpointsRequested(msg.Context)
			return m, nil
		}
		m.watchSup.SetEndpoints(msg.Context, msg.Namespace, msg.Endpoints)
		m.tables[msgs.KindServices].SetRows(m.watchSup.Rows(msgs.KindServices))
		return m, nil

	case msgs.RefreshTickMsg:
		// Always reschedule, even when auto-refresh is off or paused, so it
		// resumes on its own the moment the pane closes / it's toggled back on.
		// Table data itself is kept current by the watch streams; this tick
		// just re-renders Age text from the local watch caches — purely
		// local, zero API calls.
		next := m.refreshTickCmd()
		if !m.autoRefresh || m.showDetail || m.showLogs || !m.appStateLoaded {
			return m, next
		}
		m.reRenderAgeFromWatchCaches()
		return m, next
	}

	// Forward non-key messages to the focused component(s)
	if m.focus == focusTabs && m.appStateLoaded {
		forwardCmds := []tea.Cmd{m.activeTable().Update(msg)}
		if m.showDetail {
			forwardCmds = append(forwardCmds, m.deploymentDetail.Update(msg))
		}
		if m.showLogs {
			forwardCmds = append(forwardCmds, m.podLogs.Update(msg))
		}
		return m, tea.Batch(forwardCmds...)
	}

	if m.focus == focusLeftPane {
		cmd := m.contextList.Update(msg)
		return m, cmd
	}

	return m, nil
}

// applyWatchUpdate applies a Supervisor update to the UI: fresh rows for one
// kind's table, or a permanently failed watch surfaced as a context error.
func (m *MainPage) applyWatchUpdate(upd *watch.Update) {
	if upd == nil {
		return
	}
	if upd.GaveUp {
		errMsg := upd.Err.Error()
		m.appState.SetError(upd.Context, errMsg)
		m.errorMessage = errMsg
		m.syncContextStates()
		return
	}
	if upd.RowsChanged {
		m.appState.MarkLoaded(upd.Kind, upd.Context)
		m.tables[upd.Kind].SetRows(m.watchSup.Rows(upd.Kind))
		m.syncContextStates()
		m.updateFocusStates()
	}
}

// applyContextsState reconciles selection changes from the Context List:
// deselected contexts stop being watched and drop their rows; newly selected
// contexts start watches for all three resource kinds.
func (m *MainPage) applyContextsState(msg msgs.ContextsStateMsg) tea.Cmd {
	m.errorMessage = ""

	// Snapshot before mutations so we know which contexts were already present
	prevSelected := m.appState.Snapshot().SelectedContexts

	for _, contextName := range msg.Deselected {
		m.appState.RemoveContext(contextName)
		m.watchSup.StopContext(contextName)
	}

	for _, ms := range msg.Selected {
		m.appState.AddContext(ms.ContextName, ms.DefaultNamespace)
	}

	for _, kind := range m.tabs {
		m.tables[kind].SetRows(m.watchSup.Rows(kind))
	}
	m.syncContextStates()

	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		m.appStateLoaded = false
		for _, kind := range m.tabs {
			m.tables[kind].SetRows([]msgs.RowData{})
		}
		m.contextList.SetContextStates(nil, nil, nil)
		m.updateFocusStates()
		return nil
	}

	m.activeTab = 0 // land on the Deployments tab

	// Only load contexts that are genuinely new (not previously selected).
	// Previously selected contexts that failed stay failed until the user
	// explicitly deselects and re-selects them — that removes them from
	// prevSelected and they appear here as new on the next Enter press.
	cmdSequence := []tea.Cmd{}
	for context, namespace := range snapshot.SelectedContexts {
		if _, alreadyPresent := prevSelected[context]; alreadyPresent {
			continue
		}
		for _, kind := range m.tabs {
			m.appState.SetLoading(kind, context, true)
		}
		cmdSequence = append(cmdSequence, m.watchSup.StartContext(context, namespace)...)
	}

	m.syncContextStates()
	m.appStateLoaded = true
	m.updateFocusStates()

	if len(cmdSequence) == 0 {
		return nil
	}
	return tea.Batch(cmdSequence...)
}

// syncContextStates pushes the current loading/error/loaded status into the
// Context List's per-context indicators.
func (m *MainPage) syncContextStates() {
	s := m.appState.Snapshot()
	m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
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
	listActive := m.focus == focusTabs && !m.detailFocused && !m.logsFocused && m.appStateLoaded
	for _, kind := range m.tabs {
		m.tables[kind].SetFocused(listActive && kind == m.activeKind())
	}
	m.deploymentDetail.SetFocused(m.focus == focusTabs && m.detailFocused)
	m.podLogs.SetFocused(m.focus == focusTabs && m.logsFocused)
}

// detailPaneHeightPercent is the share of the tab content area given to the
// detail pane, out of the remainder after list rows.
const detailPaneHeightPercent = 45

// applyContentSizes resizes the resource tables and the bottom pane (Detail
// or Logs — mutually exclusive) to split the tab content area in two
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
	for _, kind := range m.tabs {
		m.tables[kind].SetSize(m.tableW, listH)
	}
	m.deploymentDetail.SetSize(m.tableW, detailH)
	m.podLogs.SetSize(m.tableW, detailH)
}

// openResourceDetail loads detail for the currently selected row on the given
// tab into the shared bottom detail pane. Returns nil if there's no valid
// selection.
func (m *MainPage) openResourceDetail(kind msgs.ResourceKind) tea.Cmd {
	row := m.tables[kind].SelectedRow()
	if row == nil {
		return nil
	}
	name, _ := row[msgs.KeyName].(string)
	namespace, _ := row[msgs.KeyNamespace].(string)
	ctxName, _ := row[msgs.KeyContext].(string)

	// Re-entering the row already shown in the pane just refocuses it instead
	// of re-fetching — e.g. after Esc dropped back to the list to scroll/pick
	// a row, Enter on that same row jumps straight back in.
	if m.showDetail && m.deploymentDetail.Matches(kind.Kind(), name, ctxName) {
		m.detailFocused = true
		m.applyContentSizes()
		m.updateFocusStates()
		return nil
	}

	m.deploymentDetail.StartLoading(kind.Kind(), name, ctxName)
	m.showDetail = true
	m.detailFocused = true
	m.applyContentSizes()
	m.updateFocusStates()

	switch kind {
	case msgs.KindPods:
		return cmds.LoadPodDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindServices:
		return cmds.LoadServiceDetailCmd(m.Client, ctxName, namespace, name)
	default:
		return cmds.LoadDeploymentDetailCmd(m.Client, ctxName, namespace, name)
	}
}

// restartActiveTabWatch force-restarts the watch(es) for only the active
// tab's resource kind, across every selected context — the "r" key.
func (m *MainPage) restartActiveTabWatch() tea.Cmd {
	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		return nil
	}

	kind := m.activeKind()
	contexts := make([]string, 0, len(snapshot.SelectedContexts))
	for context := range snapshot.SelectedContexts {
		contexts = append(contexts, context)
		m.appState.SetLoading(kind, context, true)
	}

	cmdSequence := m.watchSup.Restart(kind, contexts)
	if len(cmdSequence) == 0 {
		return nil
	}

	m.syncContextStates()
	return tea.Batch(cmdSequence...)
}

// reRenderAgeFromWatchCaches recomputes every kind's rows from the local
// watch caches (re-running the converter/formatDuration against time.Now(),
// refreshing the Age column text) and reapplies them — purely local, zero
// API calls. Driven by RefreshTickMsg.
func (m *MainPage) reRenderAgeFromWatchCaches() {
	if !m.watchSup.Watching() {
		return
	}
	for _, kind := range m.tabs {
		m.tables[kind].SetRows(m.watchSup.Rows(kind))
	}
	m.syncContextStates()
}

// fetchServiceEndpointsIfNeeded dispatches LoadServiceEndpointsCmd for every
// selected context whose current namespace hasn't had its Endpoint IPs
// fetched yet (see watch.Supervisor.NeedsEndpoints). Called only when svc
// wide mode turns on — the Ctrl+W toggle itself already revealed the other
// new columns synchronously; this just fills in the one column that needs
// a network round trip, without blocking or repeating that round trip.
func (m *MainPage) fetchServiceEndpointsIfNeeded() tea.Cmd {
	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		return nil
	}

	var cmdSequence []tea.Cmd
	for context, namespace := range snapshot.SelectedContexts {
		if !m.watchSup.NeedsEndpoints(context, namespace) {
			continue
		}
		m.watchSup.MarkEndpointsRequested(context, namespace)
		cmdSequence = append(cmdSequence, cmds.LoadServiceEndpointsCmd(m.Client, context, namespace))
	}

	if len(cmdSequence) == 0 {
		return nil
	}
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
	pods := m.tables[msgs.KindPods]
	var rows []msgs.RowData
	if keys := pods.CheckedKeys(); len(keys) > 0 {
		for _, key := range keys {
			if row := pods.CheckedRow(key); row != nil {
				rows = append(rows, row)
			}
		}
	} else if row := pods.SelectedRow(); row != nil {
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

func (m *MainPage) View() tea.View {
	return tea.View{
		Content:   m.renderView(),
		AltScreen: true,
	}
}

func (m *MainPage) renderView() string {
	if m.width < views.MinContentWidth || m.height < views.MinHeight {
		return m.renderTooSmallOverlay()
	}

	snapshot := m.appState.Snapshot()
	r := views.Solve(m.width, m.height)
	p := styles.CatppuccinMocha()

	leftFocused := m.focus == focusLeftPane
	leftBorder, rightBorder := styles.BlurColor, styles.FocusColor
	if leftFocused {
		leftBorder, rightBorder = styles.FocusColor, styles.BlurColor
	}

	// Left box: the Context List, titled in its border.
	leftTitleStyle := lipgloss.NewStyle().Foreground(p.Overlay1).Bold(true)
	if leftFocused {
		leftTitleStyle = leftTitleStyle.Foreground(p.Mauve)
	}
	leftBox := views.TitledBox(leftTitleStyle.Render("Contexts"), m.contextList.View(), r.LeftBoxW, r.BoxH, leftBorder)

	// Right box: the active tab's content, with the tab strip in the border.
	var content string
	if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
		emptyMsg := "No contexts selected\n\nPress Tab to focus contexts\nSpace to select • Enter to load"
		empty := styles.HelpBoxStyle().Align(lipgloss.Center).Render(emptyMsg)
		content = lipgloss.Place(r.RightContentW, r.RightContentH, lipgloss.Center, lipgloss.Center, empty)
	} else {
		content = m.activeTable().View()

		// Loading indicator (inline — it's brief and doesn't break layout).
		// Only shown on the active tab's first load (no rows yet) — a
		// background refresh of already-populated data relies on the subtle
		// "⏳ N loading" status bar hint instead, so auto-refresh doesn't
		// reflow the tab.
		if m.activeTable().RowCount() == 0 && hasLoading(snapshot.LoadingStates) {
			content = m.renderLoadingIndicator(snapshot.LoadingStates) + "\n\n" + content
		}

		// The bottom pane (Detail or Logs — mutually exclusive) is
		// cross-cutting: it splits whichever top tab's content area is
		// active in two, rather than being a peer tab of its own.
		if m.showDetail || m.showLogs {
			divider := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", r.RightContentW))
			var header, body string
			if m.showDetail {
				header = m.deploymentDetail.Header(r.RightContentW)
				body = m.deploymentDetail.View()
			} else {
				header = m.podLogs.Header(r.RightContentW)
				body = m.podLogs.View()
			}
			content = lipgloss.JoinVertical(lipgloss.Left, content, divider, header, body)
		}
	}
	rightBox := views.TitledBox(m.renderTabTitle(!leftFocused), content, r.RightBoxW, r.BoxH, rightBorder)

	fullView := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox),
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
	// Every segment carries the bar's background itself — a nested style's
	// ANSI reset would otherwise punch a hole in an outer background.
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Background(p.Mantle).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Background(p.Mantle).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Background(p.Mantle).Padding(0, 1)

	selectedCtx := len(snapshot.SelectedContexts)
	errCount := len(snapshot.Errors)
	loadingCount := 0
	for _, l := range snapshot.LoadingStates {
		if l {
			loadingCount++
		}
	}
	activeTabName := m.activeKind().Title()
	activeCount := m.activeTable().RowCount()

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
	if m.activeKind() == msgs.KindPods {
		if checkedCount := len(m.tables[msgs.KindPods].CheckedKeys()); checkedCount > 0 {
			statusBits = append(statusBits, fmt.Sprintf("☑ %d checked · l: open merged · Ctrl+X: clear", checkedCount))
		}
	}
	if offset, total, ok := m.activeTable().ScrollStatus(); ok {
		statusBits = append(statusBits, fmt.Sprintf("◂ col %d/%d ▸", offset, total))
	}
	if query, matches, typing, ok := m.activeTable().FilterStatus(); ok {
		cursor := ""
		if typing {
			cursor = "_"
		}
		statusBits = append(statusBits, fmt.Sprintf("/%s%s (%d match(es))", query, cursor, matches))
	}
	if m.showDetail {
		if percent, ok := m.deploymentDetail.HScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if m.showLogs {
		if percent, ok := m.podLogs.ScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if len(statusBits) == 0 {
		statusBits = append(statusBits, "Ready")
	}
	status := rightStyle.Render(strings.Join(statusBits, "  |  "))

	// Hints are a fixed, separate element anchored to the far right
	hints := lipgloss.NewStyle().Foreground(p.Overlay1).Background(p.Mantle).Faint(true).Render("Tab:focus  [ ]:tabs  ?:help  q:quit ")

	gap := styles.StatusBar.Render("  ")
	leftMid := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, mid)
	rightSection := lipgloss.JoinHorizontal(lipgloss.Top, status, gap, hints)
	spacerWidth := m.width - lipgloss.Width(leftMid) - lipgloss.Width(rightSection)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	spacer := styles.StatusBar.Render(strings.Repeat(" ", spacerWidth))

	return leftMid + spacer + rightSection
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
		{"g / Home   G / End", "Jump to first / last row (Deployments, Pods, svc tabs)"},
		{"/", "Filter the active table by name across all rows, not just the visible ones; Enter to keep it, Esc to clear"},
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

func hasLoading(loading map[string]bool) bool {
	for _, isLoading := range loading {
		if isLoading {
			return true
		}
	}
	return false
}
