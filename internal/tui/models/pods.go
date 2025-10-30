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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			p.table, cmd = p.table.Update(msg)
			p.invalidateView()
			return cmd
		case "down", "j":
			p.table, cmd = p.table.Update(msg)
			p.invalidateView()
			return cmd
		}
	}
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

func (p *PodPage) invalidateView() {
	p.viewDirty = true
	p.cachedView = ""
}

