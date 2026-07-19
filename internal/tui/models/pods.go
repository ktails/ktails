package models

import (
	tea "github.com/charmbracelet/bubbletea"
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
}

func NewPodPageModel(client *k8s.Client) *PodPage {
	p := &PodPage{
		Client:      client,
		viewDirty:   true,
		checkedPods: make(map[string]bool),
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
	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	p.invalidateView()
	return cmd
}

func (p *PodPage) SetRows(rows []msgs.RowData) {
	if p.rowsSet && rowsEqual(rows, p.rows) {
		return
	}

	p.rows = cloneRows(rows)
	p.rowsSet = true
	p.applyColumns()
	p.pushDisplayRows()

	if p.Focused {
		p.table = p.table.Focused(true)
	} else {
		p.table = p.table.Focused(false)
	}

	p.invalidateView()
}

// pushDisplayRows rebuilds the table's rows from p.rows (the raw fetched
// data), prepending a checkbox glyph and coloring Status by phase via
// StyledCell. Called whenever raw rows or check state change.
func (p *PodPage) pushDisplayRows() {
	display := make([]btable.Row, len(p.rows))
	for i, row := range p.rows {
		glyph := "☐"
		if p.checkedPods[PodRowKey(row)] {
			glyph = "☑"
		}
		display[i] = btable.NewRow(btable.RowData{
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
		})
	}
	p.table = p.table.WithRows(display)
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
// status bar's "◂ col N/M ▸" indicator. ok is false when the indicator
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
	prevIdx := p.table.GetHighlightedRowIndex()
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
	p.pushDisplayRows()
	p.table = p.table.WithHighlightedRow(prevIdx)
	p.invalidateView()
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows. Raw rows are what callers should read pod
// identity out of — the table itself renders a checkbox-prefixed copy.
func (p *PodPage) SelectedRow() msgs.RowData {
	idx := p.table.GetHighlightedRowIndex()
	if idx < 0 || idx >= len(p.rows) {
		return nil
	}
	return p.rows[idx]
}

func (p *PodPage) invalidateView() {
	p.viewDirty = true
	p.cachedView = ""
}
