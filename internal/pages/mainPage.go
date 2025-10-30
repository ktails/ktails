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
		appState:       state.NewAppState(),
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
	start := time.Now()
	defer m.logSlowUpdate(start)

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
		case "left", "shift+tab":
			prev := m.activeTab - 1
			if prev < 0 {
				return m, nil
			}

			m.activeTab = prev
			m.updateFocusStates()
			return m, nil
		}

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

		windowMsg := tea.WindowSizeMsg{}
		windowMsg.Width, windowMsg.Height = getContextPaneDimensions(m.width, m.height)
		return m, m.contextList.Update(windowMsg)

	case msgs.DeploymentTableMsg:
		// Handle errors from deployment loading
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load deployments for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoading(msg.Context, false)
			m.contextList.SetLoadingStates(m.appState.Snapshot().LoadingStates)
			return m, nil
		}

		// Success - update state
		m.appState.SetDeployments(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.deploymentList.SetRows(snapshot.Deployments)
		m.contextList.SetLoadingStates(snapshot.LoadingStates)
		m.updateFocusStates()
		return m, nil

	case msgs.ContextsStateMsg:
		// Clear previous errors when contexts change
		m.errorMessage = ""

		for _, contextName := range msg.Deselected {
			m.appState.RemoveContext(contextName)
		}

		for _, ms := range msg.Selected {
			m.appState.AddContext(ms.ContextName, ms.DefaultNamespace)
		}

		snapshot := m.appState.Snapshot()
		m.deploymentList.SetRows(snapshot.Deployments)
		m.podList.SetRows(snapshot.Pods) // Now we track pods in state too
		m.contextList.SetLoadingStates(snapshot.LoadingStates)

		if len(snapshot.SelectedContexts) == 0 {
			m.appStateLoaded = false
			m.deploymentList.SetRows([]table.Row{})
			m.podList.SetRows([]table.Row{})
			m.contextList.SetLoadingStates(nil)
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
		for context, namespace := range snapshot.SelectedContexts {
			m.appState.SetLoading(context, true)
			m.appState.SetLoadingPods(context, true)
			cmdSequence = append(cmdSequence,
				cmds.LoadDeploymentInfoCmd(m.Client, context, namespace),
				cmds.LoadPodInfoCmd(m.Client, context, namespace),
			)
		}

		m.contextList.SetLoadingStates(m.appState.Snapshot().LoadingStates)
		m.appStateLoaded = true
		m.updateFocusStates()

		if len(cmdSequence) == 0 {
			return m, nil
		}

		return m, tea.Batch(cmdSequence...)

	case msgs.PodTableMsg:
		// Handle errors from pod loading
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Failed to load pods for context '%s': %v", msg.Context, msg.Err)
			m.appState.SetError(msg.Context, errMsg)
			m.errorMessage = errMsg
			m.appState.SetLoadingPods(msg.Context, false)
			m.contextList.SetLoadingStates(m.appState.Snapshot().LoadingStates)
			return m, nil
		}

		// Success - update state
		m.appState.SetPods(msg.Context, msg.Rows)
		snapshot := m.appState.Snapshot()
		m.podList.SetRows(snapshot.Pods)
		m.contextList.SetLoadingStates(snapshot.LoadingStates)
		return m, nil

	case msgs.ErrorMsg:
		// General error message
		m.errorMessage = fmt.Sprintf("%s: %v", msg.Title, msg.Err)
		if msg.Context != "" {
			m.appState.SetError(msg.Context, m.errorMessage)
			m.contextList.SetLoadingStates(m.appState.Snapshot().LoadingStates)
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

	switch m.tabs[m.activeTab] {
	case "Kubernetes Contexts":
		m.tabContent = m.contextList.View()
	case "Deployments":
		if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
			m.tabContent = styles.HelpBoxStyle().Render(
				"No contexts selected\n\n" +
					"Press Ctrl+T to focus contexts\n" +
					"Space to select • Enter to load")

		} else {
			m.tabContent = m.deploymentList.View()
		}
	case "Pods":
		if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
			m.tabContent = styles.HelpBoxStyle().Align(lipgloss.Center).Render(
				"No contexts selected\n\n" +
					"Press Ctrl+T to focus contexts\n" +
					"Space to select • Enter to load")

		} else {
			m.tabContent = m.podList.View()
		}
	default:
		m.tabContent = styles.HelpBoxStyle().Render(
			"No contexts selected\n\n" +
				"Press Ctrl+T to focus contexts\n" +
				"Space to select • Enter to load")
	}

	// Add error banner if there are errors
	if m.errorMessage != "" {
		errorBanner := m.renderErrorBanner()
		m.tabContent = errorBanner + "\n\n" + m.tabContent
	} else if len(snapshot.Errors) > 0 {
		// Show summary of all errors
		errorBanner := m.renderErrorSummary(snapshot.Errors)
		m.tabContent = errorBanner + "\n\n" + m.tabContent
	}

	// Add loading indicator
	if hasLoading(snapshot.LoadingStates) {
		loadingMsg := m.renderLoadingIndicator(snapshot.LoadingStates)
		m.tabContent = loadingMsg + "\n\n" + m.tabContent
	}

	tabWidth := m.width - leftPaneWidth - 8
	tabHeaders := views.RenderTabHeaders(m.activeTab, m.tabs, tabWidth, tabBlur)
	tabs.WriteString(tabHeaders)
	tabs.WriteString("\n")
	tabs.WriteString(tabBottom.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Height(m.height - 8).Align(lipgloss.Center).Render(m.tabContent))

	fullView := lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, leftPane, tabs.String()), m.renderStatusBar(snapshot))

	return fullView
}

// renderStatusBar
func (m *MainPage) renderStatusBar(snapshot state.Snapshot) string {
	// Palette and basic styles
	p := styles.CatppuccinMocha()
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Padding(0, 1)

	// Build dynamic status segments
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
func (m *MainPage) renderErrorSummary(errors map[string]string) string {
	if len(errors) == 0 {
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
	for ctx, err := range errors {
		errorLines = append(errorLines, fmt.Sprintf("  • %s: %s", ctx, err))
	}
	errorLines = append(errorLines, "(Press Ctrl+E to dismiss)")

	return errorStyle.Render(strings.Join(errorLines, "\n"))
}

// renderLoadingIndicator shows which contexts are currently loading
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

	msg := fmt.Sprintf("⏳ Loading: %s...", strings.Join(loadingContexts, ", "))
	return loadingStyle.Render(msg)
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
