package models

import (
	tea "github.com/charmbracelet/bubbletea"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type ServicePage struct {
	Client  *k8s.Client
	Focused bool
	table   btable.Model

	rows       []msgs.RowData
	rowsSet    bool
	cachedView string
	viewDirty  bool

	wideMode     bool
	tableW       int
	tableH       int
	wideColCount int
	scrollable   bool
}

func NewServicePageModel(client *k8s.Client) *ServicePage {
	return &ServicePage{
		Client:    client,
		table:     newBubbleTable(svcNarrowColumns()),
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

func (s *ServicePage) SetRows(rows []msgs.RowData) {
	if s.rowsSet && rowsEqual(rows, s.rows) {
		return
	}

	s.rows = cloneRows(rows)
	s.rowsSet = true
	s.applyColumns()
	s.pushDisplayRows()

	if s.Focused {
		s.table = s.table.Focused(true)
	} else {
		s.table = s.table.Focused(false)
	}

	s.invalidateView()
}

func (s *ServicePage) pushDisplayRows() {
	display := make([]btable.Row, len(s.rows))
	for i, row := range s.rows {
		display[i] = btable.NewRow(btable.RowData{
			msgs.SvcKeyName:      row[msgs.SvcKeyName],
			msgs.SvcKeyNamespace: row[msgs.SvcKeyNamespace],
			msgs.SvcKeyType:      row[msgs.SvcKeyType],
			msgs.SvcKeyClusterIP: row[msgs.SvcKeyClusterIP],
			msgs.SvcKeyPorts:     row[msgs.SvcKeyPorts],
			msgs.SvcKeyAge:       row[msgs.SvcKeyAge],
			msgs.SvcKeyContext:   row[msgs.SvcKeyContext],
		})
	}
	s.table = s.table.WithRows(display)
}

// applyColumns rebuilds the column set for the current mode (narrow/wide),
// auto-fitting wide-mode widths to s.rows — called on every SetRows/ToggleWideMode.
func (s *ServicePage) applyColumns() {
	var cols []btable.Column
	if s.wideMode {
		cols = svcWideColumns(s.rows)
	} else {
		cols = svcNarrowColumns()
	}
	s.wideColCount = len(cols)
	s.scrollable = s.wideMode && totalColumnsWidth(cols) > s.tableW
	s.table = s.table.WithColumns(cols)
	// See PodPage.applyColumns: WithTargetWidth must be cleared in wide mode
	// or bubble-table forces totalWidth to it, silently disabling scroll.
	if s.wideMode {
		s.table = s.table.WithTargetWidth(0).WithMaxTotalWidth(s.tableW)
	} else {
		s.table = s.table.WithTargetWidth(s.tableW).WithMaxTotalWidth(s.tableW)
	}
}

// ToggleWideMode flips wide mode for this tab (sticky until the next
// resize) and rebuilds columns to fit the current data.
func (s *ServicePage) ToggleWideMode() {
	s.wideMode = !s.wideMode
	s.applyColumns()
	s.pushDisplayRows()
	s.invalidateView()
}

func (s *ServicePage) WideMode() bool {
	return s.wideMode
}

// ScrollStatus reports the current horizontal scroll position for the
// status bar's "◂ col N/M ▸" indicator. ok is false when the indicator
// should be hidden.
func (s *ServicePage) ScrollStatus() (offset, total int, ok bool) {
	if !s.wideMode || !s.scrollable {
		return 0, 0, false
	}
	return s.table.GetHorizontalScrollColumnOffset() + 1, s.wideColCount, true
}

func (s *ServicePage) ScrollLeft() {
	s.table = s.table.ScrollLeft()
	s.invalidateView()
}

func (s *ServicePage) ScrollRight() {
	s.table = s.table.ScrollRight()
	s.invalidateView()
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows.
func (s *ServicePage) SelectedRow() msgs.RowData {
	idx := s.table.GetHighlightedRowIndex()
	if idx < 0 || idx >= len(s.rows) {
		return nil
	}
	return s.rows[idx]
}

func (s *ServicePage) SetFocused(f bool) {
	s.Focused = f
	s.table = s.table.Focused(f)
	s.invalidateView()
}

func (s *ServicePage) View() string {
	if s.cachedView != "" && !s.viewDirty {
		return s.cachedView
	}

	view := s.table.View()
	s.cachedView = view
	s.viewDirty = false
	return view
}

func (s *ServicePage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	prevIdx := s.table.GetHighlightedRowIndex()
	s.tableW, s.tableH = w, h
	s.wideMode = false

	st := styles.CatppuccinBubbleTableStyle()
	s.table = newBubbleTable(svcNarrowColumns()).
		WithMinimumHeight(h).
		WithTargetWidth(w).
		WithMaxTotalWidth(w).
		HeaderStyle(st.Header).
		HighlightStyle(st.Highlight).
		WithBaseStyle(st.Base).
		Focused(s.Focused)
	s.wideColCount = len(svcNarrowColumns())
	s.scrollable = false
	s.pushDisplayRows()
	s.table = s.table.WithHighlightedRow(prevIdx)
	s.invalidateView()
}

func (s *ServicePage) invalidateView() {
	s.viewDirty = true
	s.cachedView = ""
}
