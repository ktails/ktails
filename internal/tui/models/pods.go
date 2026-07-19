package models

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

// statusRawColumn is the index of the Status field within a raw (un-prefixed)
// Pods row, matching the column order built in cmds.LoadPodInfoCmd.
const statusRawColumn = 2

// statusColIndex is the index of the Status column within podTableColumns
// (and the columns SetSize rebuilds), used to locate its rendered span.
const statusColIndex = 3

// statusColor maps a pod phase (PodInfo.Status) to its Catppuccin Mocha
// status color, per the Status Colors spec: Running=Green, Pending=Yellow,
// Failed/Unknown=Red, Succeeded=Overlay1 (dim). Unrecognized phases are
// left uncolored.
func statusColor(status string) (lipgloss.Color, bool) {
	p := styles.CatppuccinMocha()
	switch status {
	case "Running":
		return p.Green, true
	case "Pending":
		return p.Yellow, true
	case "Failed", "Unknown":
		return p.Red, true
	case "Succeeded":
		return p.Overlay1, true
	default:
		return "", false
	}
}

// statusCellSpan returns the visual column range [left, left+width) that the
// rendered Status cell occupies within a table row line, accounting for the
// Padding(0,1) bubbles/table applies to every visible cell. Columns with
// Width <= 0 are hidden and skipped entirely by bubbles/table's renderer, so
// they contribute nothing to the offset. Returns left == -1 if the Status
// column isn't currently visible.
func statusCellSpan(cols []table.Column) (left, width int) {
	for i, col := range cols {
		if col.Width <= 0 {
			continue
		}
		cellWidth := col.Width + 2
		if i == statusColIndex {
			return left, cellWidth
		}
		left += cellWidth
	}
	return -1, 0
}

// colorizeStatusColumn recolors the Status cell of each data row in an
// already-rendered table view, by phase (see statusColor).
//
// bubbles/table applies one Styles.Cell uniformly to every cell with no
// per-cell hook, so per-cell color has to be embedded in the cell's content
// string. But bubbles/table truncates/pads cell values with go-runewidth
// *before* handing them to lipgloss, and go-runewidth counts ANSI escape
// bytes as visible width — embedding a lipgloss-colored string directly into
// a table.Row value gets its escape codes sliced apart on anything but very
// wide columns, corrupting the row (verified: a colored 7-char "Running"
// value in a 10-wide column renders as garbled escape bytes and an
// unwanted "…").
//
// So instead this recolors the plain text bubbles/table already rendered
// and padded correctly, using the ansi package's escape-aware Cut/Strip,
// which also keeps any style already active at that point in the line
// (e.g. the current row's Selected background) correctly reopened after.
//
// headerLines is the number of lines the header (and its border) occupies
// at the top of view, before the first data row.
func colorizeStatusColumn(view string, cols []table.Column, rows []table.Row, headerLines int) string {
	left, width := statusCellSpan(cols)
	if left < 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	for i, row := range rows {
		li := i + headerLines
		if li >= len(lines) || len(row) <= statusRawColumn {
			continue
		}
		col, ok := statusColor(row[statusRawColumn])
		if !ok {
			continue
		}
		line := lines[li]
		prefix := ansi.Cut(line, 0, left)
		cell := ansi.Strip(ansi.Cut(line, left, left+width))
		suffix := ansi.Cut(line, left+width, len(line))
		lines[li] = prefix + lipgloss.NewStyle().Foreground(col).Render(cell) + suffix
	}
	return strings.Join(lines, "\n")
}

type PodPage struct {
	Client  *k8s.Client
	Focused bool
	table   table.Model

	// Cache for view rendering
	rows       []table.Row
	rowsSet    bool
	cachedView string
	viewDirty  bool

	// checkedPods tracks rows checked for multi-pod log tailing, keyed by
	// PodRowKey. Persists across SetRows/reopening the log pane until
	// explicitly cleared.
	checkedPods map[string]bool
}

func NewPodPageModel(client *k8s.Client) *PodPage {
	return &PodPage{
		Client:      client,
		table:       table.New(table.WithColumns(podTableColumns())),
		viewDirty:   true,
		checkedPods: make(map[string]bool),
	}
}

// PodRowKey identifies a raw (un-prefixed) Pods-table row for check-state
// tracking, keyed by context/namespace/name — the same triple used to
// pin the log pane to a specific pod.
func PodRowKey(row table.Row) string {
	if len(row) < 6 {
		return ""
	}
	return row[5] + "/" + row[1] + "/" + row[0]
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
	p.pushDisplayRows()

	if p.Focused {
		p.table.Focus()
	} else {
		p.table.Blur()
	}

	p.invalidateView()
}

// pushDisplayRows rebuilds the table's rows from p.rows (the raw fetched
// data) with a checkbox glyph prepended per row, reflecting checkedPods.
// Called whenever raw rows or check state change.
func (p *PodPage) pushDisplayRows() {
	display := make([]table.Row, len(p.rows))
	for i, row := range p.rows {
		glyph := "☐"
		if p.checkedPods[PodRowKey(row)] {
			glyph = "☑"
		}
		display[i] = append(table.Row{glyph}, row...)
	}
	p.table.SetRows(display)
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
func (p *PodPage) CheckedRow(key string) table.Row {
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

	tableStyles := styles.CatppuccinTableStyles()
	p.table.SetStyles(tableStyles)
	headerLines := lipgloss.Height(tableStyles.Header.Render("Status"))
	view := colorizeStatusColumn(p.table.View(), p.table.Columns(), p.rows, headerLines)
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
	const visibleCols = 6
	const checkW = 1
	avail := w - visibleCols*2 - checkW
	nameW := avail * 38 / 100
	nsW := avail * 22 / 100
	statusW := avail * 15 / 100
	restartsW := avail * 12 / 100
	ageW := avail - nameW - nsW - statusW - restartsW
	p.table.SetColumns([]table.Column{
		{Title: "✓", Width: checkW},
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

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows. Raw rows are what callers should read pod
// identity out of — the table itself renders a checkbox-prefixed copy.
func (p *PodPage) SelectedRow() table.Row {
	idx := p.table.Cursor()
	if idx < 0 || idx >= len(p.rows) {
		return nil
	}
	return p.rows[idx]
}

func (p *PodPage) invalidateView() {
	p.viewDirty = true
	p.cachedView = ""
}
