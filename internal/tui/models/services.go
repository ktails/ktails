package models

import (
	"strings"

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

	// filter is a k9s-style "/" filter over the Name column — see rowFilter
	// in table.go for why this exists instead of bubble-table's own filter.
	filter rowFilter

	// cursorIdx/windowStart/windowSize: see the identical fields on PodPage
	// in pods.go. cursorIdx is a position in the active index space (see
	// activeLen/activeRow), not a raw index into s.rows.
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
			if s.filter.filtering {
				s.filter.handleKey(key, len(s.rows), s.filterMatch)
				s.afterFilterChange()
				return nil
			}
			switch key.String() {
			case "down", "j":
				s.moveCursor(1)
				return nil
			case "up", "k":
				s.moveCursor(-1)
				return nil
			case "home", "g":
				s.jumpTo(0)
				return nil
			case "end", "G":
				s.jumpTo(s.activeLen() - 1)
				return nil
			case "/":
				s.filter.filtering = true
				return nil
			}
		}
	}

	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	s.invalidateView()
	return cmd
}

// filterMatch is the rowFilter matchFn for Services: a case-insensitive
// substring match against the Name column.
func (s *ServicePage) filterMatch(i int) bool {
	name, _ := s.rows[i][msgs.SvcKeyName].(string)
	return strings.Contains(strings.ToLower(name), strings.ToLower(s.filter.query))
}

// afterFilterChange: see PodPage.afterFilterChange in pods.go.
func (s *ServicePage) afterFilterChange() {
	s.cursorIdx = 0
	s.windowStart = computeWindowStart(0, s.cursorIdx, s.activeLen(), s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

// activeLen/activeRow: see PodPage in pods.go.
func (s *ServicePage) activeLen() int {
	return s.filter.len(len(s.rows))
}

func (s *ServicePage) activeRow(pos int) msgs.RowData {
	return s.rows[s.filter.absolute(pos)]
}

// FilterStatus: see PodPage.FilterStatus in pods.go.
func (s *ServicePage) FilterStatus() (query string, matches int, typing bool, ok bool) {
	if !s.filter.filtering && s.filter.query == "" {
		return "", 0, false, false
	}
	return s.filter.query, s.activeLen(), s.filter.filtering, true
}

// moveCursor: see PodPage.moveCursor in pods.go.
func (s *ServicePage) moveCursor(delta int) {
	total := s.activeLen()
	if total == 0 {
		return
	}

	s.cursorIdx += delta
	if s.cursorIdx < 0 {
		s.cursorIdx = total - 1
	} else if s.cursorIdx >= total {
		s.cursorIdx = 0
	}

	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, total, s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

// jumpTo: see PodPage.jumpTo in pods.go.
func (s *ServicePage) jumpTo(idx int) {
	total := s.activeLen()
	if total == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	} else if idx >= total {
		idx = total - 1
	}

	s.cursorIdx = idx
	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, total, s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

func (s *ServicePage) SetRows(rows []msgs.RowData) {
	if s.rowsSet && rowsEqual(rows, s.rows) {
		return
	}

	s.rows = cloneRows(rows)
	s.rowsSet = true
	s.filter.recompute(len(s.rows), s.filterMatch)
	if s.cursorIdx >= s.activeLen() {
		s.cursorIdx = max(s.activeLen()-1, 0)
	}
	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, s.activeLen(), s.windowSize)
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
// (see windowBounds in table.go) into the active index space (s.rows
// directly, or the filtered subset — see activeRow). Called whenever raw
// rows, the filter, or the cursor/window change.
func (s *ServicePage) pushDisplayRows() {
	total := s.activeLen()
	start, end := windowBounds(s.windowStart, total, s.windowSize)
	display := make([]btable.Row, 0, end-start)
	for i := start; i < end; i++ {
		row := s.activeRow(i)
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
	if s.cursorIdx < 0 || s.cursorIdx >= s.activeLen() {
		return nil
	}
	return s.activeRow(s.cursorIdx)
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
	s.windowStart = computeWindowStart(s.windowStart, s.cursorIdx, s.activeLen(), s.windowSize)
	s.pushDisplayRows()
	s.invalidateView()
}

func (s *ServicePage) invalidateView() {
	s.viewDirty = true
	s.cachedView = ""
}
