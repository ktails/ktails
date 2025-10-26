// Package models
package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/termkit/skeleton"
)

type DeploymentPage struct {
	Skel   *skeleton.Skeleton
	Client *k8s.Client
	table  table.Model
	// share contextList
	ContextName string
	Namespace   string
}

func NewDeploymentPage(client *k8s.Client) *DeploymentPage {
	return &DeploymentPage{
		Client: client,
		table:  table.New(table.WithColumns(deploymentTableColumns())),
	}
}

func (d *DeploymentPage) Init() tea.Cmd {
	return nil
}

func (d *DeploymentPage) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			d.table, cmd = d.table.Update(msg)
			return cmd
		case "down", "j":
			d.table, cmd = d.table.Update(msg)
			return cmd
		}
	}
	d.table, cmd = d.table.Update(msg)
	return cmd
}

func (d *DeploymentPage) SetRows(rows []table.Row) {
	d.table.Focus()
	d.table.SetRows(rows)
}

func (d *DeploymentPage) View() string {
	d.table.SetStyles(styles.CatppuccinTableStyles())
	return d.table.View()
}
