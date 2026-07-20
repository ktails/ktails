// Package models
package models

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type DeploymentPage struct {
	Client *k8s.Client
	table  btable.Model
	// share contextList
	ContextName string
	Namespace   string

	rows       []msgs.RowData
	rowsSet    bool
	cachedView string
	viewDirty  bool
	focused    bool

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
	// activeLen/activeRow), not a raw index into d.rows.
	cursorIdx   int
	windowStart int
	windowSize  int
}

func NewDeploymentPage(client *k8s.Client) *DeploymentPage {
	return &DeploymentPage{
		Client:     client,
		table:      newBubbleTable(deploymentNarrowColumns()),
		viewDirty:  true,
		windowSize: defaultRowWindowSize,
	}
}

func (d *DeploymentPage) Init() tea.Cmd {
	return nil
}

func (d *DeploymentPage) Update(msg tea.Msg) tea.Cmd {
	if d.focused {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if d.filter.filtering {
				d.filter.handleKey(key, len(d.rows), d.filterMatch)
				d.afterFilterChange()
				return nil
			}
			switch key.String() {
			case "down", "j":
				d.moveCursor(1)
				return nil
			case "up", "k":
				d.moveCursor(-1)
				return nil
			case "home", "g":
				d.jumpTo(0)
				return nil
			case "end", "G":
				d.jumpTo(d.activeLen() - 1)
				return nil
			case "/":
				d.filter.filtering = true
				return nil
			}
		}
	}

	var cmd tea.Cmd
	d.table, cmd = d.table.Update(msg)
	d.invalidateView()
	return cmd
}

// filterMatch is the rowFilter matchFn for Deployments: a case-insensitive
// substring match against the Name column.
func (d *DeploymentPage) filterMatch(i int) bool {
	name, _ := d.rows[i][msgs.DeployKeyName].(string)
	return strings.Contains(strings.ToLower(name), strings.ToLower(d.filter.query))
}

// afterFilterChange: see PodPage.afterFilterChange in pods.go.
func (d *DeploymentPage) afterFilterChange() {
	d.cursorIdx = 0
	d.windowStart = computeWindowStart(0, d.cursorIdx, d.activeLen(), d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

// activeLen/activeRow: see PodPage in pods.go.
func (d *DeploymentPage) activeLen() int {
	return d.filter.len(len(d.rows))
}

func (d *DeploymentPage) activeRow(pos int) msgs.RowData {
	return d.rows[d.filter.absolute(pos)]
}

// FilterStatus: see PodPage.FilterStatus in pods.go.
func (d *DeploymentPage) FilterStatus() (query string, matches int, typing bool, ok bool) {
	if !d.filter.filtering && d.filter.query == "" {
		return "", 0, false, false
	}
	return d.filter.query, d.activeLen(), d.filter.filtering, true
}

// moveCursor: see PodPage.moveCursor in pods.go.
func (d *DeploymentPage) moveCursor(delta int) {
	total := d.activeLen()
	if total == 0 {
		return
	}

	d.cursorIdx += delta
	if d.cursorIdx < 0 {
		d.cursorIdx = total - 1
	} else if d.cursorIdx >= total {
		d.cursorIdx = 0
	}

	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, total, d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

// jumpTo: see PodPage.jumpTo in pods.go.
func (d *DeploymentPage) jumpTo(idx int) {
	total := d.activeLen()
	if total == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	} else if idx >= total {
		idx = total - 1
	}

	d.cursorIdx = idx
	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, total, d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

func (d *DeploymentPage) SetRows(rows []msgs.RowData) {
	if d.rowsSet && rowsEqual(rows, d.rows) {
		return
	}

	d.rows = cloneRows(rows)
	d.rowsSet = true
	d.filter.recompute(len(d.rows), d.filterMatch)
	if d.cursorIdx >= d.activeLen() {
		d.cursorIdx = max(d.activeLen()-1, 0)
	}
	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, d.activeLen(), d.windowSize)
	d.applyColumns()
	d.pushDisplayRows()
	if d.focused {
		d.table = d.table.Focused(true)
	} else {
		d.table = d.table.Focused(false)
	}
	d.invalidateView()
}

// pushDisplayRows rebuilds the table's rows from the current row window
// (see windowBounds in table.go) into the active index space (d.rows
// directly, or the filtered subset — see activeRow), coloring the
// ready/desired replica cell via StyledCell to reflect deployment health.
// Called whenever raw rows, the filter, or the cursor/window change.
func (d *DeploymentPage) pushDisplayRows() {
	total := d.activeLen()
	start, end := windowBounds(d.windowStart, total, d.windowSize)
	display := make([]btable.Row, 0, end-start)
	for i := start; i < end; i++ {
		row := d.activeRow(i)
		display = append(display, btable.NewRow(btable.RowData{
			msgs.DeployKeyName:      row[msgs.DeployKeyName],
			msgs.DeployKeyAge:       row[msgs.DeployKeyAge],
			msgs.DeployKeyReplicas:  btable.NewStyledCellWithStyleFunc(row[msgs.DeployKeyReplicas], replicaCellStyle),
			msgs.DeployKeyContext:   row[msgs.DeployKeyContext],
			msgs.DeployKeyNamespace: row[msgs.DeployKeyNamespace],
			msgs.DeployKeyStrategy:  row[msgs.DeployKeyStrategy],
			msgs.DeployKeyAvailable: row[msgs.DeployKeyAvailable],
			msgs.DeployKeyUpdated:   row[msgs.DeployKeyUpdated],
			msgs.DeployKeySelector:  row[msgs.DeployKeySelector],
		}))
	}
	d.table = d.table.WithRows(display).WithHighlightedRow(d.cursorIdx - start)
}

// applyColumns rebuilds the column set for the current mode (narrow/wide),
// auto-fitting wide-mode widths to d.rows — called on every SetRows/ToggleWideMode.
func (d *DeploymentPage) applyColumns() {
	var cols []btable.Column
	if d.wideMode {
		cols = deploymentWideColumns(d.rows)
	} else {
		cols = deploymentNarrowColumns()
	}
	d.wideColCount = len(cols)
	d.scrollable = d.wideMode && totalColumnsWidth(cols) > d.tableW
	d.table = d.table.WithColumns(cols)
	// See PodPage.applyColumns: WithTargetWidth must be cleared in wide mode
	// or bubble-table forces totalWidth to it, silently disabling scroll.
	if d.wideMode {
		d.table = d.table.WithTargetWidth(0).WithMaxTotalWidth(d.tableW)
	} else {
		d.table = d.table.WithTargetWidth(d.tableW).WithMaxTotalWidth(d.tableW)
	}
}

// ToggleWideMode flips wide mode for this tab (sticky until the next
// resize) and rebuilds columns to fit the current data.
func (d *DeploymentPage) ToggleWideMode() {
	d.wideMode = !d.wideMode
	d.applyColumns()
	d.pushDisplayRows()
	d.invalidateView()
}

func (d *DeploymentPage) WideMode() bool {
	return d.wideMode
}

// ScrollStatus reports the current horizontal scroll position for the
// status bar's "◂ col N/M ▸" indicator. ok is false when the indicator
// should be hidden.
func (d *DeploymentPage) ScrollStatus() (offset, total int, ok bool) {
	if !d.wideMode || !d.scrollable {
		return 0, 0, false
	}
	return d.table.GetHorizontalScrollColumnOffset() + 1, d.wideColCount, true
}

func (d *DeploymentPage) ScrollLeft() {
	d.table = d.table.ScrollLeft()
	d.invalidateView()
}

func (d *DeploymentPage) ScrollRight() {
	d.table = d.table.ScrollRight()
	d.invalidateView()
}

func (d *DeploymentPage) View() string {
	if d.cachedView != "" && !d.viewDirty {
		return d.cachedView
	}

	view := d.table.View()
	d.cachedView = view
	d.viewDirty = false
	return view
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows.
func (d *DeploymentPage) SelectedRow() msgs.RowData {
	if d.cursorIdx < 0 || d.cursorIdx >= d.activeLen() {
		return nil
	}
	return d.activeRow(d.cursorIdx)
}

func (d *DeploymentPage) SetFocused(f bool) {
	d.focused = f
	d.table = d.table.Focused(f)
	d.invalidateView()
}

func (d *DeploymentPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	d.tableW, d.tableH = w, h
	d.wideMode = false

	st := styles.CatppuccinBubbleTableStyle()
	d.table = newBubbleTable(deploymentNarrowColumns()).
		WithMinimumHeight(h).
		WithTargetWidth(w).
		WithMaxTotalWidth(w).
		HeaderStyle(st.Header).
		HighlightStyle(st.Highlight).
		WithBaseStyle(st.Base).
		Focused(d.focused)
	d.wideColCount = len(deploymentNarrowColumns())
	d.scrollable = false
	d.windowSize = rowWindowSizeFor(h)
	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, d.activeLen(), d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

func (d *DeploymentPage) invalidateView() {
	d.viewDirty = true
	d.cachedView = ""
}
