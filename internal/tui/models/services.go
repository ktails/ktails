package models

import (
	tea "charm.land/bubbletea/v2"
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

	// cursorIdx/windowStart/windowSize: see the identical fields on PodPage
	// in pods.go.
	cursorIdx   int
	windowStart int
	windowSize  int
}

func NewServicePageModel(client *k8s.Client) *ServicePage {
	return &ServicePage{
		Client:     client,
		table:      newBubbleTable(svcNarrowColumns()),
		viewDirty:  true,
		windowSize: defaultRowWindowSize,
	}
}

func (s *ServicePage) Init() tea.Cmd {
	return nil
}

func (s *ServicePage) Update(msg tea.Msg) tea.Cmd {
	if s.Focused {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "down", "j":
				s.moveCursor(1)
				return nil
			case "up", "k":
				s.moveCursor(-1)
				return nil
			}
		}
	}

	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	s.invalidateView()
	return cmd
}

// moveCursor: see PodPage.moveCursor in pods.go.
func (s *ServicePage) moveCursor(delta int) {
	if len(s.rows) == 0 {
		return
	}

	s.cursorIdx += delta
	if s.cursorIdx < 0 {
		s.cursorIdx = len(s.rows) - 1
	} else if s.cursorIdx >= len(s.rows) {
		s.cursorIdx = 0
	}

	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, len(s.rows), s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

func (s *ServicePage) SetRows(rows []msgs.RowData) {
	if s.rowsSet && rowsEqual(rows, s.rows) {
		return
	}

	s.rows = cloneRows(rows)
	s.rowsSet = true
	if s.cursorIdx >= len(s.rows) {
		s.cursorIdx = max(len(s.rows)-1, 0)
	}
	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, len(s.rows), s.windowSize)
	s.applyColumns()
	s.pushDisplayRows()

	if s.Focused {
		s.table = s.table.Focused(true)
	} else {
		s.table = s.table.Focused(false)
	}

	s.invalidateView()
}

// pushDisplayRows rebuilds the table's rows from the current row window
// (see windowBounds in table.go) into s.rows. Called whenever raw rows or
// the cursor/window change.
func (s *ServicePage) pushDisplayRows() {
	start, end := windowBounds(s.windowStart, len(s.rows), s.windowSize)
	display := make([]btable.Row, 0, end-start)
	for _, row := range s.rows[start:end] {
		display = append(display, btable.NewRow(btable.RowData{
			msgs.SvcKeyName:        row[msgs.SvcKeyName],
			msgs.SvcKeyNamespace:   row[msgs.SvcKeyNamespace],
			msgs.SvcKeyType:        row[msgs.SvcKeyType],
			msgs.SvcKeyClusterIP:   row[msgs.SvcKeyClusterIP],
			msgs.SvcKeyPorts:       row[msgs.SvcKeyPorts],
			msgs.SvcKeyAge:         row[msgs.SvcKeyAge],
			msgs.SvcKeyContext:     row[msgs.SvcKeyContext],
			msgs.SvcKeySelector:    row[msgs.SvcKeySelector],
			msgs.SvcKeyExternalIP:  row[msgs.SvcKeyExternalIP],
			msgs.SvcKeyEndpointIPs: row[msgs.SvcKeyEndpointIPs],
		}))
	}
	s.table = s.table.WithRows(display).WithHighlightedRow(s.cursorIdx - start)
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
// status bar's "< col N/M >" indicator. ok is false when the indicator
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
	if s.cursorIdx < 0 || s.cursorIdx >= len(s.rows) {
		return nil
	}
	return s.rows[s.cursorIdx]
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
	s.windowSize = rowWindowSizeFor(h)
	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, len(s.rows), s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

func (s *ServicePage) invalidateView() {
	s.viewDirty = true
	s.cachedView = ""
}
