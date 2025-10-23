// Package pages, it implements maing routing tp different pages.
package pages

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/styles"
)

type MainPage struct {
	// dimensions
	width          int
	height         int
	appState       *AppState
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
	return &MainPage{
		appState:       new(AppState),
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

		cmd = m.contextList.Update(msg)
		return m, cmd

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
	style := styles.ListPaneStyle()
	// style.Width(m.width)
	// style.Height(m.height)
	headerStyle := "Header"
	footerStyle := "Footer"
	verticalJoin := lipgloss.JoinVertical(lipgloss.Left, headerStyle, style.Render(m.contextList.View()), footerStyle)
	// finalView := style.Render(m.contextList.View())
	return verticalJoin
}

func getContextPaneDimensions(w, h int) (cW, cH int) {
	cW = w / 4
	cH = h - 10
	return cW, cH
}
