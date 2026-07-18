package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

type PodPage struct {
	Client  *k8s.Client
	Focused bool
	table   table.Model

	// Cache for view rendering
	rows       []table.Row
	rowsSet    bool
	cachedView string
	viewDirty  bool
}

func NewPodPageModel(client *k8s.Client) *PodPage {
	return &PodPage{
		Client:    client,
		table:     table.New(table.WithColumns(podTableColumns())),
		viewDirty: true,
	}
}

func (p *PodPage) Init() tea.Cmd {
	return nil
}

func (p *PodPage) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	p.invalidateView()
	return cmd
}

func (p *PodPage) SetRows(rows []table.Row) {
	if p.rowsSet && rowsEqual(rows, p.rows) {
		return
	}

	cloned := cloneRows(rows)
	p.rows = cloned
	p.rowsSet = true
	p.table.SetRows(cloned)

	if p.Focused {
		p.table.Focus()
	} else {
		p.table.Blur()
	}

	p.invalidateView()
}

func (p *PodPage) Reset() {
	p.rows = nil
	p.rowsSet = false
	p.table.SetRows(nil)
	p.invalidateView()
}

func (p *PodPage) SetFocused(f bool) {
	p.Focused = f
	if f {
		p.table.Focus()
	} else {
		p.table.Blur()
	}
	p.invalidateView()
}

func (p *PodPage) View() string {
	if p.cachedView != "" && !p.viewDirty {
		return p.cachedView
	}

	p.table.SetStyles(styles.CatppuccinTableStyles())
	view := p.table.View()
	p.cachedView = view
	p.viewDirty = false
	return view
}

func (p *PodPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	p.table.SetHeight(h)
	// bubbles/table pads each visible column by 2 (Padding(0,1)); budget that
	// in so the rendered row never exceeds w.
	const visibleCols = 5
	avail := w - visibleCols*2
	nameW := avail * 38 / 100
	nsW := avail * 22 / 100
	statusW := avail * 15 / 100
	restartsW := avail * 12 / 100
	ageW := avail - nameW - nsW - statusW - restartsW
	p.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Namespace", Width: nsW},
		{Title: "Status", Width: statusW},
		{Title: "Restarts", Width: restartsW},
		{Title: "Age", Width: ageW},
		{Title: "Context", Width: 0},    // hidden, carries data for the detail tab
		{Title: "Containers", Width: 0}, // hidden, comma-separated container names for the log pane
	})
	p.invalidateView()
}

// SelectedRow returns the currently highlighted table row, or nil if there are no rows.
func (p *PodPage) SelectedRow() table.Row {
	return p.table.SelectedRow()
}

func (p *PodPage) invalidateView() {
	p.viewDirty = true
	p.cachedView = ""
}
