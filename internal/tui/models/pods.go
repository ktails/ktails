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

	p.table.SetStyles(styles.CatppuccinTableStyles())
	view := p.table.View()
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
