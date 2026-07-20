// Package models
package models

import (
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

	// cursorIdx/windowStart/windowSize: see the identical fields on PodPage
	// in pods.go.
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
			switch key.String() {
			case "down", "j":
				d.moveCursor(1)
				return nil
			case "up", "k":
				d.moveCursor(-1)
				return nil
			}
		}
	}

	var cmd tea.Cmd
	d.table, cmd = d.table.Update(msg)
	d.invalidateView()
	return cmd
}

// moveCursor: see PodPage.moveCursor in pods.go.
func (d *DeploymentPage) moveCursor(delta int) {
	if len(d.rows) == 0 {
		return
	}

	d.cursorIdx += delta
	if d.cursorIdx < 0 {
		d.cursorIdx = len(d.rows) - 1
	} else if d.cursorIdx >= len(d.rows) {
		d.cursorIdx = 0
	}

	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, len(d.rows), d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

func (d *DeploymentPage) SetRows(rows []msgs.RowData) {
	if d.rowsSet && rowsEqual(rows, d.rows) {
		return
	}

	d.rows = cloneRows(rows)
	d.rowsSet = true
	if d.cursorIdx >= len(d.rows) {
		d.cursorIdx = max(len(d.rows)-1, 0)
	}
	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, len(d.rows), d.windowSize)
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
// (see windowBounds in table.go) into d.rows, coloring the ready/desired
// replica cell via StyledCell to reflect deployment health. Called whenever
// raw rows or the cursor/window change.
func (d *DeploymentPage) pushDisplayRows() {
	start, end := windowBounds(d.windowStart, len(d.rows), d.windowSize)
	display := make([]btable.Row, 0, end-start)
	for _, row := range d.rows[start:end] {
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
// status bar's "< col N/M >" indicator. ok is false when the indicator
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
	if d.cursorIdx < 0 || d.cursorIdx >= len(d.rows) {
		return nil
	}
	return d.rows[d.cursorIdx]
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
	d.windowStart = computeWindowStart(d.windowStart, d.cursorIdx, len(d.rows), d.windowSize)
	d.pushDisplayRows()
	d.invalidateView()
}

func (d *DeploymentPage) invalidateView() {
	d.viewDirty = true
	d.cachedView = ""
}
