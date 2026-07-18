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
	contextList    *models.ContextsInfo
	deploymentList *models.DeploymentPage
	podList        *models.PodPage
	focus          focusTarget

	// k8s client
	Client *k8s.Client

	// UI overlays
	errorMessage string
	showHelp     bool
}

func NewMainPageModel(c *k8s.Client) *MainPage {
	ctxInfo := models.NewContextInfo(c)
	depList := models.NewDeploymentPage(c)
	pList := models.NewPodPageModel(c)
	tabs := styles.DefaultTabs
	tabs = append(tabs, "svc")

	m := &MainPage{
		Client:         c,
		appState:       state.NewAppState(),
		tabs:           tabs,
		tabContent:     "",
		contextList:    ctxInfo,
		deploymentList: depList,
		podList:        pList,
		appStateLoaded: false,
		focus:          focusLeftPane,
		errorMessage:   "",
		showHelp:       false,
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
			// Peel dismissals one at a time: inline error first, then context errors
			if m.errorMessage != "" {
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

		// Tab navigation (tabs focused)
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

		// Content keys forwarded to the active tab
		if m.appStateLoaded {
			switch m.tabs[m.activeTab] {
			case "Deployments":
				cmd := m.deploymentList.Update(msg)
				return m, cmd
			case "Pods":
				cmd := m.podList.Update(msg)
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
		m.deploymentList.SetSize(tableW, tableH)
		m.podList.SetSize(tableW, tableH)

		return m, m.contextList.Update(ctxMsg)

	case msgs.DeploymentTableMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load deployments for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoading(msg.Context, false)
			{ s := m.appState.Snapshot(); m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts) }
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
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)

		if len(snapshot.SelectedContexts) == 0 {
			m.appStateLoaded = false
			m.deploymentList.SetRows([]table.Row{})
			m.podList.SetRows([]table.Row{})
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
			cmdSequence = append(cmdSequence,
				cmds.LoadDeploymentInfoCmd(m.Client, context, namespace),
				cmds.LoadPodInfoCmd(m.Client, context, namespace),
			)
		}

		{ s := m.appState.Snapshot(); m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts) }
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
			{ s := m.appState.Snapshot(); m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts) }
			return m, nil
		}

		m.appState.SetPods(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.podList.SetRows(snapshot.Pods)
		m.contextList.SetContextStates(snapshot.LoadingStates, snapshot.Errors, snapshot.LoadedContexts)
		return m, nil

	case msgs.ErrorMsg:
		m.errorMessage = fmt.Sprintf("%s: %v", msg.Title, msg.Err)
		if msg.Context != "" {
			m.appState.SetError(msg.Context, m.errorMessage)
			{ s := m.appState.Snapshot(); m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts) }
		}
		return m, nil
	}

	// Forward non-key messages to the focused component
	if m.focus == focusTabs && m.appStateLoaded {
		switch m.tabs[m.activeTab] {
		case "Deployments":
			cmd := m.deploymentList.Update(msg)
			return m, cmd
		case "Pods":
			cmd := m.podList.Update(msg)
			return m, cmd
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
	shouldFocusDeployments := m.focus == focusTabs && m.tabs[m.activeTab] == "Deployments" && m.appStateLoaded
	m.deploymentList.SetFocused(shouldFocusDeployments)
	shouldFocusPods := m.focus == focusTabs && m.tabs[m.activeTab] == "Pods" && m.appStateLoaded
	m.podList.SetFocused(shouldFocusPods)
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
	default:
		m.tabContent = styles.HelpBoxStyle().Render(emptyMsg)
	}

	// Loading indicator (inline — it's brief and doesn't break layout)
	if hasLoading(snapshot.LoadingStates) {
		m.tabContent = m.renderLoadingIndicator(snapshot.LoadingStates) + "\n\n" + m.tabContent
	}

	tabWidth := m.width - leftPaneWidth - 8
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
	depCount := len(snapshot.Deployments)

	focusStr := "Left Pane"
	if m.focus == focusTabs {
		focusStr = "Tabs"
	}

	left := leftStyle.Render(fmt.Sprintf("Contexts: %d", selectedCtx))
	mid := midStyle.Render(fmt.Sprintf("Tab: %s | Focus: %s", m.tabs[m.activeTab], focusStr))

	// Dynamic status bits (loading / count / errors)
	var statusBits []string
	if loadingCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⏳ %d loading", loadingCount))
	} else if depCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("Deployments: %d", depCount))
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
		{"Enter", "Confirm selection & load"},
		{"Esc", "Close overlay / dismiss error"},
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
