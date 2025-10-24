// Package pages, it implements maing routing tp different pages.
package pages

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/styles"
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
	appState *AppState

	// base models
	contextList    *models.ContextsInfo
	deploymentList *models.DeploymentPage
	podList        *models.PodPage
	// k8s client
	Client *k8s.Client
}

func NewMainPageModel(c *k8s.Client) *MainPage {
	ctxInfo := models.NewContextInfo(c)
	depList := models.NewDeploymentPage(c)
	pList := models.NewPodPageModel(c)
	tabs := styles.DefaultTabs
	tabs = append(tabs, "svc")
	tabContent := ""
	return &MainPage{
		appState:       new(AppState),
		tabs:           tabs,
		tabContent:     tabContent,
		contextList:    ctxInfo,
		deploymentList: depList,
		podList:        pList,
	}
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return nil
}

func (m *MainPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "right", "tab":
			m.activeTab = min(m.activeTab+1, len(m.tabs)-1)
			return m, nil
		case "left", "shift+tab":
			m.activeTab = max(m.activeTab-1, 0)
			return m, nil
		default:
			cmd = m.contextList.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		windowMsg := tea.WindowSizeMsg{}
		windowMsg.Width, windowMsg.Height = getContextPaneDimensions(m.width, m.height)
		return m, m.contextList.Update(windowMsg)
	default:
		cmd = m.contextList.Update(msg)

		return m, cmd
	}
}

func (m *MainPage) View() string {
	// docStyle := styles.DocStyle()
	// // docStyle.MarginBackground(styles.CatppuccinMocha().Crust)

	tabs := strings.Builder{}
	switch m.tabs[m.activeTab] {
	case "Kubernetes Contexts":
		m.tabContent = m.contextList.View()
	default:
		m.tabContent = "More Info Coming Soon"
	}

	tabHeaders := styles.RenderTabHeaders(m.activeTab, m.tabs, m.width-10, m.height-10)
	tabs.WriteString(tabHeaders)
	tabs.WriteString("\n")
	tabs.WriteString(styles.WindowStyle.Width(lipgloss.Width(tabHeaders) - styles.WindowStyle.GetHorizontalFrameSize()).Render(m.tabContent))

	return tabs.String()
}

func getContextPaneDimensions(w, h int) (cW, cH int) {
	cW = w / 4
	cH = h - 10
	return cW, cH
}
