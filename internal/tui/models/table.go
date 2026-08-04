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

// podNarrowColumns and every other *NarrowColumns/*WideColumns function
// below share one column order (§3.4): Context (fixed, never truncated),
// Namespace, Name, then resource-specific columns — Nodes excepted, since
// it's cluster-scoped and carries no Namespace at all. Each *NarrowColumns
// function also follows the column-priority order in §8.3: Context and the
// resource's status-ish column never drop; Age is the lowest-priority fixed
// column and the first (here, only) one dropped when compact is true
// (TierCompact — see views.WidthTier).
func appendAge(cols []btable.Column, compact bool, ageCol btable.Column) []btable.Column {
	if compact {
		return cols
	}
	return append(cols, ageCol)
}

func podNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		paddedColumn(msgs.PodKeyCheck, "✓", checkColWidth),
		contextColumn(msgs.PodKeyContext),
		paddedFlexColumn(msgs.PodKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.PodKeyName, "Name", 10),
		paddedFlexColumn(msgs.PodKeyStatus, "Status", 4),
		paddedFlexColumn(msgs.PodKeyRestarts, "Restarts", 3),
		paddedFlexColumn(msgs.PodKeyCPU, "CPU", 3),
		paddedFlexColumn(msgs.PodKeyMemory, "Memory", 3),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.PodKeyAge, "Age", 3))
}

func podWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		paddedColumn(msgs.PodKeyCheck, "✓", checkColWidth),
		contextColumn(msgs.PodKeyContext),
		paddedColumn(msgs.PodKeyNamespace, "Namespace", widestValue(rows, msgs.PodKeyNamespace, "Namespace")),
		paddedColumn(msgs.PodKeyName, "Name", widestValue(rows, msgs.PodKeyName, "Name")),
		paddedColumn(msgs.PodKeyStatus, "Status", widestValue(rows, msgs.PodKeyStatus, "Status")),
		paddedColumn(msgs.PodKeyReady, "Ready", widestValue(rows, msgs.PodKeyReady, "Ready")),
		paddedColumn(msgs.PodKeyRestarts, "Restarts", widestValue(rows, msgs.PodKeyRestarts, "Restarts")),
		paddedColumn(msgs.PodKeyAge, "Age", widestValue(rows, msgs.PodKeyAge, "Age")),
		paddedColumn(msgs.PodKeyNode, "Node", widestValue(rows, msgs.PodKeyNode, "Node")),
		paddedColumn(msgs.PodKeyNodeIP, "Node IP", widestValue(rows, msgs.PodKeyNodeIP, "Node IP")),
		paddedColumn(msgs.PodKeyPodIP, "Pod IP", widestValue(rows, msgs.PodKeyPodIP, "Pod IP")),
		paddedColumn(msgs.PodKeyCPU, "CPU", widestValue(rows, msgs.PodKeyCPU, "CPU")),
		paddedColumn(msgs.PodKeyMemory, "Memory", widestValue(rows, msgs.PodKeyMemory, "Memory")),
	}
}

func deploymentNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.DeployKeyContext),
		paddedFlexColumn(msgs.DeployKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.DeployKeyName, "Name", 8),
	}
	cols = appendAge(cols, compact, paddedFlexColumn(msgs.DeployKeyAge, "Age", 3))
	return append(cols, paddedFlexColumn(msgs.DeployKeyReplicas, "ReadyReplicas", 4))
}

func deploymentWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.DeployKeyContext),
		paddedColumn(msgs.DeployKeyNamespace, "Namespace", widestValue(rows, msgs.DeployKeyNamespace, "Namespace")),
		paddedColumn(msgs.DeployKeyName, "Name", widestValue(rows, msgs.DeployKeyName, "Name")),
		paddedColumn(msgs.DeployKeyAge, "Age", widestValue(rows, msgs.DeployKeyAge, "Age")),
		paddedColumn(msgs.DeployKeyReplicas, "ReadyReplicas", widestValue(rows, msgs.DeployKeyReplicas, "ReadyReplicas")),
		paddedColumn(msgs.DeployKeyAvailable, "Available", widestValue(rows, msgs.DeployKeyAvailable, "Available")),
		paddedColumn(msgs.DeployKeyUpdated, "Updated", widestValue(rows, msgs.DeployKeyUpdated, "Updated")),
		paddedColumn(msgs.DeployKeyStrategy, "Strategy", widestValue(rows, msgs.DeployKeyStrategy, "Strategy")),
		paddedColumn(msgs.DeployKeySelector, "Selector", widestValue(rows, msgs.DeployKeySelector, "Selector")),
	}
}

func svcNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.SvcKeyContext),
		paddedFlexColumn(msgs.SvcKeyNamespace, "Namespace", 18),
		paddedFlexColumn(msgs.SvcKeyName, "Name", 28),
		paddedFlexColumn(msgs.SvcKeyType, "Type", 14),
		paddedFlexColumn(msgs.SvcKeyClusterIP, "ClusterIP", 18),
		paddedFlexColumn(msgs.SvcKeyPorts, "Ports", 12),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.SvcKeyAge, "Age", 10))
}

func svcWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.SvcKeyContext),
		paddedColumn(msgs.SvcKeyNamespace, "Namespace", widestValue(rows, msgs.SvcKeyNamespace, "Namespace")),
		paddedColumn(msgs.SvcKeyName, "Name", widestValue(rows, msgs.SvcKeyName, "Name")),
		paddedColumn(msgs.SvcKeyType, "Type", widestValue(rows, msgs.SvcKeyType, "Type")),
		paddedColumn(msgs.SvcKeyClusterIP, "ClusterIP", widestValue(rows, msgs.SvcKeyClusterIP, "ClusterIP")),
		paddedColumn(msgs.SvcKeyPorts, "Ports", widestValue(rows, msgs.SvcKeyPorts, "Ports")),
		paddedColumn(msgs.SvcKeyAge, "Age", widestValue(rows, msgs.SvcKeyAge, "Age")),
		paddedColumn(msgs.SvcKeySelector, "Selector", widestValue(rows, msgs.SvcKeySelector, "Selector")),
		paddedColumn(msgs.SvcKeyExternalIP, "ExternalIP", widestValue(rows, msgs.SvcKeyExternalIP, "ExternalIP")),
		paddedColumn(msgs.SvcKeyEndpointIPs, "EndpointIPs", widestValue(rows, msgs.SvcKeyEndpointIPs, "EndpointIPs")),
	}
}

func configMapNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.ConfigMapKeyContext),
		paddedFlexColumn(msgs.ConfigMapKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.ConfigMapKeyName, "Name", 10),
		paddedFlexColumn(msgs.ConfigMapKeyKeys, "Keys", 2),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.ConfigMapKeyAge, "Age", 3))
}

func configMapWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.ConfigMapKeyContext),
		paddedColumn(msgs.ConfigMapKeyNamespace, "Namespace", widestValue(rows, msgs.ConfigMapKeyNamespace, "Namespace")),
		paddedColumn(msgs.ConfigMapKeyName, "Name", widestValue(rows, msgs.ConfigMapKeyName, "Name")),
		paddedColumn(msgs.ConfigMapKeyKeys, "Keys", widestValue(rows, msgs.ConfigMapKeyKeys, "Keys")),
		paddedColumn(msgs.ConfigMapKeyAge, "Age", widestValue(rows, msgs.ConfigMapKeyAge, "Age")),
		paddedColumn(msgs.ConfigMapKeyKeyNames, "Data Keys", widestValue(rows, msgs.ConfigMapKeyKeyNames, "Data Keys")),
	}
}

func secretNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.SecretKeyContext),
		paddedFlexColumn(msgs.SecretKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.SecretKeyName, "Name", 10),
		paddedFlexColumn(msgs.SecretKeyType, "Type", 6),
		paddedFlexColumn(msgs.SecretKeyKeys, "Keys", 2),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.SecretKeyAge, "Age", 3))
}

func secretWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.SecretKeyContext),
		paddedColumn(msgs.SecretKeyNamespace, "Namespace", widestValue(rows, msgs.SecretKeyNamespace, "Namespace")),
		paddedColumn(msgs.SecretKeyName, "Name", widestValue(rows, msgs.SecretKeyName, "Name")),
		paddedColumn(msgs.SecretKeyType, "Type", widestValue(rows, msgs.SecretKeyType, "Type")),
		paddedColumn(msgs.SecretKeyKeys, "Keys", widestValue(rows, msgs.SecretKeyKeys, "Keys")),
		paddedColumn(msgs.SecretKeyAge, "Age", widestValue(rows, msgs.SecretKeyAge, "Age")),
	}
}

func jobNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.JobKeyContext),
		paddedFlexColumn(msgs.JobKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.JobKeyName, "Name", 10),
		paddedFlexColumn(msgs.JobKeyCompletions, "Completions", 3),
		paddedFlexColumn(msgs.JobKeyStatus, "Status", 4),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.JobKeyAge, "Age", 3))
}

func jobWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.JobKeyContext),
		paddedColumn(msgs.JobKeyNamespace, "Namespace", widestValue(rows, msgs.JobKeyNamespace, "Namespace")),
		paddedColumn(msgs.JobKeyName, "Name", widestValue(rows, msgs.JobKeyName, "Name")),
		paddedColumn(msgs.JobKeyCompletions, "Completions", widestValue(rows, msgs.JobKeyCompletions, "Completions")),
		paddedColumn(msgs.JobKeyStatus, "Status", widestValue(rows, msgs.JobKeyStatus, "Status")),
		paddedColumn(msgs.JobKeyDuration, "Duration", widestValue(rows, msgs.JobKeyDuration, "Duration")),
		paddedColumn(msgs.JobKeyAge, "Age", widestValue(rows, msgs.JobKeyAge, "Age")),
	}
}

func cronJobNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.CronJobKeyContext),
		paddedFlexColumn(msgs.CronJobKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.CronJobKeyName, "Name", 10),
		paddedFlexColumn(msgs.CronJobKeySchedule, "Schedule", 5),
		paddedFlexColumn(msgs.CronJobKeySuspend, "Suspend", 3),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.CronJobKeyAge, "Age", 3))
}

func cronJobWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.CronJobKeyContext),
		paddedColumn(msgs.CronJobKeyNamespace, "Namespace", widestValue(rows, msgs.CronJobKeyNamespace, "Namespace")),
		paddedColumn(msgs.CronJobKeyName, "Name", widestValue(rows, msgs.CronJobKeyName, "Name")),
		paddedColumn(msgs.CronJobKeySchedule, "Schedule", widestValue(rows, msgs.CronJobKeySchedule, "Schedule")),
		paddedColumn(msgs.CronJobKeySuspend, "Suspend", widestValue(rows, msgs.CronJobKeySuspend, "Suspend")),
		paddedColumn(msgs.CronJobKeyLastScheduled, "Last Scheduled", widestValue(rows, msgs.CronJobKeyLastScheduled, "Last Scheduled")),
		paddedColumn(msgs.CronJobKeyAge, "Age", widestValue(rows, msgs.CronJobKeyAge, "Age")),
	}
}

func statefulSetNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.StatefulSetKeyContext),
		paddedFlexColumn(msgs.StatefulSetKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.StatefulSetKeyName, "Name", 8),
	}
	cols = appendAge(cols, compact, paddedFlexColumn(msgs.StatefulSetKeyAge, "Age", 3))
	return append(cols, paddedFlexColumn(msgs.StatefulSetKeyReady, "Ready", 4))
}

func statefulSetWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.StatefulSetKeyContext),
		paddedColumn(msgs.StatefulSetKeyNamespace, "Namespace", widestValue(rows, msgs.StatefulSetKeyNamespace, "Namespace")),
		paddedColumn(msgs.StatefulSetKeyName, "Name", widestValue(rows, msgs.StatefulSetKeyName, "Name")),
		paddedColumn(msgs.StatefulSetKeyAge, "Age", widestValue(rows, msgs.StatefulSetKeyAge, "Age")),
		paddedColumn(msgs.StatefulSetKeyReady, "Ready", widestValue(rows, msgs.StatefulSetKeyReady, "Ready")),
		paddedColumn(msgs.StatefulSetKeySelector, "Selector", widestValue(rows, msgs.StatefulSetKeySelector, "Selector")),
	}
}

func daemonSetNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.DaemonSetKeyContext),
		paddedFlexColumn(msgs.DaemonSetKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.DaemonSetKeyName, "Name", 8),
	}
	cols = appendAge(cols, compact, paddedFlexColumn(msgs.DaemonSetKeyAge, "Age", 3))
	return append(cols, paddedFlexColumn(msgs.DaemonSetKeyReady, "Ready", 4))
}

func daemonSetWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.DaemonSetKeyContext),
		paddedColumn(msgs.DaemonSetKeyNamespace, "Namespace", widestValue(rows, msgs.DaemonSetKeyNamespace, "Namespace")),
		paddedColumn(msgs.DaemonSetKeyName, "Name", widestValue(rows, msgs.DaemonSetKeyName, "Name")),
		paddedColumn(msgs.DaemonSetKeyAge, "Age", widestValue(rows, msgs.DaemonSetKeyAge, "Age")),
		paddedColumn(msgs.DaemonSetKeyReady, "Ready", widestValue(rows, msgs.DaemonSetKeyReady, "Ready")),
		paddedColumn(msgs.DaemonSetKeySelector, "Selector", widestValue(rows, msgs.DaemonSetKeySelector, "Selector")),
	}
}

func ingressNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.IngressKeyContext),
		paddedFlexColumn(msgs.IngressKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.IngressKeyName, "Name", 10),
		paddedFlexColumn(msgs.IngressKeyHosts, "Hosts", 10),
		paddedFlexColumn(msgs.IngressKeyClass, "Class", 4),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.IngressKeyAge, "Age", 3))
}

func ingressWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.IngressKeyContext),
		paddedColumn(msgs.IngressKeyNamespace, "Namespace", widestValue(rows, msgs.IngressKeyNamespace, "Namespace")),
		paddedColumn(msgs.IngressKeyName, "Name", widestValue(rows, msgs.IngressKeyName, "Name")),
		paddedColumn(msgs.IngressKeyHosts, "Hosts", widestValue(rows, msgs.IngressKeyHosts, "Hosts")),
		paddedColumn(msgs.IngressKeyClass, "Class", widestValue(rows, msgs.IngressKeyClass, "Class")),
		paddedColumn(msgs.IngressKeyAge, "Age", widestValue(rows, msgs.IngressKeyAge, "Age")),
		paddedColumn(msgs.IngressKeyBackends, "Backends", widestValue(rows, msgs.IngressKeyBackends, "Backends")),
	}
}

func pdbNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.PDBKeyContext),
		paddedFlexColumn(msgs.PDBKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.PDBKeyName, "Name", 10),
		paddedFlexColumn(msgs.PDBKeyMinMaxAvailable, "Min/Max Available", 6),
		paddedFlexColumn(msgs.PDBKeyAllowedDisruptions, "Allowed Disruptions", 4),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.PDBKeyAge, "Age", 3))
}

func pdbWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.PDBKeyContext),
		paddedColumn(msgs.PDBKeyNamespace, "Namespace", widestValue(rows, msgs.PDBKeyNamespace, "Namespace")),
		paddedColumn(msgs.PDBKeyName, "Name", widestValue(rows, msgs.PDBKeyName, "Name")),
		paddedColumn(msgs.PDBKeyMinMaxAvailable, "Min/Max Available", widestValue(rows, msgs.PDBKeyMinMaxAvailable, "Min/Max Available")),
		paddedColumn(msgs.PDBKeyAllowedDisruptions, "Allowed Disruptions", widestValue(rows, msgs.PDBKeyAllowedDisruptions, "Allowed Disruptions")),
		paddedColumn(msgs.PDBKeyAge, "Age", widestValue(rows, msgs.PDBKeyAge, "Age")),
		paddedColumn(msgs.PDBKeyCurrentHealthy, "Current Healthy", widestValue(rows, msgs.PDBKeyCurrentHealthy, "Current Healthy")),
		paddedColumn(msgs.PDBKeyDesiredHealthy, "Desired Healthy", widestValue(rows, msgs.PDBKeyDesiredHealthy, "Desired Healthy")),
	}
}

func hpaNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.HPAKeyContext),
		paddedFlexColumn(msgs.HPAKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.HPAKeyName, "Name", 10),
		paddedFlexColumn(msgs.HPAKeyReference, "Reference", 8),
		paddedFlexColumn(msgs.HPAKeyMinMax, "Min-Max", 3),
		paddedFlexColumn(msgs.HPAKeyReplicas, "Replicas", 3),
		paddedFlexColumn(msgs.HPAKeyTargets, "Targets", 6),
	}
	return appendAge(cols, compact, paddedFlexColumn(msgs.HPAKeyAge, "Age", 3))
}

func hpaWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.HPAKeyContext),
		paddedColumn(msgs.HPAKeyNamespace, "Namespace", widestValue(rows, msgs.HPAKeyNamespace, "Namespace")),
		paddedColumn(msgs.HPAKeyName, "Name", widestValue(rows, msgs.HPAKeyName, "Name")),
		paddedColumn(msgs.HPAKeyReference, "Reference", widestValue(rows, msgs.HPAKeyReference, "Reference")),
		paddedColumn(msgs.HPAKeyMinMax, "Min-Max", widestValue(rows, msgs.HPAKeyMinMax, "Min-Max")),
		paddedColumn(msgs.HPAKeyReplicas, "Replicas", widestValue(rows, msgs.HPAKeyReplicas, "Replicas")),
		paddedColumn(msgs.HPAKeyTargets, "Targets", widestValue(rows, msgs.HPAKeyTargets, "Targets")),
		paddedColumn(msgs.HPAKeyAge, "Age", widestValue(rows, msgs.HPAKeyAge, "Age")),
	}
}

// nodeNarrowColumns/nodeWideColumns omit Namespace: Nodes are cluster-scoped
// (see msgs.NodeKeyName's doc comment), so Context is followed directly by
// Name rather than a Namespace column that would always read empty.
func nodeNarrowColumns(compact bool) []btable.Column {
	cols := []btable.Column{
		contextColumn(msgs.NodeKeyContext),
		paddedFlexColumn(msgs.NodeKeyName, "Name", 12),
		paddedFlexColumn(msgs.NodeKeyStatus, "Status", 4),
		paddedFlexColumn(msgs.NodeKeyRoles, "Roles", 5),
		paddedFlexColumn(msgs.NodeKeyCPU, "CPU", 3),
		paddedFlexColumn(msgs.NodeKeyMemory, "Memory", 3),
	}
	cols = appendAge(cols, compact, paddedFlexColumn(msgs.NodeKeyAge, "Age", 3))
	return append(cols, paddedFlexColumn(msgs.NodeKeyVersion, "Version", 4))
}

func nodeWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		contextColumn(msgs.NodeKeyContext),
		paddedColumn(msgs.NodeKeyName, "Name", widestValue(rows, msgs.NodeKeyName, "Name")),
		paddedColumn(msgs.NodeKeyStatus, "Status", widestValue(rows, msgs.NodeKeyStatus, "Status")),
		paddedColumn(msgs.NodeKeyRoles, "Roles", widestValue(rows, msgs.NodeKeyRoles, "Roles")),
		paddedColumn(msgs.NodeKeyAge, "Age", widestValue(rows, msgs.NodeKeyAge, "Age")),
		paddedColumn(msgs.NodeKeyVersion, "Version", widestValue(rows, msgs.NodeKeyVersion, "Version")),
		paddedColumn(msgs.NodeKeyInternalIP, "InternalIP", widestValue(rows, msgs.NodeKeyInternalIP, "InternalIP")),
		paddedColumn(msgs.NodeKeyOS, "OS/Arch", widestValue(rows, msgs.NodeKeyOS, "OS/Arch")),
		paddedColumn(msgs.NodeKeyCPU, "CPU", widestValue(rows, msgs.NodeKeyCPU, "CPU")),
		paddedColumn(msgs.NodeKeyMemory, "Memory", widestValue(rows, msgs.NodeKeyMemory, "Memory")),
	}
}
