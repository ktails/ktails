// Package models
package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

type DeploymentPage struct {
	Client *k8s.Client
	table  table.Model
	// share contextList
	ContextName string
	Namespace   string

	rows       []table.Row
	rowsSet    bool
	cachedView string
	viewDirty  bool
}

func NewDeploymentPage(client *k8s.Client) *DeploymentPage {
	return &DeploymentPage{
		Client:    client,
		table:     table.New(table.WithColumns(deploymentTableColumns())),
		viewDirty: true,
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
			d.invalidateView()
			return cmd
		case "down", "j":
			d.table, cmd = d.table.Update(msg)
			d.invalidateView()
			return cmd
		}
	}
	d.table, cmd = d.table.Update(msg)
	d.invalidateView()
	return cmd
}

func (d *DeploymentPage) SetRows(rows []table.Row) {
	if d.rowsSet && rowsEqual(rows, d.rows) {
		return
	}

	cloned := cloneRows(rows)
	d.rows = cloned
	d.rowsSet = true
	d.table.SetRows(cloned)
	d.table.Focus()
	d.invalidateView()
}

func (d *DeploymentPage) View() string {
	if d.cachedView != "" && !d.viewDirty {
		return d.cachedView
	}

	d.table.SetStyles(styles.CatppuccinTableStyles())
	view := d.table.View()
	d.cachedView = view
	d.viewDirty = false
	return view
}

func (d *DeploymentPage) SetFocused(f bool) {
	if f {
		d.table.Focus()
	} else {
		d.table.Blur()
	}
	d.invalidateView()
}

func (d *DeploymentPage) invalidateView() {
	d.viewDirty = true
	d.cachedView = ""
}

func rowsEqual(a, b []table.Row) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}

		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}

	return true
}

func cloneRows(rows []table.Row) []table.Row {
	if len(rows) == 0 {
		return make([]table.Row, 0)
	}

	cloned := make([]table.Row, len(rows))
	for i := range rows {
		if len(rows[i]) == 0 {
			cloned[i] = nil
			continue
		}

		cells := make(table.Row, len(rows[i]))
		copy(cells, rows[i])
		cloned[i] = cells
	}

	return cloned
}
