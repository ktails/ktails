// Package pages, it implements main routing to different pages.
package pages

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Detail pane — a cross-cutting bottom split opened by Enter from
	// Deployments or Pods. It's not a peer tab: it stays put, splitting
	// whichever top tab's content area you're currently on.
	showDetail    bool
	detailFocused bool

	tableW, tableH int
}

func NewMainPageModel(c *k8s.Client) *MainPage {
	ctxInfo := models.NewContextInfo(c)
	depList := models.NewDeploymentPage(c)
	pList := models.NewPodPageModel(c)
	svcList := models.NewServicePageModel(c)
	detailPage := models.NewResourceDetailPage()
	tabs := styles.DefaultTabs
	tabs = append(tabs, "svc")

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
		appStateLoaded:   false,
		focus:            focusLeftPane,
		errorMessage:     "",
		showHelp:         false,
	}

	m.updateFocusStates()
	return m
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return nil
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
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "esc":
			// Peel dismissals one at a time: unfocus the detail pane, then close
			// it, then inline error, then context errors.
			if m.detailFocused {
				m.detailFocused = false
				m.updateFocusStates()
			} else if m.showDetail {
				m.showDetail = false
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
			if nextTab == "Deployments" {
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
		// pane for that row and gives it keyboard focus for scrolling.
		if m.appStateLoaded && keypress == "enter" &&
			(m.tabs[m.activeTab] == "Deployments" || m.tabs[m.activeTab] == "Pods" || m.tabs[m.activeTab] == "svc") {
			if cmd := m.openResourceDetail(m.tabs[m.activeTab]); cmd != nil {
				return m, cmd
			}
			return m, nil
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

		leftW := m.width / 3
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
			m.deploymentList.SetRows([]table.Row{})
			m.podList.SetRows([]table.Row{})
			m.svcList.SetRows([]table.Row{})
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
	listActive := m.focus == focusTabs && !m.detailFocused
	shouldFocusDeployments := listActive && m.tabs[m.activeTab] == "Deployments" && m.appStateLoaded
	m.deploymentList.SetFocused(shouldFocusDeployments)
	shouldFocusPods := listActive && m.tabs[m.activeTab] == "Pods" && m.appStateLoaded
	m.podList.SetFocused(shouldFocusPods)
	shouldFocusSvc := listActive && m.tabs[m.activeTab] == "svc" && m.appStateLoaded
	m.svcList.SetFocused(shouldFocusSvc)
	m.deploymentDetail.SetFocused(m.focus == focusTabs && m.detailFocused)
}

// detailPaneHeightPercent is the share of the tab content area given to the
// detail pane, out of the remainder after list rows.
const detailPaneHeightPercent = 45

// applyContentSizes resizes the Deployments/Pods lists and the detail pane to
// split the tab content area in two whenever the detail pane is open.
func (m *MainPage) applyContentSizes() {
	listH := m.tableH
	detailH := 0
	if m.showDetail {
		detailH = m.tableH * detailPaneHeightPercent / 100
		if detailH < 6 {
			detailH = 6
		}
		listH = m.tableH - detailH - 1 // 1 line reserved for the divider/banner
		if listH < 3 {
			listH = 3
		}
	}
	m.deploymentList.SetSize(m.tableW, listH)
	m.podList.SetSize(m.tableW, listH)
	m.svcList.SetSize(m.tableW, listH)
	m.deploymentDetail.SetSize(m.tableW, detailH)
}

// openResourceDetail loads detail for the currently selected row on the given
// top tab ("Deployments", "Pods", or "svc") into the shared bottom detail pane.
// Returns nil if there's no valid selection.
func (m *MainPage) openResourceDetail(sourceTab string) tea.Cmd {
	var kind, name, ctxName, namespace string

	switch sourceTab {
	case "Deployments":
		row := m.deploymentList.SelectedRow()
		if len(row) < 5 {
			return nil
		}
		kind, name, ctxName, namespace = "Deployment", row[0], row[3], row[4]
	case "Pods":
		row := m.podList.SelectedRow()
		if len(row) < 6 {
			return nil
		}
		kind, name, namespace, ctxName = "Pod", row[0], row[1], row[5]
	case "svc":
		row := m.svcList.SelectedRow()
		if len(row) < 7 {
			return nil
		}
		kind, name, namespace, ctxName = "Service", row[0], row[1], row[6]
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

func (m *MainPage) View() string {
	snapshot := m.appState.Snapshot()

	leftPaneWidth := m.width / 3
	leftPane := ""
	tabBlur := false
	tabBottom := styles.WindowStyle

	switch m.focus {
	case focusLeftPane:
		leftPane = views.RenderLeftPane(m.contextList.View(), leftPaneWidth, m.height-10)
		tabBlur = true
		tabBottom = styles.WindowBlurStyle

	case focusTabs:
		leftPane = views.RenderLeftPaneBlur(m.contextList.View(), leftPaneWidth, m.height-10)
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

	// The detail pane is cross-cutting: it splits whichever top tab's content
	// area is active in two, rather than being a peer tab of its own.
	if m.showDetail {
		p := styles.CatppuccinMocha()
		dividerW := m.tableW
		if dividerW < 1 {
			dividerW = 1
		}
		divider := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", dividerW))
		joined := lipgloss.JoinVertical(lipgloss.Left,
			m.tabContent,
			divider,
			m.deploymentDetail.Header(),
			m.deploymentDetail.View(),
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
		{"Space", "Toggle context selection"},
		{"Enter", "Confirm selection & load / open + focus detail pane (refocuses instantly if already loaded)"},
		{"Ctrl+R", "Jump back into an open detail pane without changing its resource"},
		{"↑/↓ j/k PgUp/PgDn", "Scroll detail pane (while it has focus)"},
		{"Home / End", "Jump to top / bottom of detail pane"},
		{"Esc", "Unfocus detail pane, then close it / overlay / dismiss error"},
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
		if pad := width - lipgloss.Width(line); pad > 0 {
			lines[i] = line + strings.Repeat(" ", pad)
		}
	}
	return strings.Join(lines, "\n")
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
	cW = w / 4
	cH = h - 10
	return cW, cH
}
