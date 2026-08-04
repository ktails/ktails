package models

import (
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

// rowFilter implements a k9s-style "/" full-list substring filter shared by
// the Pods/Deployments/svc tables. bubble-table has its own built-in "/"
// filter, but it only ever searches whatever was last passed to WithRows —
// which, since these tables now hand it a bounded window rather than every
// row (see rowWindowSizeFor), would silently search only the rows currently
// on screen. rowFilter instead tracks match *positions* into the owning
// table's full row set, so filtering and the window/cursor machinery both
// operate over the same "active index space": every row when no filter is
// active, or just the matches when one is.
type rowFilter struct {
	query     string
	filtering bool
	matches   []int
}

// active reports whether a filter is currently narrowing the row set, either
// mid-edit or already committed with Enter.
func (f *rowFilter) active() bool {
	return f.query != ""
}

// recompute rebuilds f.matches by scanning row positions [0,total) with
// matchFn, called whenever the query or the underlying row set changes.
func (f *rowFilter) recompute(total int, matchFn func(i int) bool) {
	if f.query == "" {
		f.matches = nil
		return
	}
	f.matches = f.matches[:0]
	for i := 0; i < total; i++ {
		if matchFn(i) {
			f.matches = append(f.matches, i)
		}
	}
}

// len returns how many rows are currently selectable: the full row count
// when no filter is active, or the match count otherwise.
func (f *rowFilter) len(total int) int {
	if !f.active() {
		return total
	}
	return len(f.matches)
}

// absolute maps a position in the active index space back to an absolute
// row index.
func (f *rowFilter) absolute(pos int) int {
	if !f.active() {
		return pos
	}
	return f.matches[pos]
}

// handleKey processes one keypress while filter-typing mode is on,
// mutating the query (and re-running matchFn) as needed. Enter commits the
// query and exits typing mode without changing it further; Esc clears the
// query entirely and exits typing mode; Backspace/printable text edit the
// query live.
func (f *rowFilter) handleKey(key tea.KeyPressMsg, total int, matchFn func(i int) bool) {
	switch key.String() {
	case "enter":
		f.filtering = false
	case "esc":
		f.filtering = false
		f.query = ""
		f.recompute(total, matchFn)
	case "backspace":
		if f.query != "" {
			r := []rune(f.query)
			f.query = string(r[:len(r)-1])
			f.recompute(total, matchFn)
		}
	default:
		if key.Text != "" {
			f.query += key.Text
			f.recompute(total, matchFn)
		}
	}
}

// newBubbleTable builds a bubble-table Model with the options common to
// every resource table: no pagination (the spec keeps today's continuous
// full-list scroll, not discrete pages), and no border — bubble-table draws
// a full grid border by default, which the old bubbles/table never had and
// which just duplicates the app's own pane border around the tab content.
func newBubbleTable(cols []btable.Column) btable.Model {
	return btable.New(cols).WithNoPagination().Border(btable.Border{})
}

// defaultRowWindowSize is the row-window size used before a table has ever
// had a real height (SetSize not yet called, or a degenerate height). It's
// also roughly what a typical terminal pane ends up showing.
const defaultRowWindowSize = 20

// rowWindowSizeFor returns how many rows a resource table's window should
// hold for a pane of height tableH. With WithNoPagination, bubble-table's
// VisibleIndices() always renders the *entire* row set it's given — not just
// whatever fits in WithMinimumHeight — so the window must be sized to what
// actually fits on screen, not just to "small enough to be fast": any window
// taller than the visible pane still overflows it and pushes the rest of the
// layout (including the bottom status bar) off-screen, just less severely
// than rendering every row would. tableH includes bubble-table's own header
// line, hence the -1.
func rowWindowSizeFor(tableH int) int {
	size := tableH - 1
	if size < 5 {
		return defaultRowWindowSize
	}
	return size
}

// computeWindowStart returns the start index of the windowSize-row window
// that should be visible for the given absolute cursor position. It scrolls
// by the minimum amount needed to keep the cursor in view — the same
// behavior as a normal scrolling list — rather than recentering, so the
// window only ever holds exactly as many rows as the pane can show.
func computeWindowStart(prevStart, cursor, total, windowSize int) int {
	if total <= windowSize {
		return 0
	}

	start := prevStart
	if cursor < start {
		start = cursor
	} else if cursor >= start+windowSize {
		start = cursor - windowSize + 1
	}

	if start < 0 {
		start = 0
	}
	if start > total-windowSize {
		start = total - windowSize
	}
	return start
}

// windowBounds returns the [start, end) slice bounds of the current window
// into a total-length row set, given the window's start index and size.
func windowBounds(start, total, windowSize int) (int, int) {
	if total <= windowSize {
		return 0, total
	}
	end := start + windowSize
	if end > total {
		end = total
	}
	return start, end
}

// checkColWidth is the content width (before padding) of the Pods checkbox
// column glyph. The old bubbles/table implementation applied one shared
// Padding(0,1) cell style across every column including the checkbox, so it
// needs the same padding here (via paddedColumn) to avoid rendering flush
// against the Name column.
const checkColWidth = 1

// columnPadStyle mirrors the old bubbles/table cell look (Padding(0,1)).
// bubble-table only honors padding set directly on a Column's style (see
// cell.go), not on the table's base style, so every data column applies it
// individually.
func columnPadStyle() lipgloss.Style {
	return lipgloss.NewStyle().Padding(0, 1)
}

// paddedColumn builds a fixed-width wide-mode column sized to contentWidth
// plus the 2 columns of horizontal padding columnPadStyle adds.
func paddedColumn(key, title string, contentWidth int) btable.Column {
	return btable.NewColumn(key, title, contentWidth+2).WithStyle(columnPadStyle())
}

func paddedFlexColumn(key, title string, flexFactor int) btable.Column {
	return btable.NewFlexColumn(key, title, flexFactor).WithStyle(columnPadStyle())
}

// widestValue returns the widest string found under key across rows,
// falling back to the header's own width — used to auto-fit wide-mode
// column widths to whatever data is currently loaded (recomputed on every
// SetRows, per the wide-mode spec).
func widestValue(rows []msgs.RowData, key, header string) int {
	widest := lipgloss.Width(header)
	for _, row := range rows {
		s, ok := row[key].(string)
		if !ok {
			continue
		}
		if w := lipgloss.Width(s); w > widest {
			widest = w
		}
	}
	return widest
}

// totalColumnsWidth sums rendered column widths plus the border overhead
// bubble-table itself adds (one column of border per column, plus one),
// mirroring its own recalculateWidth so callers can tell whether a set of
// wide-mode columns will overflow a given viewport width.
func totalColumnsWidth(cols []btable.Column) int {
	total := 0
	for _, c := range cols {
		total += c.Width()
	}
	return total + len(cols) + 1
}

// contextShortCodeLen is the fixed length of the Context column's short
// code (§3.3.1): long enough to disambiguate typical context names, short
// enough that the column never needs to compete with Name for width.
const contextShortCodeLen = 10

// contextColWidth is the Context column's fixed content width: a 1-cell
// identity dot, a space, then the short code. Every resource table uses
// this exact width via contextColumn so the column lines up across tabs.
const contextColWidth = 1 + 1 + contextShortCodeLen

// contextShortCode returns a fixed-length prefix of a context name for the
// Context column. This is a short code, not an ellipsis-truncation: the
// design spec forbids the context identifier ever ending in "…", so a
// longer name is simply cut to a fixed length with no truncation marker.
func contextShortCode(name string) string {
	if lipgloss.Width(name) <= contextShortCodeLen {
		return name
	}
	return string([]rune(name)[:contextShortCodeLen])
}

// contextColumn returns the Context column shared by every resource table:
// always the first data column (after the Pods checkbox), fixed-width, and
// never a flex column — it must never be squeezed or truncated the way
// Name is allowed to be.
func contextColumn(key string) btable.Column {
	return paddedColumn(key, "Context", contextColWidth)
}

// ansiForegroundReset is SGR 39 ("default foreground"), used in place of
// lipgloss's own full reset (SGR 0) after the Context column's identity
// dot — a full reset also clears whatever background the surrounding cell
// already set (e.g. the highlighted row's FocusColor fill), which silently
// breaks the highlight for every character after the dot. Resetting only
// the foreground leaves that background intact.
const ansiForegroundReset = "\x1b[39m"

// contextCell builds the Context column's cell content: an identity dot
// plus short code (state.AppState.AddContext assigns the colour once a
// context is confirmed; a context with no colour yet gets a hollow dot in
// neutral gray instead of guessing). Only the dot glyph itself carries the
// identity colour — the short code is left in whatever foreground the
// surrounding row/cell style already provides. This is deliberately
// self-contained: an identity hue rendered across a whole colored word
// clashes badly with the Blue focus-accent background on the highlighted
// row (see the "painful on the eye" report), but a single colored dot next
// to plain text doesn't compete with it the same way — so there's no need
// to know whether this particular row happens to be highlighted at all.
// neutralCellStyle is the zero-value style returned wherever a cell has no
// color to apply — a package var so callers share one immutable instance
// instead of each allocating their own equivalent empty lipgloss.Style.
var neutralCellStyle = lipgloss.NewStyle()

// contextDotStyles maps every color a context identity dot can ever be
// rendered in — styles.IdentityColors' full rotation, plus Overlay1 for a
// context with no color assigned yet (state.AppState.AddContext always
// assigns from one of these) — to its lipgloss.Style, built once rather
// than per contextCell call: every row of every kind's table calls this on
// every render.
var contextDotStyles = func() map[color.Color]lipgloss.Style {
	m := make(map[color.Color]lipgloss.Style, len(styles.IdentityColors)+1)
	for _, c := range styles.IdentityColors {
		m[c] = lipgloss.NewStyle().Foreground(c)
	}
	m[styles.CatppuccinMocha().Overlay1] = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay1)
	return m
}()

func contextCell(name string, colors map[string]color.Color) btable.StyledCell {
	col, hasColor := colors[name]
	dot := "●"
	if !hasColor {
		dot = "○"
		col = styles.CatppuccinMocha().Overlay1
	}
	style, ok := contextDotStyles[col]
	if !ok {
		// Shouldn't happen — every color reaching here comes from
		// styles.IdentityColors or the Overlay1 fallback above — but fall
		// back to building the style rather than rendering with no color.
		style = lipgloss.NewStyle().Foreground(col)
	}
	rendered := style.Render(dot)
	rendered = strings.TrimSuffix(rendered, ansi.ResetStyle) + ansiForegroundReset
	text := rendered + " " + contextShortCode(name)
	return btable.NewStyledCell(text, neutralCellStyle)
}

// statusCellStyles maps a pod phase (PodInfo.Status) to its Catppuccin
// Mocha status color, per the Status Colors spec: Running=Green,
// Pending=Yellow, Failed/Unknown=Red, Succeeded=Overlay1 (dim).
// Unrecognized phases fall back to neutralCellStyle. Built once rather than
// per statusCellStyle call — bubble-table invokes this on every Status cell
// of every render.
var statusCellStyles = map[string]lipgloss.Style{
	"Running":   lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Green),
	"Pending":   lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Yellow),
	"Failed":    lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Red),
	"Unknown":   lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Red),
	"Succeeded": lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay1),
}

// statusCellStyle is a btable.StyledCellFunc that colors the Status cell by
// phase (see statusCellStyles). Cell style is applied by bubble-table at
// render time, after content-width truncation — the whole reason this
// migration dropped the old post-render ANSI-recoloring workaround.
func statusCellStyle(input btable.StyledCellFuncInput) lipgloss.Style {
	status, _ := input.Data.(string)
	if style, ok := statusCellStyles[status]; ok {
		return style
	}
	return neutralCellStyle
}

// Replica-ratio cell styles, built once — see statusCellStyles.
var (
	replicaCellStyleRed    = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Red)
	replicaCellStyleYellow = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Yellow)
	replicaCellStyleGreen  = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Green)
)

// replicaCellStyle is a btable.StyledCellFunc that colors a "ready/desired"
// replica cell (as produced by the Deployments watch cache's Rows): green when fully
// ready, yellow when partially ready, red when zero replicas are ready but
// some are desired.
func replicaCellStyle(input btable.StyledCellFuncInput) lipgloss.Style {
	cell, _ := input.Data.(string)
	ready, desired, ok := strings.Cut(cell, "/")
	if !ok {
		return neutralCellStyle
	}
	readyN, err := strconv.Atoi(ready)
	if err != nil {
		return neutralCellStyle
	}
	desiredN, err := strconv.Atoi(desired)
	if err != nil {
		return neutralCellStyle
	}

	switch {
	case readyN == desiredN:
		return replicaCellStyleGreen
	case readyN > 0:
		return replicaCellStyleYellow
	default:
		return replicaCellStyleRed
	}
}

// colKind selects which of the four column constructors a colEntry builds:
// colCheck (Pods' checkbox glyph, fixed checkColWidth in both modes),
// colContext (the shared Context column, fixed contextColWidth), or colData
// (every other column — flex-sized in narrow mode via flex, auto-fit to
// loaded data in wide mode via widestValue).
type colKind int

const (
	colData colKind = iota
	colCheck
	colContext
)

// colEntry is one column's declarative spec, in the exact order it should
// appear. flex and dropWhenCompact are narrow-mode-only (ignored by
// buildWideColumns): flex is the narrow-mode flex factor for a colData
// entry; dropWhenCompact marks the one column — always Age, by convention
// across every kind below — that TierCompact drops (§8.3; see
// buildNarrowColumns).
type colEntry struct {
	kind            colKind
	key             string
	title           string
	flex            int
	dropWhenCompact bool
}

// buildNarrowColumns and every *NarrowColumns function below share one
// column order (§3.4): Context (fixed, never truncated), Namespace, Name,
// then resource-specific columns — Nodes excepted, since it's
// cluster-scoped and carries no Namespace at all. dropWhenCompact
// implements the column-priority order in §8.3: Context and the resource's
// status-ish column never drop; Age is the lowest-priority fixed column and
// the only one dropped when compact is true (TierCompact — see
// views.WidthTier). Age's position in the entry list varies per kind (it's
// not always last — several kinds append one more column after it), which
// this preserves simply by each kind's entries being listed in the actual
// desired output order.
func buildNarrowColumns(entries []colEntry, compact bool) []btable.Column {
	cols := make([]btable.Column, 0, len(entries))
	for _, e := range entries {
		if e.dropWhenCompact && compact {
			continue
		}
		switch e.kind {
		case colCheck:
			cols = append(cols, paddedColumn(e.key, e.title, checkColWidth))
		case colContext:
			cols = append(cols, contextColumn(e.key))
		default:
			cols = append(cols, paddedFlexColumn(e.key, e.title, e.flex))
		}
	}
	return cols
}

// buildWideColumns is buildNarrowColumns' wide-mode counterpart: every
// colData entry auto-fits to whatever's currently loaded (widestValue)
// instead of flexing, and nothing is ever dropped (there's no compact
// concept in wide mode).
func buildWideColumns(entries []colEntry, rows []msgs.RowData) []btable.Column {
	cols := make([]btable.Column, 0, len(entries))
	for _, e := range entries {
		switch e.kind {
		case colCheck:
			cols = append(cols, paddedColumn(e.key, e.title, checkColWidth))
		case colContext:
			cols = append(cols, contextColumn(e.key))
		default:
			cols = append(cols, paddedColumn(e.key, e.title, widestValue(rows, e.key, e.title)))
		}
	}
	return cols
}

var podNarrowSpec = []colEntry{
	{kind: colCheck, key: msgs.PodKeyCheck, title: "✓"},
	{kind: colContext, key: msgs.PodKeyContext},
	{key: msgs.PodKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.PodKeyName, title: "Name", flex: 10},
	{key: msgs.PodKeyStatus, title: "Status", flex: 4},
	{key: msgs.PodKeyRestarts, title: "Restarts", flex: 3},
	{key: msgs.PodKeyCPU, title: "CPU", flex: 3},
	{key: msgs.PodKeyMemory, title: "Memory", flex: 3},
	{key: msgs.PodKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var podWideSpec = []colEntry{
	{kind: colCheck, key: msgs.PodKeyCheck, title: "✓"},
	{kind: colContext, key: msgs.PodKeyContext},
	{key: msgs.PodKeyNamespace, title: "Namespace"},
	{key: msgs.PodKeyName, title: "Name"},
	{key: msgs.PodKeyStatus, title: "Status"},
	{key: msgs.PodKeyReady, title: "Ready"},
	{key: msgs.PodKeyRestarts, title: "Restarts"},
	{key: msgs.PodKeyAge, title: "Age"},
	{key: msgs.PodKeyNode, title: "Node"},
	{key: msgs.PodKeyNodeIP, title: "Node IP"},
	{key: msgs.PodKeyPodIP, title: "Pod IP"},
	{key: msgs.PodKeyCPU, title: "CPU"},
	{key: msgs.PodKeyMemory, title: "Memory"},
}

func podNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(podNarrowSpec, compact)
}
func podWideColumns(rows []msgs.RowData) []btable.Column { return buildWideColumns(podWideSpec, rows) }

var deploymentNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.DeployKeyContext},
	{key: msgs.DeployKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.DeployKeyName, title: "Name", flex: 8},
	{key: msgs.DeployKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
	{key: msgs.DeployKeyReplicas, title: "ReadyReplicas", flex: 4},
}

var deploymentWideSpec = []colEntry{
	{kind: colContext, key: msgs.DeployKeyContext},
	{key: msgs.DeployKeyNamespace, title: "Namespace"},
	{key: msgs.DeployKeyName, title: "Name"},
	{key: msgs.DeployKeyAge, title: "Age"},
	{key: msgs.DeployKeyReplicas, title: "ReadyReplicas"},
	{key: msgs.DeployKeyAvailable, title: "Available"},
	{key: msgs.DeployKeyUpdated, title: "Updated"},
	{key: msgs.DeployKeyStrategy, title: "Strategy"},
	{key: msgs.DeployKeySelector, title: "Selector"},
}

func deploymentNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(deploymentNarrowSpec, compact)
}
func deploymentWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(deploymentWideSpec, rows)
}

var svcNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.SvcKeyContext},
	{key: msgs.SvcKeyNamespace, title: "Namespace", flex: 18},
	{key: msgs.SvcKeyName, title: "Name", flex: 28},
	{key: msgs.SvcKeyType, title: "Type", flex: 14},
	{key: msgs.SvcKeyClusterIP, title: "ClusterIP", flex: 18},
	{key: msgs.SvcKeyPorts, title: "Ports", flex: 12},
	{key: msgs.SvcKeyAge, title: "Age", flex: 10, dropWhenCompact: true},
}

var svcWideSpec = []colEntry{
	{kind: colContext, key: msgs.SvcKeyContext},
	{key: msgs.SvcKeyNamespace, title: "Namespace"},
	{key: msgs.SvcKeyName, title: "Name"},
	{key: msgs.SvcKeyType, title: "Type"},
	{key: msgs.SvcKeyClusterIP, title: "ClusterIP"},
	{key: msgs.SvcKeyPorts, title: "Ports"},
	{key: msgs.SvcKeyAge, title: "Age"},
	{key: msgs.SvcKeySelector, title: "Selector"},
	{key: msgs.SvcKeyExternalIP, title: "ExternalIP"},
	{key: msgs.SvcKeyEndpointIPs, title: "EndpointIPs"},
}

func svcNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(svcNarrowSpec, compact)
}
func svcWideColumns(rows []msgs.RowData) []btable.Column { return buildWideColumns(svcWideSpec, rows) }

var configMapNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.ConfigMapKeyContext},
	{key: msgs.ConfigMapKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.ConfigMapKeyName, title: "Name", flex: 10},
	{key: msgs.ConfigMapKeyKeys, title: "Keys", flex: 2},
	{key: msgs.ConfigMapKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var configMapWideSpec = []colEntry{
	{kind: colContext, key: msgs.ConfigMapKeyContext},
	{key: msgs.ConfigMapKeyNamespace, title: "Namespace"},
	{key: msgs.ConfigMapKeyName, title: "Name"},
	{key: msgs.ConfigMapKeyKeys, title: "Keys"},
	{key: msgs.ConfigMapKeyAge, title: "Age"},
	{key: msgs.ConfigMapKeyKeyNames, title: "Data Keys"},
}

func configMapNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(configMapNarrowSpec, compact)
}
func configMapWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(configMapWideSpec, rows)
}

var secretNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.SecretKeyContext},
	{key: msgs.SecretKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.SecretKeyName, title: "Name", flex: 10},
	{key: msgs.SecretKeyType, title: "Type", flex: 6},
	{key: msgs.SecretKeyKeys, title: "Keys", flex: 2},
	{key: msgs.SecretKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var secretWideSpec = []colEntry{
	{kind: colContext, key: msgs.SecretKeyContext},
	{key: msgs.SecretKeyNamespace, title: "Namespace"},
	{key: msgs.SecretKeyName, title: "Name"},
	{key: msgs.SecretKeyType, title: "Type"},
	{key: msgs.SecretKeyKeys, title: "Keys"},
	{key: msgs.SecretKeyAge, title: "Age"},
}

func secretNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(secretNarrowSpec, compact)
}
func secretWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(secretWideSpec, rows)
}

var jobNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.JobKeyContext},
	{key: msgs.JobKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.JobKeyName, title: "Name", flex: 10},
	{key: msgs.JobKeyCompletions, title: "Completions", flex: 3},
	{key: msgs.JobKeyStatus, title: "Status", flex: 4},
	{key: msgs.JobKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var jobWideSpec = []colEntry{
	{kind: colContext, key: msgs.JobKeyContext},
	{key: msgs.JobKeyNamespace, title: "Namespace"},
	{key: msgs.JobKeyName, title: "Name"},
	{key: msgs.JobKeyCompletions, title: "Completions"},
	{key: msgs.JobKeyStatus, title: "Status"},
	{key: msgs.JobKeyDuration, title: "Duration"},
	{key: msgs.JobKeyAge, title: "Age"},
}

func jobNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(jobNarrowSpec, compact)
}
func jobWideColumns(rows []msgs.RowData) []btable.Column { return buildWideColumns(jobWideSpec, rows) }

var cronJobNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.CronJobKeyContext},
	{key: msgs.CronJobKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.CronJobKeyName, title: "Name", flex: 10},
	{key: msgs.CronJobKeySchedule, title: "Schedule", flex: 5},
	{key: msgs.CronJobKeySuspend, title: "Suspend", flex: 3},
	{key: msgs.CronJobKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var cronJobWideSpec = []colEntry{
	{kind: colContext, key: msgs.CronJobKeyContext},
	{key: msgs.CronJobKeyNamespace, title: "Namespace"},
	{key: msgs.CronJobKeyName, title: "Name"},
	{key: msgs.CronJobKeySchedule, title: "Schedule"},
	{key: msgs.CronJobKeySuspend, title: "Suspend"},
	{key: msgs.CronJobKeyLastScheduled, title: "Last Scheduled"},
	{key: msgs.CronJobKeyAge, title: "Age"},
}

func cronJobNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(cronJobNarrowSpec, compact)
}
func cronJobWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(cronJobWideSpec, rows)
}

var statefulSetNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.StatefulSetKeyContext},
	{key: msgs.StatefulSetKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.StatefulSetKeyName, title: "Name", flex: 8},
	{key: msgs.StatefulSetKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
	{key: msgs.StatefulSetKeyReady, title: "Ready", flex: 4},
}

var statefulSetWideSpec = []colEntry{
	{kind: colContext, key: msgs.StatefulSetKeyContext},
	{key: msgs.StatefulSetKeyNamespace, title: "Namespace"},
	{key: msgs.StatefulSetKeyName, title: "Name"},
	{key: msgs.StatefulSetKeyAge, title: "Age"},
	{key: msgs.StatefulSetKeyReady, title: "Ready"},
	{key: msgs.StatefulSetKeySelector, title: "Selector"},
}

func statefulSetNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(statefulSetNarrowSpec, compact)
}
func statefulSetWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(statefulSetWideSpec, rows)
}

var daemonSetNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.DaemonSetKeyContext},
	{key: msgs.DaemonSetKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.DaemonSetKeyName, title: "Name", flex: 8},
	{key: msgs.DaemonSetKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
	{key: msgs.DaemonSetKeyReady, title: "Ready", flex: 4},
}

var daemonSetWideSpec = []colEntry{
	{kind: colContext, key: msgs.DaemonSetKeyContext},
	{key: msgs.DaemonSetKeyNamespace, title: "Namespace"},
	{key: msgs.DaemonSetKeyName, title: "Name"},
	{key: msgs.DaemonSetKeyAge, title: "Age"},
	{key: msgs.DaemonSetKeyReady, title: "Ready"},
	{key: msgs.DaemonSetKeySelector, title: "Selector"},
}

func daemonSetNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(daemonSetNarrowSpec, compact)
}
func daemonSetWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(daemonSetWideSpec, rows)
}

var ingressNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.IngressKeyContext},
	{key: msgs.IngressKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.IngressKeyName, title: "Name", flex: 10},
	{key: msgs.IngressKeyHosts, title: "Hosts", flex: 10},
	{key: msgs.IngressKeyClass, title: "Class", flex: 4},
	{key: msgs.IngressKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var ingressWideSpec = []colEntry{
	{kind: colContext, key: msgs.IngressKeyContext},
	{key: msgs.IngressKeyNamespace, title: "Namespace"},
	{key: msgs.IngressKeyName, title: "Name"},
	{key: msgs.IngressKeyHosts, title: "Hosts"},
	{key: msgs.IngressKeyClass, title: "Class"},
	{key: msgs.IngressKeyAge, title: "Age"},
	{key: msgs.IngressKeyBackends, title: "Backends"},
}

func ingressNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(ingressNarrowSpec, compact)
}
func ingressWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(ingressWideSpec, rows)
}

var pdbNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.PDBKeyContext},
	{key: msgs.PDBKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.PDBKeyName, title: "Name", flex: 10},
	{key: msgs.PDBKeyMinMaxAvailable, title: "Min/Max Available", flex: 6},
	{key: msgs.PDBKeyAllowedDisruptions, title: "Allowed Disruptions", flex: 4},
	{key: msgs.PDBKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var pdbWideSpec = []colEntry{
	{kind: colContext, key: msgs.PDBKeyContext},
	{key: msgs.PDBKeyNamespace, title: "Namespace"},
	{key: msgs.PDBKeyName, title: "Name"},
	{key: msgs.PDBKeyMinMaxAvailable, title: "Min/Max Available"},
	{key: msgs.PDBKeyAllowedDisruptions, title: "Allowed Disruptions"},
	{key: msgs.PDBKeyAge, title: "Age"},
	{key: msgs.PDBKeyCurrentHealthy, title: "Current Healthy"},
	{key: msgs.PDBKeyDesiredHealthy, title: "Desired Healthy"},
}

func pdbNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(pdbNarrowSpec, compact)
}
func pdbWideColumns(rows []msgs.RowData) []btable.Column { return buildWideColumns(pdbWideSpec, rows) }

var hpaNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.HPAKeyContext},
	{key: msgs.HPAKeyNamespace, title: "Namespace", flex: 5},
	{key: msgs.HPAKeyName, title: "Name", flex: 10},
	{key: msgs.HPAKeyReference, title: "Reference", flex: 8},
	{key: msgs.HPAKeyMinMax, title: "Min-Max", flex: 3},
	{key: msgs.HPAKeyReplicas, title: "Replicas", flex: 3},
	{key: msgs.HPAKeyTargets, title: "Targets", flex: 6},
	{key: msgs.HPAKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
}

var hpaWideSpec = []colEntry{
	{kind: colContext, key: msgs.HPAKeyContext},
	{key: msgs.HPAKeyNamespace, title: "Namespace"},
	{key: msgs.HPAKeyName, title: "Name"},
	{key: msgs.HPAKeyReference, title: "Reference"},
	{key: msgs.HPAKeyMinMax, title: "Min-Max"},
	{key: msgs.HPAKeyReplicas, title: "Replicas"},
	{key: msgs.HPAKeyTargets, title: "Targets"},
	{key: msgs.HPAKeyAge, title: "Age"},
}

func hpaNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(hpaNarrowSpec, compact)
}
func hpaWideColumns(rows []msgs.RowData) []btable.Column { return buildWideColumns(hpaWideSpec, rows) }

// nodeNarrowSpec/nodeWideSpec omit Namespace: Nodes are cluster-scoped (see
// msgs.NodeKeyName's doc comment), so Context is followed directly by Name
// rather than a Namespace column that would always read empty.
var nodeNarrowSpec = []colEntry{
	{kind: colContext, key: msgs.NodeKeyContext},
	{key: msgs.NodeKeyName, title: "Name", flex: 12},
	{key: msgs.NodeKeyStatus, title: "Status", flex: 4},
	{key: msgs.NodeKeyRoles, title: "Roles", flex: 5},
	{key: msgs.NodeKeyCPU, title: "CPU", flex: 3},
	{key: msgs.NodeKeyMemory, title: "Memory", flex: 3},
	{key: msgs.NodeKeyAge, title: "Age", flex: 3, dropWhenCompact: true},
	{key: msgs.NodeKeyVersion, title: "Version", flex: 4},
}

var nodeWideSpec = []colEntry{
	{kind: colContext, key: msgs.NodeKeyContext},
	{key: msgs.NodeKeyName, title: "Name"},
	{key: msgs.NodeKeyStatus, title: "Status"},
	{key: msgs.NodeKeyRoles, title: "Roles"},
	{key: msgs.NodeKeyAge, title: "Age"},
	{key: msgs.NodeKeyVersion, title: "Version"},
	{key: msgs.NodeKeyInternalIP, title: "InternalIP"},
	{key: msgs.NodeKeyOS, title: "OS/Arch"},
	{key: msgs.NodeKeyCPU, title: "CPU"},
	{key: msgs.NodeKeyMemory, title: "Memory"},
}

func nodeNarrowColumns(compact bool) []btable.Column {
	return buildNarrowColumns(nodeNarrowSpec, compact)
}
func nodeWideColumns(rows []msgs.RowData) []btable.Column {
	return buildWideColumns(nodeWideSpec, rows)
}
