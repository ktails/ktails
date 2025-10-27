// Package pages, it implements maing routing tp different pages.
package pages

import (
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

	// tabs, basic implementation from bubbletea e.g. https://github.com/charmbracelet/bubbletea/blob/main/examples/tabs/main.go
	tabs       []string
	tabContent string
	activeTab  int

	// future App state
	appState       *AppState
	appStateLoaded bool
	// base models
	contextList    *models.ContextsInfo
	deploymentList *models.DeploymentPage
	podList        *models.PodPage
	focus          focusTarget
	// k8s client
	Client *k8s.Client
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

		for i, t := range m.tabs {
			if t == "Deployments" {
				m.activeTab = i
				break
			}
		}

		cmdSequence := []tea.Cmd{}
		for context, namespace := range m.appState.SelectedContexts {
			m.appState.SetLoading(context, true)
			cmdSequence = append(cmdSequence, cmds.LoadDeploymentInfoCmd(m.Client, context, namespace))
		}

		m.appStateLoaded = true
		m.updateFocusStates()
		return m, tea.Sequence(cmdSequence...)

	case msgs.DeploymentTableMsg:
		m.appState.SetDeployments(msg.Context, msg.Rows)
		allRows := m.appState.GetAllDeployments()
		m.deploymentList.SetRows(allRows)
		m.updateFocusStates()
		return m, nil
	}

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

	switch m.focus {
	case focusLeftPane:
		leftPane = views.RenderLeftPane(m.contextList.View(), leftPaneWidth, m.height-10)
		tabBlur = true

	case focusTabs:
		leftPane = views.RenderLeftPaneBlur(m.contextList.View(), leftPaneWidth, m.height-10)
		tabBlur = false
	}

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

	tabWidth := m.width - leftPaneWidth - 10

	tabHeaders := views.RenderTabHeaders(m.activeTab, m.tabs, tabWidth, tabBlur)
	tabs.WriteString(tabHeaders)
	tabs.WriteString("\n")
	tabs.WriteString(styles.WindowStyle.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Height(m.height - 8).Align(lipgloss.Left).Render(m.tabContent))
	fullView := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, tabs.String())
	return fullView
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
