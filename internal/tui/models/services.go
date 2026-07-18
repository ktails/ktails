package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

type ServicePage struct {
	Client  *k8s.Client
	Focused bool
	table   table.Model

	rows       []table.Row
	rowsSet    bool
	cachedView string
	viewDirty  bool
}

func NewServicePageModel(client *k8s.Client) *ServicePage {
	return &ServicePage{
		Client:    client,
		table:     table.New(table.WithColumns(svcTableColumns())),
		viewDirty: true,
	}
}

func (s *ServicePage) Init() tea.Cmd {
	return nil
}

func (s *ServicePage) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	s.invalidateView()
	return cmd
}

func (s *ServicePage) SetRows(rows []table.Row) {
	if s.rowsSet && rowsEqual(rows, s.rows) {
		return
	}

	cloned := cloneRows(rows)
	s.rows = cloned
	s.rowsSet = true
	s.table.SetRows(cloned)

	if s.Focused {
		s.table.Focus()
	} else {
		s.table.Blur()
	}

	s.invalidateView()
}

// SelectedRow returns the currently highlighted table row, or nil if there are no rows.
func (s *ServicePage) SelectedRow() table.Row {
	return s.table.SelectedRow()
}

func (s *ServicePage) SetFocused(f bool) {
	s.Focused = f
	if f {
		s.table.Focus()
	} else {
		s.table.Blur()
	}
	s.invalidateView()
}

func (s *ServicePage) View() string {
	if s.cachedView != "" && !s.viewDirty {
		return s.cachedView
	}

	s.table.SetStyles(styles.CatppuccinTableStyles())
	view := s.table.View()
	s.cachedView = view
	s.viewDirty = false
	return view
}

func (s *ServicePage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	s.table.SetHeight(h)
	avail := w - 4
	nameW := avail * 28 / 100
	nsW := avail * 18 / 100
	typeW := avail * 14 / 100
	ipW := avail * 18 / 100
	portsW := avail * 12 / 100
	ageW := avail - nameW - nsW - typeW - ipW - portsW
	s.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Namespace", Width: nsW},
		{Title: "Type", Width: typeW},
		{Title: "ClusterIP", Width: ipW},
		{Title: "Ports", Width: portsW},
		{Title: "Age", Width: ageW},
		{Title: "Context", Width: 0}, // hidden, carries data for the detail tab
	})
	s.invalidateView()
}

func (s *ServicePage) invalidateView() {
	s.viewDirty = true
	s.cachedView = ""
}
