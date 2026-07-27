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
	wideColumns   func(rows []msgs.RowData) []btable.Column
	// displayRow converts one raw row into bubble-table's row format;
	// colors is the context->identity-colour map for the Context column
	// (see contextCell in table.go), refreshed via ResourceTable.SetContextColors.
	displayRow func(row msgs.RowData, checked bool, colors map[string]color.Color) btable.RowData
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

// rowContext reads a raw row's context name for the Context column's
// colour lookup — every kind shares msgs.KeyContext under the hood (see
// the *KeyContext aliases in msgs.go).
func rowContext(row msgs.RowData) string {
	name, _ := row[msgs.KeyContext].(string)
	return name
}

func podDisplayRow(row msgs.RowData, checked bool, colors map[string]color.Color) btable.RowData {
	glyph := "☐"
	if checked {
		glyph = "☑"
	}
	return btable.RowData{
		msgs.PodKeyCheck:      glyph,
		msgs.PodKeyContext:    contextCell(rowContext(row), colors),
		msgs.PodKeyNamespace:  row[msgs.PodKeyNamespace],
		msgs.PodKeyName:       row[msgs.PodKeyName],
		msgs.PodKeyStatus:     btable.NewStyledCellWithStyleFunc(row[msgs.PodKeyStatus], statusCellStyle),
		msgs.PodKeyRestarts:   row[msgs.PodKeyRestarts],
		msgs.PodKeyAge:        row[msgs.PodKeyAge],
		msgs.PodKeyContainers: row[msgs.PodKeyContainers],
		msgs.PodKeyNode:       row[msgs.PodKeyNode],
		msgs.PodKeyNodeIP:     row[msgs.PodKeyNodeIP],
		msgs.PodKeyPodIP:      row[msgs.PodKeyPodIP],
		msgs.PodKeyReady:      row[msgs.PodKeyReady],
	}
}

func deploymentDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.DeployKeyContext:   contextCell(rowContext(row), colors),
		msgs.DeployKeyNamespace: row[msgs.DeployKeyNamespace],
		msgs.DeployKeyName:      row[msgs.DeployKeyName],
		msgs.DeployKeyAge:       row[msgs.DeployKeyAge],
		msgs.DeployKeyReplicas:  btable.NewStyledCellWithStyleFunc(row[msgs.DeployKeyReplicas], replicaCellStyle),
		msgs.DeployKeyStrategy:  row[msgs.DeployKeyStrategy],
		msgs.DeployKeyAvailable: row[msgs.DeployKeyAvailable],
		msgs.DeployKeyUpdated:   row[msgs.DeployKeyUpdated],
		msgs.DeployKeySelector:  row[msgs.DeployKeySelector],
	}
}

func svcDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.SvcKeyContext:     contextCell(rowContext(row), colors),
		msgs.SvcKeyNamespace:   row[msgs.SvcKeyNamespace],
		msgs.SvcKeyName:        row[msgs.SvcKeyName],
		msgs.SvcKeyType:        row[msgs.SvcKeyType],
		msgs.SvcKeyClusterIP:   row[msgs.SvcKeyClusterIP],
		msgs.SvcKeyPorts:       row[msgs.SvcKeyPorts],
		msgs.SvcKeyAge:         row[msgs.SvcKeyAge],
		msgs.SvcKeySelector:    row[msgs.SvcKeySelector],
		msgs.SvcKeyExternalIP:  row[msgs.SvcKeyExternalIP],
		msgs.SvcKeyEndpointIPs: row[msgs.SvcKeyEndpointIPs],
	}
}

func configMapDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.ConfigMapKeyContext:   contextCell(rowContext(row), colors),
		msgs.ConfigMapKeyNamespace: row[msgs.ConfigMapKeyNamespace],
		msgs.ConfigMapKeyName:      row[msgs.ConfigMapKeyName],
		msgs.ConfigMapKeyKeys:      row[msgs.ConfigMapKeyKeys],
		msgs.ConfigMapKeyAge:       row[msgs.ConfigMapKeyAge],
		msgs.ConfigMapKeyKeyNames:  row[msgs.ConfigMapKeyKeyNames],
	}
}

func secretDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.SecretKeyContext:   contextCell(rowContext(row), colors),
		msgs.SecretKeyNamespace: row[msgs.SecretKeyNamespace],
		msgs.SecretKeyName:      row[msgs.SecretKeyName],
		msgs.SecretKeyType:      row[msgs.SecretKeyType],
		msgs.SecretKeyKeys:      row[msgs.SecretKeyKeys],
		msgs.SecretKeyAge:       row[msgs.SecretKeyAge],
	}
}

func jobDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.JobKeyContext:     contextCell(rowContext(row), colors),
		msgs.JobKeyNamespace:   row[msgs.JobKeyNamespace],
		msgs.JobKeyName:        row[msgs.JobKeyName],
		msgs.JobKeyCompletions: row[msgs.JobKeyCompletions],
		msgs.JobKeyDuration:    row[msgs.JobKeyDuration],
		msgs.JobKeyAge:         row[msgs.JobKeyAge],
		msgs.JobKeyStatus:      row[msgs.JobKeyStatus],
	}
}

func cronJobDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.CronJobKeyContext:       contextCell(rowContext(row), colors),
		msgs.CronJobKeyNamespace:     row[msgs.CronJobKeyNamespace],
		msgs.CronJobKeyName:          row[msgs.CronJobKeyName],
		msgs.CronJobKeySchedule:      row[msgs.CronJobKeySchedule],
		msgs.CronJobKeySuspend:       row[msgs.CronJobKeySuspend],
		msgs.CronJobKeyAge:           row[msgs.CronJobKeyAge],
		msgs.CronJobKeyLastScheduled: row[msgs.CronJobKeyLastScheduled],
	}
}

func statefulSetDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.StatefulSetKeyContext:   contextCell(rowContext(row), colors),
		msgs.StatefulSetKeyNamespace: row[msgs.StatefulSetKeyNamespace],
		msgs.StatefulSetKeyName:      row[msgs.StatefulSetKeyName],
		msgs.StatefulSetKeyReady:     row[msgs.StatefulSetKeyReady],
		msgs.StatefulSetKeyAge:       row[msgs.StatefulSetKeyAge],
		msgs.StatefulSetKeySelector:  row[msgs.StatefulSetKeySelector],
	}
}

func daemonSetDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.DaemonSetKeyContext:   contextCell(rowContext(row), colors),
		msgs.DaemonSetKeyNamespace: row[msgs.DaemonSetKeyNamespace],
		msgs.DaemonSetKeyName:      row[msgs.DaemonSetKeyName],
		msgs.DaemonSetKeyReady:     row[msgs.DaemonSetKeyReady],
		msgs.DaemonSetKeyAge:       row[msgs.DaemonSetKeyAge],
		msgs.DaemonSetKeySelector:  row[msgs.DaemonSetKeySelector],
	}
}

func ingressDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.IngressKeyContext:   contextCell(rowContext(row), colors),
		msgs.IngressKeyNamespace: row[msgs.IngressKeyNamespace],
		msgs.IngressKeyName:      row[msgs.IngressKeyName],
		msgs.IngressKeyHosts:     row[msgs.IngressKeyHosts],
		msgs.IngressKeyClass:     row[msgs.IngressKeyClass],
		msgs.IngressKeyAge:       row[msgs.IngressKeyAge],
		msgs.IngressKeyBackends:  row[msgs.IngressKeyBackends],
	}
}

func nodeDisplayRow(row msgs.RowData, _ bool, colors map[string]color.Color) btable.RowData {
	return btable.RowData{
		msgs.NodeKeyContext:    contextCell(rowContext(row), colors),
		msgs.NodeKeyName:       row[msgs.NodeKeyName],
		msgs.NodeKeyStatus:     row[msgs.NodeKeyStatus],
		msgs.NodeKeyRoles:      row[msgs.NodeKeyRoles],
		msgs.NodeKeyAge:        row[msgs.NodeKeyAge],
		msgs.NodeKeyVersion:    row[msgs.NodeKeyVersion],
		msgs.NodeKeyInternalIP: row[msgs.NodeKeyInternalIP],
		msgs.NodeKeyOS:         row[msgs.NodeKeyOS],
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

	rows       []msgs.RowData
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
// pin the log pane to a specific pod.
func PodRowKey(row msgs.RowData) string {
	if row == nil {
		return ""
	}
	ctx, _ := row[msgs.KeyContext].(string)
	ns, _ := row[msgs.KeyNamespace].(string)
	name, _ := row[msgs.KeyName].(string)
	if ctx == "" && ns == "" && name == "" {
		return ""
	}
	return ctx + "/" + ns + "/" + name
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
	name, _ := t.rows[i][msgs.KeyName].(string)
	return strings.Contains(strings.ToLower(name), strings.ToLower(t.filter.query))
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
func (t *ResourceTable) activeRow(pos int) msgs.RowData {
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
func (t *ResourceTable) SetRows(rows []msgs.RowData) {
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
		display = append(display, btable.NewRow(t.spec.displayRow(row, t.checked[PodRowKey(row)], t.contextColors)))
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
func (t *ResourceTable) CheckedRow(key string) msgs.RowData {
	for _, row := range t.rows {
		if PodRowKey(row) == key {
			return row
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

func (t *ResourceTable) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	t.tableW, t.tableH = w, h
	t.wideMode = false

	compact := t.tier == views.TierCompact
	st := styles.CatppuccinBubbleTableStyle()
	t.table = newBubbleTable(t.spec.narrowColumns(compact)).
		WithMinimumHeight(h).
		WithTargetWidth(w).
		WithMaxTotalWidth(w).
		HeaderStyle(st.Header).
		HighlightStyle(st.Highlight).
		WithBaseStyle(st.Base).
		WithHorizontalFreezeColumnCount(t.spec.freezeColumns).
		Focused(t.focused)
	t.wideColCount = len(t.spec.narrowColumns(compact))
	t.scrollable = false
	t.windowSize = rowWindowSizeFor(h)
	t.windowStart = computeWindowStart(t.windowStart, t.cursorIdx, t.activeLen(), t.windowSize)
	t.pushDisplayRows()
	t.invalidateView()
}

// SelectedRow returns the raw (un-prefixed) row currently under the cursor,
// or nil if there are no rows. Raw rows are what callers should read
// resource identity out of — the table itself renders a display copy.
func (t *ResourceTable) SelectedRow() msgs.RowData {
	if t.cursorIdx < 0 || t.cursorIdx >= t.activeLen() {
		return nil
	}
	return t.activeRow(t.cursorIdx)
}

func (t *ResourceTable) invalidateView() {
	t.viewDirty = true
	t.cachedView = ""
}
