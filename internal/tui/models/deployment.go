// Package models
package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/cmds"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/termkit/skeleton"
)

type DeploymentPage struct {
	Skel   *skeleton.Skeleton
	Client *k8s.Client
	table  table.Model
	// share contextList
	ContextName string
	Namespace   string
	// allRows
	allRows []table.Row
}

func NewDeploymentPage(s *skeleton.Skeleton, client *k8s.Client) *DeploymentPage {
	return &DeploymentPage{
		Skel:   s,
		Client: client,
		table:  table.New(table.WithColumns(deploymentTableColumns())),
	}
}

func (d *DeploymentPage) Init() tea.Cmd {
	return nil
}

func (d *DeploymentPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "j":
			d.table, cmd = d.table.Update(msg)
			return d, cmd
		case "down", "k":
			d.table, cmd = d.table.Update(msg)
			return d, cmd
		}
	// case msgs.ResetPodTableMsg:
	// 	p.allRows = []table.Row{}
	// 	return p, nil
	case msgs.ContextsSelectedMsg:
		d.ContextName = msg.ContextName
		d.Namespace = msg.DefaultNamespace
		return d, d.loadDeployments()
	case msgs.DeploymentTableMsg:
		d.allRows = append(d.allRows, msg.Rows...)
		d.handleDeploymentTableMsg(d.allRows)

		return d, nil
	}
	return d, nil
}

func (d *DeploymentPage) handleDeploymentTableMsg(rows []table.Row) {
	d.table.Focus()
	d.table.SetRows(rows)
}

func (d *DeploymentPage) loadDeployments() tea.Cmd {
	d.Skel.TriggerUpdate()
	return cmds.LoadDeploymentInfoCmd(d.Client, d.ContextName, d.Namespace)
}

func (d *DeploymentPage) View() string {
	return d.table.View()
}
