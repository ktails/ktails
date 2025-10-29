package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/cmds"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type PodPage struct {
	// Context name for this pod list
	ContextName string
	Namespace   string
	Focused     bool
	Client      *k8s.Client
	PageTitle   string
	table       table.Model
	allRows     []table.Row
}

func NewPodPageModel(client *k8s.Client) *PodPage {
	return &PodPage{
		Client: client,
		table:  table.New(table.WithColumns(podTableColumns())),
	}
}

func (p *PodPage) Init() tea.Cmd {
	return nil
}

func (p *PodPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			p.table, cmd = p.table.Update(msg)
			return p, cmd
		case "down", "j":
			p.table, cmd = p.table.Update(msg)
			return p, cmd
		}
	case msgs.ContextsSelectedMsg:
		p.ContextName = msg.ContextName
		p.Namespace = msg.DefaultNamespace
		return p, p.loadPods()
	case msgs.PodTableMsg:
		p.allRows = append(p.allRows, msg.Rows...)
		p.handlePodTableMsg(p.allRows)

		return p, nil
	}
	return p, nil
}

func (p *PodPage) handlePodTableMsg(rows []table.Row) {
	p.table.Focus()
	p.table.SetRows(rows)
}

func (p *PodPage) loadPods() tea.Cmd {
	return cmds.LoadPodInfoCmd(p.Client, p.ContextName, p.Namespace)
}

func (p *PodPage) View() string {
	p.table.SetStyles(styles.CatppuccinTableStyles())
	return p.table.View()
}
