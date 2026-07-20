package models

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type PodPage struct {
	Client  *k8s.Client
	Focused bool
	table   btable.Model

	// Cache for view rendering
	rows       []msgs.RowData
	rowsSet    bool
	cachedView string
	viewDirty  bool

	// checkedPods tracks rows checked for multi-pod log tailing, keyed by
	// PodRowKey. Persists across SetRows/reopening the log pane until
	// explicitly cleared.
	checkedPods map[string]bool

	// wideMode is sticky per tab (this is the Pods tab's own instance) and
	// only reset on resize — see SetSize.
	wideMode     bool
	tableW       int
	tableH       int
	wideColCount int
	scrollable   bool

	// filter is a k9s-style "/" filter over the Name column — see rowFilter
	// in table.go for why this exists instead of bubble-table's own filter.
	filter rowFilter

	// cursorIdx is a position in the *active index space* — p.rows directly
	// when filter is inactive, or filter.matches when it's not (see
	// activeLen/activeRow) — not a raw index into p.rows. bubble-table's own
	// highlighted-row index is relative to the windowed slice handed to it
	// (see windowStart/windowSize in table.go), so it can't be used directly
	// once more rows are loaded than fit in a window either.
	cursorIdx   int
	windowStart int
	windowSize  int
}

func NewPodPageModel(client *k8s.Client) *PodPage {
	p := &PodPage{
		Client:      client,
		viewDirty:   true,
		checkedPods: make(map[string]bool),
		windowSize:  defaultRowWindowSize,
	}
	p.table = newBubbleTable(podNarrowColumns())
	return p
}

// PodRowKey identifies a raw (un-prefixed) Pods-table row for check-state
// tracking, keyed by context/namespace/name — the same triple used to
// pin the log pane to a specific pod.
func PodRowKey(row msgs.RowData) string {
	if row == nil {
		return ""
	}
	ctx, _ := row[msgs.PodKeyContext].(string)
	ns, _ := row[msgs.PodKeyNamespace].(string)
	name, _ := row[msgs.PodKeyName].(string)
	if ctx == "" && ns == "" && name == "" {
		return ""
	}
	return ctx + "/" + ns + "/" + name
}

func (p *PodPage) Init() tea.Cmd {
	return nil
}

func (p *PodPage) Update(msg tea.Msg) tea.Cmd {
	if p.Focused {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if p.filter.filtering {
				p.filter.handleKey(key, len(p.rows), p.filterMatch)
				p.afterFilterChange()
				return nil
			}
			switch key.String() {
			case "down", "j":
				p.moveCursor(1)
				return nil
			case "up", "k":
				p.moveCursor(-1)
				return nil
			case "home", "g":
				p.jumpTo(0)
				return nil
			case "end", "G":
				p.jumpTo(p.activeLen() - 1)
				return nil
			case "/":
				p.filter.filtering = true
				return nil
			}
		}
	}

	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	p.invalidateView()
	return cmd
}

// filterMatch is the rowFilter matchFn for Pods: a case-insensitive
// substring match against the Name column.
func (p *PodPage) filterMatch(i int) bool {
	name, _ := p.rows[i][msgs.PodKeyName].(string)
	return strings.Contains(strings.ToLower(name), strings.ToLower(p.filter.query))
}

// afterFilterChange re-syncs the cursor/window to the (possibly just
// changed) filtered index space, jumping to the first match — mirroring
// k9s, which jumps to the first match as you type rather than leaving the
// cursor at a now-meaningless position.
func (p *PodPage) afterFilterChange() {
	p.cursorIdx = 0
	p.windowStart = computeWindowStart(0, p.cursorIdx, p.activeLen(), p.windowSize)
	p.pushDisplayRows()
	p.invalidateView()
}

// activeLen returns how many rows are currently selectable: the full row
// count when no filter is active, or the match count otherwise.
func (p *PodPage) activeLen() int {
	return p.filter.len(len(p.rows))
}

// activeRow returns the raw row at position pos in the active index space.
func (p *PodPage) activeRow(pos int) msgs.RowData {
	return p.rows[p.filter.absolute(pos)]
}

// FilterStatus reports the current filter text and match count, for the
// status bar's "/query (N matches)" indicator. ok is false when no filter
// is active — neither being typed nor already committed.
func (p *PodPage) FilterStatus() (query string, matches int, typing bool, ok bool) {
	if !p.filter.filtering && p.filter.query == "" {
		return "", 0, false, false
	}
	return p.filter.query, p.activeLen(), p.filter.filtering, true
}

// moveCursor shifts the cursor by delta within the active index space,
// wrapping at either end (mirroring bubble-table's own moveHighlightUp/
// Down), then slides the row window to keep the new cursor visible.
func (p *PodPage) moveCursor(delta int) {
	total := p.activeLen()
	if total == 0 {
		return
	}

	p.cursorIdx += delta
	if p.cursorIdx < 0 {
		p.cursorIdx = total - 1
	} else if p.cursorIdx >= total {
		p.cursorIdx = 0
	}

	p.windowStart = computeWindowStart(p.windowStart, p.cursorIdx, total, p.windowSize)
	p.pushDisplayRows()
	p.invalidateView()
}

// jumpTo moves the cursor directly to the given position in the active
// index space (clamped), then slides the window to keep it visible — the
// whole row set is always held in p.rows (see SetRows), only the *rendered*
// window is bounded, so jumping straight to the last row of a 2000-pod list
// is just a window recompute, not a full re-fetch or re-render of every row.
func (p *PodPage) jumpTo(idx int) {
	total := p.activeLen()
	if total == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	} else if idx >= total {
		idx = total - 1
	}

	p.cursorIdx = idx
	p.windowStart = computeWindowStart(p.windowStart, p.cursorIdx, total, p.windowSize)
	p.pushDisplayRows()
	p.invalidateView()
}

func (p *PodPage) SetRows(rows []msgs.RowData) {
	if p.rowsSet && rowsEqual(rows, p.rows) {
		return
	}

	p.rows = cloneRows(rows)
	p.rowsSet = true
	p.filter.recompute(len(p.rows), p.filterMatch)
	if p.cursorIdx >= p.activeLen() {
		p.cursorIdx = max(p.activeLen()-1, 0)
	}
	p.windowStart = computeWindowStart(p.windowStart, p.cursorIdx, p.activeLen(), p.windowSize)
	p.applyColumns()
	p.pushDisplayRows()

	if p.Focused {
		p.table = p.table.Focused(true)
	} else {
		p.table = p.table.Focused(false)
	}

	p.invalidateView()
}

// pushDisplayRows rebuilds the table's rows from the current row window
// (see windowBounds in table.go) into the active index space (p.rows
// directly, or the filtered subset — see activeRow), prepending a checkbox
// glyph and coloring Status by phase via StyledCell. Called whenever raw
// rows, check state, the filter, or the cursor/window change.
func (p *PodPage) pushDisplayRows() {
	total := p.activeLen()
	start, end := windowBounds(p.windowStart, total, p.windowSize)
	display := make([]btable.Row, 0, end-start)
	for i := start; i < end; i++ {
		row := p.activeRow(i)
		// Plain ASCII (see styles.ASCIIBorder for why): ☐/☑ carry an
		// Ambiguous East Asian Width that some terminals (e.g. Ghostty's
		// default grapheme-width-method) render as double-width.
		glyph := "-"
		if p.checkedPods[PodRowKey(row)] {
			glyph = "x"
		}
		display = append(display, btable.NewRow(btable.RowData{
			msgs.PodKeyCheck:      glyph,
			msgs.PodKeyName:       row[msgs.PodKeyName],
			msgs.PodKeyNamespace:  row[msgs.PodKeyNamespace],
			msgs.PodKeyStatus:     btable.NewStyledCellWithStyleFunc(row[msgs.PodKeyStatus], statusCellStyle),
			msgs.PodKeyRestarts:   row[msgs.PodKeyRestarts],
			msgs.PodKeyAge:        row[msgs.PodKeyAge],
			msgs.PodKeyContext:    row[msgs.PodKeyContext],
			msgs.PodKeyContainers: row[msgs.PodKeyContainers],
			msgs.PodKeyNode:       row[msgs.PodKeyNode],
			msgs.PodKeyNodeIP:     row[msgs.PodKeyNodeIP],
			msgs.PodKeyPodIP:      row[msgs.PodKeyPodIP],
			msgs.PodKeyReady:      row[msgs.PodKeyReady],
		}))
	}
	p.table = p.table.WithRows(display).WithHighlightedRow(p.cursorIdx - start)
}

// applyColumns rebuilds the column set for the current mode (narrow/wide),
// auto-fitting wide-mode widths to p.rows — called on every SetRows/ToggleWideMode.
func (p *PodPage) applyColumns() {
	var cols []btable.Column
	if p.wideMode {
		cols = podWideColumns(p.rows)
	} else {
		cols = podNarrowColumns()
	}
	p.wideColCount = len(cols)
	p.scrollable = p.wideMode && totalColumnsWidth(cols) > p.tableW
	p.table = p.table.WithColumns(cols).WithHorizontalFreezeColumnCount(1)
	// WithTargetWidth governs flex-column sizing (narrow mode) and, if left
	// set, forces bubble-table's own totalWidth to that value even for fixed
	// wide-mode columns — which would silently disable scrolling. Clear it in
	// wide mode so the real (possibly overflowing) fixed-column sum is used.
	if p.wideMode {
		p.table = p.table.WithTargetWidth(0).WithMaxTotalWidth(p.tableW)
	} else {
		p.table = p.table.WithTargetWidth(p.tableW).WithMaxTotalWidth(p.tableW)
	}
}

// ToggleWideMode flips wide mode for this tab (sticky until the next
// resize) and rebuilds columns to fit the current data.
func (p *PodPage) ToggleWideMode() {
	p.wideMode = !p.wideMode
	p.applyColumns()
	p.pushDisplayRows()
	p.invalidateView()
}

func (p *PodPage) WideMode() bool {
	return p.wideMode
}

// ScrollStatus reports the current horizontal scroll position for the
// status bar's "< col N/M >" indicator. ok is false when the indicator
// should be hidden (not in wide mode, or nothing to scroll).
func (p *PodPage) ScrollStatus() (offset, total int, ok bool) {
	if !p.wideMode || !p.scrollable {
		return 0, 0, false
	}
	return p.table.GetHorizontalScrollColumnOffset() + 1, p.wideColCount, true
}

func (p *PodPage) ScrollLeft() {
	p.table = p.table.ScrollLeft()
	p.invalidateView()
}

func (p *PodPage) ScrollRight() {
	p.table = p.table.ScrollRight()
	p.invalidateView()
}

// ToggleChecked flips the checked state of the row identified by key
// (see PodRowKey), for inclusion in a merged multi-pod log stream.
func (p *PodPage) ToggleChecked(key string) {
	if key == "" {
		return
	}
	if p.checkedPods[key] {
		delete(p.checkedPods, key)
	} else {
		p.checkedPods[key] = true
	}
	p.pushDisplayRows()
	p.invalidateView()
}

// ClearChecked unchecks every row.
func (p *PodPage) ClearChecked() {
	if len(p.checkedPods) == 0 {
		return
	}
	p.checkedPods = make(map[string]bool)
	p.pushDisplayRows()
	p.invalidateView()
}

// IsChecked reports whether the row identified by key is checked.
func (p *PodPage) IsChecked(key string) bool {
	return p.checkedPods[key]
}

// CheckedKeys returns the keys of all currently checked rows, in no
// particular order.
func (p *PodPage) CheckedKeys() []string {
	keys := make([]string, 0, len(p.checkedPods))
	for k := range p.checkedPods {
		keys = append(keys, k)
	}
	return keys
}

// CheckedRow returns the raw (un-prefixed) row for a given check key, or
// nil if no such row is currently loaded.
func (p *PodPage) CheckedRow(key string) msgs.RowData {
	for _, row := range p.rows {
		if PodRowKey(row) == key {
			return row
		}
	}
	return nil
}

func (p *PodPage) Reset() {
	p.rows = nil
	p.rowsSet = false
	p.cursorIdx = 0
	p.windowStart = 0
	p.filter = rowFilter{}
	p.table = p.table.WithRows(nil)
	p.invalidateView()
}

func (p *PodPage) SetFocused(f bool) {
	p.Focused = f
	p.table = p.table.Focused(f)
	p.invalidateView()
}

func (p *PodPage) View() string {
	if p.cachedView != "" && !p.viewDirty {
		return p.cachedView
	}

	view := p.table.View()
	p.cachedView = view
	p.viewDirty = false
	return view
}

func (p *PodPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	p.tableW, p.tableH = w, h
	p.wideMode = false

	st := styles.CatppuccinBubbleTableStyle()
	p.table = newBubbleTable(podNarrowColumns()).
		WithMinimumHeight(h).
		WithTargetWidth(w).
		WithMaxTotalWidth(w).
		HeaderStyle(st.Header).
		HighlightStyle(st.Highlight).
		WithBaseStyle(st.Base).
		WithHorizontalFreezeColumnCount(1).
		Focused(p.Focused)
	p.wideColCount = len(podNarrowColumns())
	p.scrollable = false
	p.windowSize = rowWindowSizeFor(h)
	p.windowStart = computeWindowStart(p.windowStart, p.cursorIdx, p.activeLen(), p.windowSize)
	p.pushDisplayRows()
	p.invalidateView()
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows. Raw rows are what callers should read pod
// identity out of — the table itself renders a checkbox-prefixed copy.
func (p *PodPage) SelectedRow() msgs.RowData {
	if p.cursorIdx < 0 || p.cursorIdx >= p.activeLen() {
		return nil
	}
	return p.activeRow(p.cursorIdx)
}

func (p *PodPage) invalidateView() {
	p.viewDirty = true
	p.cachedView = ""
}
