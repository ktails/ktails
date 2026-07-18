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
	focused    bool
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
	if d.focused {
		d.table.Focus()
	} else {
		d.table.Blur()
	}
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

// SelectedRow returns the currently highlighted table row, or nil if there are no rows.
func (d *DeploymentPage) SelectedRow() table.Row {
	return d.table.SelectedRow()
}

func (d *DeploymentPage) SetFocused(f bool) {
	d.focused = f
	if f {
		d.table.Focus()
	} else {
		d.table.Blur()
	}
	d.invalidateView()
}

func (d *DeploymentPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	d.table.SetHeight(h)
	avail := w - 4
	nameW := avail * 40 / 100
	ageW := avail * 15 / 100
	replicasW := avail * 22 / 100
	ctxW := avail - nameW - ageW - replicasW
	d.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Age", Width: ageW},
		{Title: "ReadyReplicas", Width: replicasW},
		{Title: "Context", Width: ctxW},
		{Title: "Namespace", Width: 0}, // hidden, carries data for the detail panel
	})
	d.invalidateView()
}

func (d *DeploymentPage) invalidateView() {
	d.viewDirty = true
	d.cachedView = ""
}
