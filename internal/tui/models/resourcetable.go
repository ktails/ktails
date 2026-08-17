package models

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/ktails/ktails/internal/tui/views"
)

// tableSpec is everything that genuinely differs between the three resource
// tables: their columns, how a raw row becomes a display row, and whether
// rows are checkable (Pods only — for multi-pod log tailing). Everything
// else — cursor, windowing, "/" filter, wide mode, column scroll, view
// caching — is ResourceTable's implementation, shared by all three.
type tableSpec struct {
	// narrowColumns builds narrow-mode columns; compact drops the lowest-
	// priority fixed column (Age) per the column-priority order in §8.3,
	// for TierCompact terminals.
	narrowColumns func(compact bool) []btable.Column
	wideColumns   func(rows []msgs.Row) []btable.Column
	// displayRow converts one raw row into bubble-table's row format;
	// colors is the context->identity-colour map for the Context column
	// (see contextCell in table.go), refreshed via ResourceTable.SetContextColors.
	displayRow func(row msgs.Row, checked bool, colors map[string]color.Color) btable.RowData
	// freezeColumns pins the first N columns during horizontal scroll
	// (the Pods checkbox column).
	freezeColumns int
	checkable     bool
}

func specFor(kind msgs.ResourceKind) tableSpec {
	switch kind {
	case msgs.KindPods:
		return tableSpec{
			narrowColumns: podNarrowColumns,
			wideColumns:   podWideColumns,
			displayRow:    podDisplayRow,
			// Checkbox + Context: the Context column is the tool's core
			// differentiator (§3.4) and must never scroll out of view in
			// wide mode, same as the checkbox.
			freezeColumns: 2,
			checkable:     true,
		}
	case msgs.KindDeployments:
		return tableSpec{
			narrowColumns: deploymentNarrowColumns,
			wideColumns:   deploymentWideColumns,
			displayRow:    deploymentDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindServices:
		return tableSpec{
			narrowColumns: svcNarrowColumns,
			wideColumns:   svcWideColumns,
			displayRow:    svcDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindConfigMaps:
		return tableSpec{
			narrowColumns: configMapNarrowColumns,
			wideColumns:   configMapWideColumns,
			displayRow:    configMapDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindSecrets:
		return tableSpec{
			narrowColumns: secretNarrowColumns,
			wideColumns:   secretWideColumns,
			displayRow:    secretDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindJobs:
		return tableSpec{
			narrowColumns: jobNarrowColumns,
			wideColumns:   jobWideColumns,
			displayRow:    jobDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindCronJobs:
		return tableSpec{
			narrowColumns: cronJobNarrowColumns,
			wideColumns:   cronJobWideColumns,
			displayRow:    cronJobDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindStatefulSets:
		return tableSpec{
			narrowColumns: statefulSetNarrowColumns,
			wideColumns:   statefulSetWideColumns,
			displayRow:    statefulSetDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindDaemonSets:
		return tableSpec{
			narrowColumns: daemonSetNarrowColumns,
			wideColumns:   daemonSetWideColumns,
			displayRow:    daemonSetDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindIngresses:
		return tableSpec{
			narrowColumns: ingressNarrowColumns,
			wideColumns:   ingressWideColumns,
			displayRow:    ingressDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindPodDisruptionBudgets:
		return tableSpec{
			narrowColumns: pdbNarrowColumns,
			wideColumns:   pdbWideColumns,
			displayRow:    pdbDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindHorizontalPodAutoscalers:
		return tableSpec{
			narrowColumns: hpaNarrowColumns,
			wideColumns:   hpaWideColumns,
			displayRow:    hpaDisplayRow,
			freezeColumns: 1,
		}
	case msgs.KindNodes:
		return tableSpec{
			narrowColumns: nodeNarrowColumns,
			wideColumns:   nodeWideColumns,
			freezeColumns: 1,
			displayRow:    nodeDisplayRow,
		}
	}
	return tableSpec{}
}

func podDisplayRow(row msgs.Row, checked bool, colors map[string]color.Color) btable.RowData {
	glyph := "☐"
	if checked {
		glyph = "☑"
	}
	return btable.RowData{
		msgs.PodKeyCheck:     glyph,
		msgs.PodKeyContext:   contextCell(row.Context, colors),
		msgs.PodKeyNamespace: row.Namespace,
		msgs.PodKeyName:      row.Name,
		msgs.PodKeyStatus:    btable.NewStyledCellWithStyleFunc(row.Cells[msgs.PodKeyStatus], statusCellStyle),
		msgs.PodKeyRestarts:  row.Cells[msgs.PodKeyRestarts],
		msgs.PodKeyAge:       row.Cells[msgs.PodKeyAge],
		msgs.PodKeyNode:      row.Cells[msgs.PodKeyNode],
		msgs.PodKeyNodeIP:    row.Cells[msgs.PodKeyNodeIP],
		msgs.PodKeyPodIP:     row.Cells[msgs.PodKeyPodIP],
		msgs.PodKeyReady:     row.Cells[msgs.PodKeyReady],
		msgs.PodKeyCPU:       row.Cells[msgs.PodKeyCPU],
		msgs.PodKeyMemory:    row.Cells[msgs.PodKeyMemory],
	}
}

func deploymentDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.DeployKeyContext:   contextCell(row.Context, colors),
		msgs.DeployKeyNamespace: row.Namespace,
		msgs.DeployKeyName:      row.Name,
		msgs.DeployKeyAge:       row.Cells[msgs.DeployKeyAge],
		msgs.DeployKeyReplicas:  btable.NewStyledCellWithStyleFunc(row.Cells[msgs.DeployKeyReplicas], replicaCellStyle),
		msgs.DeployKeyStrategy:  row.Cells[msgs.DeployKeyStrategy],
		msgs.DeployKeyAvailable: row.Cells[msgs.DeployKeyAvailable],
		msgs.DeployKeyUpdated:   row.Cells[msgs.DeployKeyUpdated],
		msgs.DeployKeySelector:  row.Cells[msgs.DeployKeySelector],
	}
}

func svcDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.SvcKeyContext:     contextCell(row.Context, colors),
		msgs.SvcKeyNamespace:   row.Namespace,
		msgs.SvcKeyName:        row.Name,
		msgs.SvcKeyType:        row.Cells[msgs.SvcKeyType],
		msgs.SvcKeyClusterIP:   row.Cells[msgs.SvcKeyClusterIP],
		msgs.SvcKeyPorts:       row.Cells[msgs.SvcKeyPorts],
		msgs.SvcKeyAge:         row.Cells[msgs.SvcKeyAge],
		msgs.SvcKeySelector:    row.Cells[msgs.SvcKeySelector],
		msgs.SvcKeyExternalIP:  row.Cells[msgs.SvcKeyExternalIP],
		msgs.SvcKeyEndpointIPs: row.Cells[msgs.SvcKeyEndpointIPs],
	}
}

func configMapDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.ConfigMapKeyContext:   contextCell(row.Context, colors),
		msgs.ConfigMapKeyNamespace: row.Namespace,
		msgs.ConfigMapKeyName:      row.Name,
		msgs.ConfigMapKeyKeys:      row.Cells[msgs.ConfigMapKeyKeys],
		msgs.ConfigMapKeyAge:       row.Cells[msgs.ConfigMapKeyAge],
		msgs.ConfigMapKeyKeyNames:  row.Cells[msgs.ConfigMapKeyKeyNames],
	}
}

func secretDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.SecretKeyContext:   contextCell(row.Context, colors),
		msgs.SecretKeyNamespace: row.Namespace,
		msgs.SecretKeyName:      row.Name,
		msgs.SecretKeyType:      row.Cells[msgs.SecretKeyType],
		msgs.SecretKeyKeys:      row.Cells[msgs.SecretKeyKeys],
		msgs.SecretKeyAge:       row.Cells[msgs.SecretKeyAge],
	}
}

func jobDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.JobKeyContext:     contextCell(row.Context, colors),
		msgs.JobKeyNamespace:   row.Namespace,
		msgs.JobKeyName:        row.Name,
		msgs.JobKeyCompletions: row.Cells[msgs.JobKeyCompletions],
		msgs.JobKeyDuration:    row.Cells[msgs.JobKeyDuration],
		msgs.JobKeyAge:         row.Cells[msgs.JobKeyAge],
		msgs.JobKeyStatus:      row.Cells[msgs.JobKeyStatus],
	}
}

func cronJobDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.CronJobKeyContext:       contextCell(row.Context, colors),
		msgs.CronJobKeyNamespace:     row.Namespace,
		msgs.CronJobKeyName:          row.Name,
		msgs.CronJobKeySchedule:      row.Cells[msgs.CronJobKeySchedule],
		msgs.CronJobKeySuspend:       row.Cells[msgs.CronJobKeySuspend],
		msgs.CronJobKeyAge:           row.Cells[msgs.CronJobKeyAge],
		msgs.CronJobKeyLastScheduled: row.Cells[msgs.CronJobKeyLastScheduled],
	}
}

func statefulSetDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.StatefulSetKeyContext:   contextCell(row.Context, colors),
		msgs.StatefulSetKeyNamespace: row.Namespace,
		msgs.StatefulSetKeyName:      row.Name,
		msgs.StatefulSetKeyReady:     row.Cells[msgs.StatefulSetKeyReady],
		msgs.StatefulSetKeyAge:       row.Cells[msgs.StatefulSetKeyAge],
		msgs.StatefulSetKeySelector:  row.Cells[msgs.StatefulSetKeySelector],
	}
}

func daemonSetDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.DaemonSetKeyContext:   contextCell(row.Context, colors),
		msgs.DaemonSetKeyNamespace: row.Namespace,
		msgs.DaemonSetKeyName:      row.Name,
		msgs.DaemonSetKeyReady:     row.Cells[msgs.DaemonSetKeyReady],
		msgs.DaemonSetKeyAge:       row.Cells[msgs.DaemonSetKeyAge],
		msgs.DaemonSetKeySelector:  row.Cells[msgs.DaemonSetKeySelector],
	}
}

func ingressDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.IngressKeyContext:   contextCell(row.Context, colors),
		msgs.IngressKeyNamespace: row.Namespace,
		msgs.IngressKeyName:      row.Name,
		msgs.IngressKeyHosts:     row.Cells[msgs.IngressKeyHosts],
		msgs.IngressKeyClass:     row.Cells[msgs.IngressKeyClass],
		msgs.IngressKeyAge:       row.Cells[msgs.IngressKeyAge],
		msgs.IngressKeyBackends:  row.Cells[msgs.IngressKeyBackends],
	}
}

func pdbDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.PDBKeyContext:            contextCell(row.Context, colors),
		msgs.PDBKeyNamespace:          row.Namespace,
		msgs.PDBKeyName:               row.Name,
		msgs.PDBKeyMinMaxAvailable:    row.Cells[msgs.PDBKeyMinMaxAvailable],
		msgs.PDBKeyAllowedDisruptions: row.Cells[msgs.PDBKeyAllowedDisruptions],
		msgs.PDBKeyAge:                row.Cells[msgs.PDBKeyAge],
		msgs.PDBKeyCurrentHealthy:     row.Cells[msgs.PDBKeyCurrentHealthy],
		msgs.PDBKeyDesiredHealthy:     row.Cells[msgs.PDBKeyDesiredHealthy],
	}
}

func hpaDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.HPAKeyContext:   contextCell(row.Context, colors),
		msgs.HPAKeyNamespace: row.Namespace,
		msgs.HPAKeyName:      row.Name,
		msgs.HPAKeyReference: row.Cells[msgs.HPAKeyReference],
		msgs.HPAKeyMinMax:    row.Cells[msgs.HPAKeyMinMax],
		msgs.HPAKeyReplicas:  row.Cells[msgs.HPAKeyReplicas],
		msgs.HPAKeyTargets:   row.Cells[msgs.HPAKeyTargets],
		msgs.HPAKeyAge:       row.Cells[msgs.HPAKeyAge],
	}
}

func nodeDisplayRow(row msgs.Row, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.NodeKeyContext:    contextCell(row.Context, colors),
		msgs.NodeKeyName:       row.Name,
		msgs.NodeKeyStatus:     row.Cells[msgs.NodeKeyStatus],
		msgs.NodeKeyRoles:      row.Cells[msgs.NodeKeyRoles],
		msgs.NodeKeyAge:        row.Cells[msgs.NodeKeyAge],
		msgs.NodeKeyVersion:    row.Cells[msgs.NodeKeyVersion],
		msgs.NodeKeyInternalIP: row.Cells[msgs.NodeKeyInternalIP],
		msgs.NodeKeyOS:         row.Cells[msgs.NodeKeyOS],
		msgs.NodeKeyCPU:        row.Cells[msgs.NodeKeyCPU],
		msgs.NodeKeyMemory:     row.Cells[msgs.NodeKeyMemory],
	}
}

// ResourceTable is the one table model behind the Deployments/Pods/svc tabs.
// It renders a bounded window of its full row set (see rowWindowSizeFor),
// tracks its own cursor in the active index space (all rows, or the "/"
// filter's matches), and supports sticky wide mode with horizontal column
// scroll. Which resource it shows is entirely a matter of its tableSpec.
type ResourceTable struct {
	table btable.Model
	spec  tableSpec

	rows       []msgs.Row
	rowsSet    bool
	cachedView string
	viewDirty  bool
	focused    bool

	// checked tracks rows checked for multi-pod log tailing, keyed by
	// PodRowKey. Persists across SetRows/reopening the log pane until
	// explicitly cleared. Nil unless spec.checkable.
	checked map[string]bool

	// contextColors is the context->identity-colour map for the Context
	// column (see SetContextColors), kept in sync with
	// state.AppState.Snapshot().ContextColors.
	contextColors map[string]color.Color

	// tier is the current adaptive-layout width tier (§8.1), set via
	// SetTier — TierCompact drops the lowest-priority narrow-mode column
	// (§8.3).
	tier views.Tier

	// wideMode is sticky per tab and only reset on resize — see SetSize.
	wideMode     bool
	tableW       int
	tableH       int
	wideColCount int
	scrollable   bool

	// filter is a k9s-style "/" filter over the Name column — see rowFilter
	// in table.go for why this exists instead of bubble-table's own filter.
	filter rowFilter

	// cursorIdx is a position in the *active index space* — t.rows directly
	// when filter is inactive, or filter.matches when it's not (see
	// activeLen/activeRow) — not a raw index into t.rows. bubble-table's own
	// highlighted-row index is relative to the windowed slice handed to it
	// (see windowStart/windowSize in table.go), so it can't be used directly
	// once more rows are loaded than fit in a window either.
	cursorIdx   int
	windowStart int
	windowSize  int
}

// NewResourceTable builds the table for one resource kind.
func NewResourceTable(kind msgs.ResourceKind) *ResourceTable {
	spec := specFor(kind)
	t := &ResourceTable{
		spec:       spec,
		viewDirty:  true,
		windowSize: defaultRowWindowSize,
	}
	if spec.checkable {
		t.checked = make(map[string]bool)
	}
	t.table = newBubbleTable(spec.narrowColumns(false))
	return t
}

// SetTier updates the adaptive-layout width tier (§8.1) and, if narrow mode
// is active, rebuilds columns for it (TierCompact drops Age — see §8.3).
func (t *ResourceTable) SetTier(tier views.Tier) {
	if t.tier == tier {
		return
	}
	t.tier = tier
	if !t.wideMode {
		t.applyColumns()
		t.invalidateView()
	}
}

// PodRowKey identifies a raw (un-prefixed) Pods-table row for check-state
// tracking, keyed by context/namespace/name — the same triple used to
// pin the log pane to a specific pod. row is nil-able so SelectedRow's "no
// row" result can pass straight through without the caller checking first.
func PodRowKey(row *msgs.Row) string {
	if row == nil {
		return ""
	}
	if row.Context == "" && row.Namespace == "" && row.Name == "" {
		return ""
	}
	return row.Context + "/" + row.Namespace + "/" + row.Name
}

func (t *ResourceTable) Update(msg tea.Msg) tea.Cmd {
	if t.focused {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if t.filter.filtering {
				t.filter.handleKey(key, len(t.rows), t.filterMatch)
				t.afterFilterChange()
				return nil
			}
			switch key.String() {
			case "down", "j":
				t.moveCursor(1)
				return nil
			case "up", "k":
				t.moveCursor(-1)
				return nil
			case "pgdown", "ctrl+d":
				t.pageCursor(1)
				return nil
			case "pgup", "ctrl+u":
				t.pageCursor(-1)
				return nil
			case "home", "g":
				t.jumpTo(0)
				return nil
			case "end", "G":
				t.jumpTo(t.activeLen() - 1)
				return nil
			case "/":
				t.filter.filtering = true
				return nil
			}
		}
	}

	var cmd tea.Cmd
	t.table, cmd = t.table.Update(msg)
	t.invalidateView()
	return cmd
}

// filterMatch is the rowFilter matchFn: a case-insensitive substring match
// against the Name column.
func (t *ResourceTable) filterMatch(i int) bool {
	return strings.Contains(strings.ToLower(t.rows[i].Name), strings.ToLower(t.filter.query))
}

// afterFilterChange re-syncs the cursor/window to the (possibly just
// changed) filtered index space, jumping to the first match — mirroring
// k9s, which jumps to the first match as you type rather than leaving the
// cursor at a now-meaningless position.
func (t *ResourceTable) afterFilterChange() {
	t.cursorIdx = 0
	t.windowStart = computeWindowStart(0, t.cursorIdx, t.activeLen(), t.windowSize)
	t.pushDisplayRows()
	t.invalidateView()
}

// activeLen returns how many rows are currently selectable: the full row
// count when no filter is active, or the match count otherwise.
func (t *ResourceTable) activeLen() int {
	return t.filter.len(len(t.rows))
}

// activeRow returns the raw row at position pos in the active index space.
func (t *ResourceTable) activeRow(pos int) msgs.Row {
	return t.rows[t.filter.absolute(pos)]
}

// RowCount returns the total number of loaded rows, ignoring any filter.
func (t *ResourceTable) RowCount() int {
	return len(t.rows)
}

// FilterStatus reports the current filter text and match count, for the
// status bar's "/query (N matches)" indicator. ok is false when no filter
// is active — neither being typed nor already committed.
func (t *ResourceTable) FilterStatus() (query string, matches int, typing bool, ok bool) {
	if !t.filter.filtering && t.filter.query == "" {
		return "", 0, false, false
	}
	return t.filter.query, t.activeLen(), t.filter.filtering, true
}

// moveCursor shifts the cursor by delta within the active index space,
// wrapping at either end (mirroring bubble-table's own moveHighlightUp/
// Down), then slides the row window to keep the new cursor visible.
func (t *ResourceTable) moveCursor(delta int) {
	total := t.activeLen()
	if total == 0 {
		return
	}

	t.cursorIdx += delta
	if t.cursorIdx < 0 {
		t.cursorIdx = total - 1
	} else if t.cursorIdx >= total {
		t.cursorIdx = 0
	}

	t.windowStart = computeWindowStart(t.windowStart, t.cursorIdx, total, t.windowSize)
	t.pushDisplayRows()
	t.invalidateView()
}

// pageCursor moves the cursor by one visible page (windowSize rows) in the
// given direction (+1 down, -1 up), clamping at either end rather than
// wrapping — a page jump landing back at the opposite end of the list would
// be disorienting in a way moveCursor's single-step wrap isn't.
func (t *ResourceTable) pageCursor(dir int) {
	size := t.windowSize
	if size < 1 {
		size = 1
	}
	t.jumpTo(t.cursorIdx + dir*size)
}

// jumpTo moves the cursor directly to the given position in the active
// index space (clamped), then slides the window to keep it visible — the
// whole row set is always held in t.rows (see SetRows), only the *rendered*
// window is bounded, so jumping straight to the last row of a 2000-pod list
// is just a window recompute, not a full re-fetch or re-render of every row.
func (t *ResourceTable) jumpTo(idx int) {
	total := t.activeLen()
	if total == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	} else if idx >= total {
		idx = total - 1
	}

	t.cursorIdx = idx
	t.windowStart = computeWindowStart(t.windowStart, t.cursorIdx, total, t.windowSize)
	t.pushDisplayRows()
	t.invalidateView()
}

// SetRows replaces the full row set, preserving cursor/window/filter/scroll
// state. Callers hand over ownership of rows — the watch caches build every
// row map fresh, so no defensive clone is taken.
func (t *ResourceTable) SetRows(rows []msgs.Row) {
	if t.rowsSet && rowsEqual(rows, t.rows) {
		return
	}

	t.rows = rows
	t.rowsSet = true
	t.filter.recompute(len(t.rows), t.filterMatch)
	if t.cursorIdx >= t.activeLen() {
		t.cursorIdx = max(t.activeLen()-1, 0)
	}
	t.windowStart = computeWindowStart(t.windowStart, t.cursorIdx, t.activeLen(), t.windowSize)
	t.applyColumns()
	t.pushDisplayRows()
	t.table = t.table.Focused(t.focused)
	t.invalidateView()
}

// SetContextColors refreshes the Context column's colour lookup and
// re-renders the visible row window so every table's Context swatch stays
// in sync with the sidebar's (see ContextsInfo.SetContextColors).
func (t *ResourceTable) SetContextColors(colors map[string]color.Color) {
	if colorMapsEqual(t.contextColors, colors) {
		return
	}
	t.contextColors = colors
	if t.rowsSet {
		t.pushDisplayRows()
		t.invalidateView()
	}
}

func colorMapsEqual(a, b map[string]color.Color) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// pushDisplayRows rebuilds the table's rows from the current row window
// (see windowBounds in table.go) into the active index space (t.rows
// directly, or the filtered subset — see activeRow), converting each raw
// row through the spec's displayRow. Called whenever raw rows, check state,
// the filter, or the cursor/window change.
func (t *ResourceTable) pushDisplayRows() {
	total := t.activeLen()
	start, end := windowBounds(t.windowStart, total, t.windowSize)
	display := make([]btable.Row, 0, end-start)
	for i := start; i < end; i++ {
		row := t.activeRow(i)
		display = append(display, btable.NewRow(t.spec.displayRow(row, t.checked[PodRowKey(&row)], t.contextColors)))
	}
	t.table = t.table.WithRows(display).WithHighlightedRow(t.cursorIdx - start)
}

// applyColumns rebuilds the column set for the current mode (narrow/wide),
// auto-fitting wide-mode widths to t.rows — called on every SetRows/ToggleWideMode.
func (t *ResourceTable) applyColumns() {
	var cols []btable.Column
	if t.wideMode {
		cols = t.spec.wideColumns(t.rows)
	} else {
		cols = t.spec.narrowColumns(t.tier == views.TierCompact)
	}
	t.wideColCount = len(cols)
	t.scrollable = t.wideMode && totalColumnsWidth(cols) > t.tableW
	t.table = t.table.WithColumns(cols).WithHorizontalFreezeColumnCount(t.spec.freezeColumns)
	// WithTargetWidth governs flex-column sizing (narrow mode) and, if left
	// set, forces bubble-table's own totalWidth to that value even for fixed
	// wide-mode columns — which would silently disable scrolling. Clear it in
	// wide mode so the real (possibly overflowing) fixed-column sum is used.
	if t.wideMode {
		t.table = t.table.WithTargetWidth(0).WithMaxTotalWidth(t.tableW)
	} else {
		t.table = t.table.WithTargetWidth(t.tableW).WithMaxTotalWidth(t.tableW)
	}
}

// ToggleWideMode flips wide mode for this tab (sticky until the next
// resize) and rebuilds columns to fit the current data.
func (t *ResourceTable) ToggleWideMode() {
	t.wideMode = !t.wideMode
	t.applyColumns()
	t.pushDisplayRows()
	t.invalidateView()
}

func (t *ResourceTable) WideMode() bool {
	return t.wideMode
}

// ScrollStatus reports the current horizontal scroll position for the
// status bar's "◂ col N/M ▸" indicator. ok is false when the indicator
// should be hidden (not in wide mode, or nothing to scroll).
func (t *ResourceTable) ScrollStatus() (offset, total int, ok bool) {
	if !t.wideMode || !t.scrollable {
		return 0, 0, false
	}
	return t.table.GetHorizontalScrollColumnOffset() + 1, t.wideColCount, true
}

func (t *ResourceTable) ScrollLeft() {
	t.table = t.table.ScrollLeft()
	t.invalidateView()
}

func (t *ResourceTable) ScrollRight() {
	t.table = t.table.ScrollRight()
	t.invalidateView()
}

// ToggleChecked flips the checked state of the row identified by key
// (see PodRowKey), for inclusion in a merged multi-pod log stream. No-op
// on tables whose spec isn't checkable.
func (t *ResourceTable) ToggleChecked(key string) {
	if key == "" || t.checked == nil {
		return
	}
	if t.checked[key] {
		delete(t.checked, key)
	} else {
		t.checked[key] = true
	}
	t.pushDisplayRows()
	t.invalidateView()
}

// ClearChecked unchecks every row.
func (t *ResourceTable) ClearChecked() {
	if len(t.checked) == 0 {
		return
	}
	t.checked = make(map[string]bool)
	t.pushDisplayRows()
	t.invalidateView()
}

// IsChecked reports whether the row identified by key is checked.
func (t *ResourceTable) IsChecked(key string) bool {
	return t.checked[key]
}

// CheckedKeys returns the keys of all currently checked rows, in no
// particular order.
func (t *ResourceTable) CheckedKeys() []string {
	keys := make([]string, 0, len(t.checked))
	for k := range t.checked {
		keys = append(keys, k)
	}
	return keys
}

// CheckedRow returns the raw (un-prefixed) row for a given check key, or
// nil if no such row is currently loaded.
func (t *ResourceTable) CheckedRow(key string) *msgs.Row {
	for i := range t.rows {
		if PodRowKey(&t.rows[i]) == key {
			return &t.rows[i]
		}
	}
	return nil
}

func (t *ResourceTable) SetFocused(f bool) {
	t.focused = f
	t.table = t.table.Focused(f)
	t.invalidateView()
}

func (t *ResourceTable) View() string {
	if t.cachedView != "" && !t.viewDirty {
		return t.cachedView
	}

	view := t.table.View()
	t.cachedView = view
	t.viewDirty = false
	return view
}

// SetSize resizes the table. It updates the existing btable.Model via its
// builder methods (which each return an updated copy, not a rebuild) rather
// than constructing a brand-new one — the previous version called
// newBubbleTable (btable.New(cols), a fresh internal state) on every resize,
// meaning ×13 tables rebuilt from scratch continuously during a terminal
// drag. Non-size options (Border, NoPagination — set once in
// NewResourceTable's newBubbleTable call) are left untouched by construction
// since t.table itself now persists across resizes.
func (t *ResourceTable) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	t.tableW, t.tableH = w, h
	// Resetting wide mode on every resize, rather than re-fitting its
	// (possibly overflowing) fixed column set to the new width, avoids a
	// class of bubble-table column-width edge cases that aren't worth the
	// modest allocation preserving it across resize would save — see
	// IMPROVEMENT_PLAN.md Phase 2.7.
	t.wideMode = false
	t.applyColumns() // narrow-mode columns, WithTargetWidth/WithMaxTotalWidth, wideColCount, scrollable

	st := styles.CatppuccinBubbleTableStyle()
	t.table = t.table.
		WithMinimumHeight(h).
		HeaderStyle(st.Header).
		HighlightStyle(st.Highlight).
		WithBaseStyle(st.Base).
		Focused(t.focused)

	t.windowSize = rowWindowSizeFor(h)
	t.windowStart = computeWindowStart(t.windowStart, t.cursorIdx, t.activeLen(), t.windowSize)
	t.pushDisplayRows()
	t.invalidateView()
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows. Raw rows are what callers should read
// resource identity out of — the table itself renders a display copy.
func (t *ResourceTable) SelectedRow() *msgs.Row {
	if t.cursorIdx < 0 || t.cursorIdx >= t.activeLen() {
		return nil
	}
	row := t.activeRow(t.cursorIdx)
	return &row
}

func (t *ResourceTable) invalidateView() {
	t.viewDirty = true
	t.cachedView = ""
}
