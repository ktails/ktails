// Package pages, it implements main routing to different pages.
package pages

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ktails/ktails/internal/k8s"
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
	appState       *AppState
	appStateLoaded bool

	// base models
	contextList    *models.ContextsInfo
	deploymentList *models.DeploymentPage
	podList        *models.PodPage
	focus          focusTarget

	// k8s client
	Client *k8s.Client

	// Error display
	errorMessage string
}

func NewMainPageModel(c *k8s.Client) *MainPage {
	ctxInfo := models.NewContextInfo(c)
	depList := models.NewDeploymentPage(c)
	pList := models.NewPodPageModel(c)
	tabs := styles.DefaultTabs
	tabs = append(tabs, "svc")

	m := &MainPage{
		Client:         c,
		appState:       NewAppState(),
		tabs:           tabs,
		tabContent:     "",
		contextList:    ctxInfo,
		deploymentList: depList,
		podList:        pList,
		appStateLoaded: false,
		focus:          focusLeftPane,
		errorMessage:   "",
	}

	m.updateFocusStates()
	return m
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return nil
}

func (m *MainPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keypress := msg.String()
		switch keypress {
		case "ctrl+t":
			m.toggleFocus()
			return m, nil
		case "ctrl+c", "q":
			return m, tea.Quit
		case "ctrl+e":
			// Clear error message
			m.errorMessage = ""
			return m, nil
		}

		if m.focus == focusLeftPane {
			cmd := m.contextList.Update(msg)
			return m, cmd
		}

		switch keypress {
		case "right", "tab":
			m.activeTab = min(m.activeTab+1, len(m.tabs)-1)
			m.updateFocusStates()
			return m, nil
		case "left", "shift+tab":
			m.activeTab = max(m.activeTab-1, 0)
			m.updateFocusStates()
			return m, nil
		}

		if m.tabs[m.activeTab] == "Deployments" && m.appStateLoaded {
			cmd := m.deploymentList.Update(msg)
			return m, cmd
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		windowMsg := tea.WindowSizeMsg{}
		windowMsg.Width, windowMsg.Height = getContextPaneDimensions(m.width, m.height)
		return m, m.contextList.Update(windowMsg)

	case msgs.ContextsStateMsg:
		// Clear previous errors when contexts change
		m.errorMessage = ""

		for _, contextName := range msg.Deselected {
			m.appState.RemoveContext(contextName)
		}

		for _, ms := range msg.Selected {
			m.appState.AddContext(ms.ContextName, ms.DefaultNamespace)
		}

		m.deploymentList.SetRows(m.appState.GetAllDeployments())

		if len(m.appState.SelectedContexts) == 0 {
			m.appStateLoaded = false
			m.deploymentList.SetRows([]table.Row{})
			m.updateFocusStates()
			return m, nil
		}

		// Switch to Deployments tab
		for i, t := range m.tabs {
			if t == "Deployments" {
				m.activeTab = i
				break
			}
		}

		// Load data for all selected contexts
		cmdSequence := []tea.Cmd{}
		for context, namespace := range m.appState.SelectedContexts {
			m.appState.SetLoading(context, true)
			cmdSequence = append(cmdSequence, cmds.LoadDeploymentInfoCmd(m.Client, context, namespace))
		}

		m.appStateLoaded = true
		m.updateFocusStates()
		return m, tea.Sequence(cmdSequence...)

	case msgs.DeploymentTableMsg:
		// Handle errors from deployment loading
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load deployments for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoading(msg.Context, false)
			return m, nil
		}

		// Success - update state
		m.appState.SetDeployments(msg.Context, msg.Rows)
		allRows := m.appState.GetAllDeployments()
		m.deploymentList.SetRows(allRows)
		m.updateFocusStates()
		return m, nil

	case msgs.PodTableMsg:
		// Handle errors from pod loading
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load pods for context '%s': %v", msg.Context, msg.Err)
			m.errorMessage = errMsg
			return m, nil
		}
		// Success - forward to pod list
		return m.podList.Update(msg)

	case msgs.ErrorMsg:
		// General error message
		m.errorMessage = fmt.Sprintf("%s: %v", msg.Title, msg.Err)
		if msg.Context != "" {
			m.appState.SetError(msg.Context, m.errorMessage)
		}
		return m, nil
	}

	// Forward to focused component
	if m.focus == focusTabs && m.tabs[m.activeTab] == "Deployments" && m.appStateLoaded {
		cmd := m.deploymentList.Update(msg)
		return m, cmd
	}

	if m.focus == focusLeftPane {
		cmd := m.contextList.Update(msg)
		return m, cmd
	}

	return m, nil
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
}

func (m *MainPage) View() string {

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

	switch m.tabs[m.activeTab] {
	case "Kubernetes Contexts":
		m.tabContent = m.contextList.View()
	case "Deployments":
		if !m.appStateLoaded || len(m.appState.SelectedContexts) == 0 {
			m.tabContent = "No contexts selected. Go back to 'Kubernetes Contexts' tab and select contexts."
		} else {
			m.tabContent = m.deploymentList.View()
		}
	default:
		m.tabContent = "More Info Coming Soon"
	}

	// Add error banner if there are errors
	if m.errorMessage != "" {
		errorBanner := m.renderErrorBanner()
		m.tabContent = errorBanner + "\n\n" + m.tabContent
	} else if len(m.appState.Errors) > 0 {
		// Show summary of all errors
		errorBanner := m.renderErrorSummary()
		m.tabContent = errorBanner + "\n\n" + m.tabContent
	}

	// Add loading indicator
	if m.appState.IsAnyLoading() {
		loadingMsg := m.renderLoadingIndicator()
		m.tabContent = loadingMsg + "\n\n" + m.tabContent
	}

	tabWidth := m.width - leftPaneWidth - 8
	tabHeaders := views.RenderTabHeaders(m.activeTab, m.tabs, tabWidth, tabBlur)
	tabs.WriteString(tabHeaders)
	tabs.WriteString("\n")
	tabs.WriteString(tabBottom.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Height(m.height - 8).Align(lipgloss.Left).Render(m.tabContent))

	fullView := lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, leftPane, tabs.String()), m.renderStatusBar())

	return fullView
}

// renderStatusBar
func (m *MainPage) renderStatusBar() string {
	// Palette and basic styles
	p := styles.CatppuccinMocha()
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Padding(0, 1)

	// Build dynamic status segments
	selectedCtx := len(m.appState.SelectedContexts)
	errCount := len(m.appState.Errors)
	loadingCount := 0
	for _, l := range m.appState.LoadingDeployments {
		if l {
			loadingCount++
		}
	}
	depCount := len(m.appState.GetAllDeployments())

	focusStr := "Left Pane"
	if m.focus == focusTabs {
		focusStr = "Tabs"
	}

	// Left segment
	left := leftStyle.Render(fmt.Sprintf("Contexts: %d", selectedCtx))

	// Middle segment
	mid := midStyle.Render(fmt.Sprintf("Tab: %s | Focus: %s", m.tabs[m.activeTab], focusStr))

	// Right segment
	rightBits := []string{}
	if loadingCount > 0 {
		rightBits = append(rightBits, fmt.Sprintf("⏳ %d loading", loadingCount))
	} else if depCount > 0 {
		rightBits = append(rightBits, fmt.Sprintf("Deployments: %d", depCount))
	}
	if errCount > 0 {
		rightBits = append(rightBits, fmt.Sprintf("⚠ %d error(s)", errCount))
	}
	if len(rightBits) == 0 {
		rightBits = append(rightBits, "Ready")
	}
	right := rightStyle.Render(strings.Join(rightBits, "  |  "))

	// Layout across the full bar width
	barWidth := m.width - 2
	leftMid := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", mid)
	spacerWidth := barWidth - lipgloss.Width(leftMid) - lipgloss.Width(right)
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	line := leftMid + strings.Repeat(" ", spacerWidth) + right

	return styles.StatusBar.Width(barWidth).Render(line)
}

// renderErrorBanner creates a styled error message banner
func (m *MainPage) renderErrorBanner() string {
	p := styles.CatppuccinMocha()
	errorStyle := lipgloss.NewStyle().
		Foreground(p.Red).
		Background(p.Surface0).
		Bold(true).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red)

	return errorStyle.Render("⚠ Error: " + m.errorMessage + " (Press Ctrl+E to dismiss)")
}

// renderErrorSummary creates a summary of all context errors
func (m *MainPage) renderErrorSummary() string {
	if len(m.appState.Errors) == 0 {
		return ""
	}

	p := styles.CatppuccinMocha()
	errorStyle := lipgloss.NewStyle().
		Foreground(p.Red).
		Background(p.Surface0).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red)

	var errorLines []string
	errorLines = append(errorLines, "⚠ Errors encountered:")
	for ctx, err := range m.appState.Errors {
		errorLines = append(errorLines, fmt.Sprintf("  • %s: %s", ctx, err))
	}
	errorLines = append(errorLines, "(Press Ctrl+E to dismiss)")

	return errorStyle.Render(strings.Join(errorLines, "\n"))
}

// renderLoadingIndicator shows which contexts are currently loading
func (m *MainPage) renderLoadingIndicator() string {
	p := styles.CatppuccinMocha()
	loadingStyle := lipgloss.NewStyle().
		Foreground(p.Blue).
		Background(p.Surface0).
		Padding(0, 1)

	var loadingContexts []string
	for ctx, loading := range m.appState.LoadingDeployments {
		if loading {
			loadingContexts = append(loadingContexts, ctx)
		}
	}

	if len(loadingContexts) == 0 {
		return ""
	}

	msg := fmt.Sprintf("⏳ Loading: %s...", strings.Join(loadingContexts, ", "))
	return loadingStyle.Render(msg)
}

func getContextPaneDimensions(w, h int) (cW, cH int) {
	cW = w / 4
	cH = h - 10
	return cW, cH
}

// local helpers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
