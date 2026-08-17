// Package pages: render.go holds MainPage's status bar, overlay, and
// left-box render helpers — split out of mainPage.go (which keeps the top-
// level renderView dispatcher and per-tab table rendering) purely to keep
// the latter under a readable length. No behavior differs by being here;
// it's the same package.
package pages

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ktails/ktails/internal/state"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/ktails/ktails/internal/tui/views"
)

func (m *MainPage) renderStatusBar(snapshot state.Snapshot) string {
	p := styles.CatppuccinMocha()
	// Every segment carries the bar's background itself — a nested style's
	// ANSI reset would otherwise punch a hole in an outer background.
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Background(p.Mantle).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Background(p.Mantle).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Background(p.Mantle).Padding(0, 1)

	selectedCtx := len(snapshot.SelectedContexts)
	errCount := len(snapshot.Errors)
	loadingCount := 0
	for _, l := range snapshot.LoadingStates {
		if l {
			loadingCount++
		}
	}
	activeTabName := m.activeKind().Title()
	activeCount := m.activeTable().RowCount()

	focusStr := "Left Pane"
	if m.focus == focusTabs {
		focusStr = "Tabs"
	}

	left := leftStyle.Render(fmt.Sprintf("Contexts: %d", selectedCtx))
	mid := midStyle.Render(fmt.Sprintf("Tab: %s | Focus: %s", activeTabName, focusStr))

	// Everything below is state — what's currently selected/loading/focused
	// — and belongs on the left (§3.5). The right zone is reserved entirely
	// for keybind hints so the two zones never both report the same kind of
	// thing.
	var statusBits []string
	if loadingCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⏳ %d loading", loadingCount))
	} else if activeCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("%s: %d", activeTabName, activeCount))
	}
	if m.activeKind() == msgs.KindPods {
		if checkedCount := len(m.tables[msgs.KindPods].CheckedKeys()); checkedCount > 0 {
			statusBits = append(statusBits, fmt.Sprintf("☑ %d checked", checkedCount))
		}
	}
	if offset, total, ok := m.activeTable().ScrollStatus(); ok {
		statusBits = append(statusBits, fmt.Sprintf("◂ col %d/%d ▸", offset, total))
	}
	if query, matches, typing, ok := m.activeTable().FilterStatus(); ok {
		cursor := ""
		if typing {
			cursor = "_"
		}
		statusBits = append(statusBits, fmt.Sprintf("/%s%s (%d match(es))", query, cursor, matches))
	}
	if m.focus == focusLeftPane && m.activeLeftSection == sectionNamespaces {
		if query, matches, typing, ok := m.namespacesPane.FilterStatus(); ok {
			cursor := ""
			if typing {
				cursor = "_"
			}
			statusBits = append(statusBits, fmt.Sprintf("/%s%s (%d match(es))", query, cursor, matches))
		}
	}
	if m.showDetail {
		if percent, ok := m.resourceDetail.HScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if m.showLogs {
		if percent, ok := m.podLogs.ScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if len(statusBits) == 0 {
		statusBits = append(statusBits, "Ready")
	}
	status := rightStyle.Render(strings.Join(statusBits, "  |  "))

	// Hints are the right zone's only content — a context-sensitive
	// keybind list (§4), never state, so the two footer zones never
	// duplicate the same kind of information.
	hints := lipgloss.NewStyle().Foreground(p.Overlay1).Background(p.Mantle).Faint(true).Render(m.footerHints(errCount) + " ")

	gap := styles.StatusBar.Render("  ")
	leftMid := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, mid, gap, status)

	// The error indicator gets its own Red segment (distinct from the
	// generic Green status text) and its own "(e)" hint, since it's the
	// entry point into the non-modal error summary (see the "e" keybind) —
	// a one-line "N errors" isn't itself informative enough to act on.
	rightSection := hints
	if errCount > 0 {
		errorSegment := lipgloss.NewStyle().Foreground(p.Red).Background(p.Mantle).Bold(true).Padding(0, 1).
			Render(fmt.Sprintf("⚠ %d context error(s) (e)", errCount))
		rightSection = lipgloss.JoinHorizontal(lipgloss.Top, errorSegment, gap, hints)
	}
	spacerWidth := m.width - lipgloss.Width(leftMid) - lipgloss.Width(rightSection)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	spacer := styles.StatusBar.Render(strings.Repeat(" ", spacerWidth))

	return leftMid + spacer + rightSection
}

// footerHints returns the right footer zone's keybind hint list (§4),
// picking the 4-6 keys relevant to whatever currently has focus rather than
// always showing the same fixed set — e.g. "d delete" makes no sense while
// the context picker has focus. errCount appends "e errors" when nonzero,
// so the "e" toggle for the non-modal error summary is only advertised when
// there's actually something to show.
func (m *MainPage) footerHints(errCount int) string {
	var hints string
	switch {
	case m.showLogs:
		hints = "↑↓ scroll  w wrap  c isolate  esc close  ctrl+c quit"
	case m.showDetail:
		hints = "↑↓ scroll  y yaml  ctrl+r focus  esc close  ctrl+c quit"
	case m.focus == focusLeftPane:
		hints = "space select  ↵ confirm  tab focus  ?  help  ctrl+c quit"
	case m.activeKind() == msgs.KindPods:
		hints = "/ filter  [ ] tabs  space check  l logs  d describe  ?  help"
	default:
		hints = "/ filter  [ ] tabs  d describe  r refresh  ?  help  ctrl+c quit"
	}
	if errCount > 0 {
		hints += "  e errors"
	}
	return hints
}

// helpKeyColWidth is the fixed width of the key-binding column in the help
// overlay. Every key label in helpBindings must fit within it — a label
// that doesn't is shortened, with whatever context it lost (which pane,
// which mode) moved into the description instead, rather than letting it
// overflow into the description column.
const helpKeyColWidth = 16

// helpBinding is one row of the help overlay: a key label and its
// description.
type helpBinding struct{ key, desc string }

// helpBindings is the full keybinding reference shown by renderHelpOverlay.
var helpBindings = []helpBinding{
	{"Tab / Shift+Tab", "Switch pane focus"},
	{"[ / ]", "Navigate tabs, or cycle sidebar sections when it has focus"},
	{"Space/Enter", "Namespaces: check to watch; Enter applies"},
	{"a (Namespaces)", "Toggle all namespaces checked, or restore prior selection"},
	{"/ (Namespaces)", "Filter namespace rows; Enter to keep, Esc to clear"},
	{"Space/Enter", "Clusters: bulk (de)select a cluster's contexts; Enter applies"},
	{"← / →", "Navigate tabs (alias)"},
	{"↑↓ j/k", "Move up / down"},
	{"PgUp / PgDn", "Move up/down a page (also Ctrl+U/Ctrl+D)"},
	{"g/Home G/End", "Jump to first / last row"},
	{"/", "Filter the active table by name; Enter to keep, Esc to clear"},
	{"Space", "Toggle context selection / check a Pods row for logs"},
	{"Enter (Contexts)", "Confirm selection & load"},
	{"d", "Open/focus detail pane for the row under cursor"},
	{"l (Pods tab)", "Open/reconcile log pane for checked rows (or cursor row)"},
	{"Ctrl+X (Pods)", "Clear all checked rows"},
	{"r", "Refresh the active tab across all selected contexts"},
	{"c (log focused)", "Isolate one source, or return to the full merge"},
	{"Ctrl+R", "Jump back into an open detail pane"},
	{"R", "Toggle auto-refresh on/off"},
	{"Shift+N/M/A", "Cycle sort by Name / Namespace / Age: asc -> desc -> off"},
	{"↑↓ jk PgUp/Dn", "Scroll detail/log pane (while focused)"},
	{"y (detail focus)", "Jump to the YAML section"},
	{"Home / End", "Jump to top / bottom of detail/log pane"},
	{"Esc", "Unfocus, then close pane / overlay / dismiss error"},
	{"e", "Toggle the context-error summary (when any context has an error)"},
	{"?", "Toggle this help"},
	{"Ctrl+C", "Quit"},
}

// renderHelpBinding renders one binding row, wrapping its description to
// descWidth (ansi.Wrap, so it never silently overflows into whatever's
// beside it) and blank-indenting the key column on any wrapped
// continuation lines so they stay aligned under the description rather
// than repeating or truncating the key.
func renderHelpBinding(b helpBinding, keyStyle, descStyle lipgloss.Style, descWidth int) string {
	descLines := strings.Split(ansi.Wrap(b.desc, descWidth, ""), "\n")

	indent := strings.Repeat(" ", helpKeyColWidth+1)
	out := make([]string, len(descLines))
	for i, dl := range descLines {
		if i == 0 {
			out[i] = keyStyle.Render(b.key) + " " + descStyle.Render(dl)
		} else {
			out[i] = indent + descStyle.Render(dl)
		}
	}
	return strings.Join(out, "\n")
}

// helpBoxOverheadW/helpBoxOverheadH are the help box's own border+padding
// (RoundedBorder — 1 cell each side; Padding(1, 3) — 1 row/3 cols each
// side), subtracted from the terminal size to get the content budget
// actually available for text.
const (
	helpBoxOverheadW = 8 // 3+3 padding, 1+1 border
	helpBoxOverheadH = 4 // 1+1 padding, 1+1 border
)

// renderHelpOverlay renders the full keybinding reference as a centered,
// bordered box, sized to fit the app's own minimum terminal (80x24) rather
// than the ~125x33 the original static layout needed. Descriptions wrap to
// the available width instead of overflowing, and the whole list falls back
// to two side-by-side columns (half the rows each) if a single column would
// otherwise exceed the terminal's height — but only when there's genuinely
// room for two columns; below that width the overflow is accepted rather
// than crushing either column past readability, matching the "keep it
// simple and static, no scrolling widget" approach used here.
func (m *MainPage) renderHelpOverlay() string {
	p := styles.CatppuccinMocha()

	titleStyle := lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(p.Blue).Bold(true).Width(helpKeyColWidth)
	descStyle := lipgloss.NewStyle().Foreground(p.Text)
	sepStyle := lipgloss.NewStyle().Foreground(p.Overlay0)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Mauve).
		Background(p.Mantle).
		Padding(1, 3)

	boxBudget := m.width - 4 - helpBoxOverheadW
	if boxBudget < helpKeyColWidth+10 {
		boxBudget = helpKeyColWidth + 10
	}

	renderColumn := func(rows []helpBinding, descWidth int) string {
		lines := make([]string, len(rows))
		for i, b := range rows {
			lines[i] = renderHelpBinding(b, keyStyle, descStyle, descWidth)
		}
		return strings.Join(lines, "\n")
	}

	singleDescWidth := boxBudget - helpKeyColWidth - 1
	if singleDescWidth > 70 {
		singleDescWidth = 70 // wide terminals still wrap at a readable prose width
	}
	content := renderColumn(helpBindings, singleDescWidth)
	sepWidth := lipgloss.Width(content)

	availH := (m.height - views.FooterHeight) - helpBoxOverheadH - 2 // -2: title line + separator line
	if lipgloss.Height(content) > availH && m.width >= 100 {
		colBudget := (boxBudget - 3) / 2 // 3-col gap between the two columns
		colDescWidth := colBudget - helpKeyColWidth - 1
		if colDescWidth < 16 {
			colDescWidth = 16
		}
		mid := (len(helpBindings) + 1) / 2
		left := renderColumn(helpBindings[:mid], colDescWidth)
		right := renderColumn(helpBindings[mid:], colDescWidth)
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
		sepWidth = lipgloss.Width(content)
	}

	sep := sepStyle.Render(strings.Repeat("─", sepWidth))
	box := boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Keybindings"), sep, content))
	return lipgloss.Place(m.width, m.height-views.FooterHeight, lipgloss.Center, lipgloss.Center, box)
}

// renderTooSmallOverlay replaces the whole TUI with a plain message when the
// terminal is below views.MinContentWidth x views.MinHeight — below that, the
// real layout doesn't have room to render without breaking, so we don't try.
func (m *MainPage) renderTooSmallOverlay() string {
	p := styles.CatppuccinMocha()
	msg := fmt.Sprintf(
		"Terminal window is too small\n\nCurrent size: %d x %d\nMinimum size: %d x %d\n\nPlease resize your terminal",
		m.width, m.height, views.MinContentWidth, views.MinHeight,
	)
	body := lipgloss.NewStyle().Foreground(p.Text).Align(lipgloss.Center).Render(msg)

	// Below a certain point there isn't even room for the box border/padding
	// — fall back to bare text rather than let lipgloss mangle it further.
	if m.width < 20 || m.height < 6 {
		return body
	}

	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Yellow).
		Padding(1, 3).
		Align(lipgloss.Center).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MainPage) renderErrorOverlay(msg string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Error")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", overlaySepWidth(maxW)))
	body := lipgloss.NewStyle().Foreground(p.Text).Render(msg)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-views.FooterHeight, lipgloss.Center, lipgloss.Center, box.Render(content))
}

// overlaySepWidth returns the usable content width inside an overlay box
// styled Width(maxW).Padding(1, 3).Border(lipgloss.RoundedBorder()) — i.e.
// maxW minus the border's 2 columns and the padding's 3+3 columns, so a
// separator rule spans exactly the content area instead of wrapping onto
// the next line (maxW-2, the previous computation, ignored padding
// entirely).
func overlaySepWidth(maxW int) int {
	const overlayHorizontalChrome = 8 // RoundedBorder 1+1, Padding(1,3) 3+3
	w := maxW - overlayHorizontalChrome
	if w < 1 {
		w = 1
	}
	return w
}

func (m *MainPage) renderErrorSummaryOverlay(errors map[string]string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Errors encountered")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", overlaySepWidth(maxW)))
	// Map iteration order is randomized per Go's spec — without sorting,
	// this list would reshuffle every single frame.
	ctxNames := make([]string, 0, len(errors))
	for ctx := range errors {
		ctxNames = append(ctxNames, ctx)
	}
	sort.Strings(ctxNames)
	var bodyLines []string
	for _, ctx := range ctxNames {
		bodyLines = append(bodyLines, fmt.Sprintf("• %s: %s", ctx, errors[ctx]))
	}
	body := lipgloss.NewStyle().Foreground(p.Text).Render(strings.Join(bodyLines, "\n"))
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc/e to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-views.FooterHeight, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *MainPage) renderLoadingIndicator(loading map[string]bool) string {
	p := styles.CatppuccinMocha()
	loadingStyle := lipgloss.NewStyle().
		Foreground(p.Blue).
		Background(p.Surface0).
		Padding(0, 1)

	var loadingContexts []string
	for ctx, isLoading := range loading {
		if isLoading {
			loadingContexts = append(loadingContexts, ctx)
		}
	}
	// Map iteration order is randomized per Go's spec — without sorting,
	// this list would reshuffle every single frame.
	sort.Strings(loadingContexts)

	if len(loadingContexts) == 0 {
		return ""
	}

	return loadingStyle.Render(fmt.Sprintf("⏳ Loading: %s...", strings.Join(loadingContexts, ", ")))
}

// leftSections is how many always-visible sections the Context List box is
// divided into, and how many one-line headers that costs.
const leftSections = 3

// minLeftSectionHeight is the minimum content height any one section is
// left with, even on a barely-tall-enough terminal.
const minLeftSectionHeight = 1

// splitLeftSections divides the Context List box's content height across
// its three always-visible, stacked sections — Contexts (the interactive
// list, given the most room), Namespaces, and Clusters — each preceded by
// a one-line header. total is the box's full content height (views.Rects'
// LeftContentH); the three results plus leftSections header lines always
// sum back to exactly total.
func splitLeftSections(total int) (contextsH, namespacesH, clustersH int) {
	remaining := total - leftSections
	if remaining < leftSections*minLeftSectionHeight {
		remaining = leftSections * minLeftSectionHeight
	}

	namespacesH = remaining / 4
	if namespacesH < minLeftSectionHeight {
		namespacesH = minLeftSectionHeight
	}
	clustersH = remaining / 4
	if clustersH < minLeftSectionHeight {
		clustersH = minLeftSectionHeight
	}
	contextsH = remaining - namespacesH - clustersH
	if contextsH < minLeftSectionHeight {
		contextsH = minLeftSectionHeight
	}
	return contextsH, namespacesH, clustersH
}

// renderLeftBox renders the Context List box's content: the interactive
// Contexts list, then the interactive Namespaces and Clusters lists (per-
// context multi-select namespaces, and grouping Contexts' rows by
// kubeconfig cluster for bulk select/deselect), each clipped
// (views.FitBlock) to its allotted height so one section can never push the
// others out of place. Only Namespaces and Clusters get their own header
// rendered here — Contexts' header is the outer box's border title instead.
func (m *MainPage) renderLeftBox(r views.Rects, focused bool) string {
	contextsH, namespacesH, clustersH := splitLeftSections(r.LeftContentH)

	p := styles.CatppuccinMocha()
	dimHeaderStyle := lipgloss.NewStyle().Foreground(p.Overlay1).Bold(true)
	// Each section's header only picks up the focus colour when it's
	// genuinely the section with keyboard focus (see activeLeftSection) —
	// otherwise every section would look focused whenever the sidebar has
	// focus at all.
	namespacesHeaderStyle := dimHeaderStyle
	if focused && m.activeLeftSection == sectionNamespaces {
		namespacesHeaderStyle = namespacesHeaderStyle.Foreground(styles.FocusColor)
	}
	clustersHeaderStyle := dimHeaderStyle
	if focused && m.activeLeftSection == sectionClusters {
		clustersHeaderStyle = clustersHeaderStyle.Foreground(styles.FocusColor)
	}

	m.clusterList.Refresh()

	blocks := []string{
		views.FitBlock(m.contextList.View(), r.LeftContentW, contextsH),
		namespacesHeaderStyle.Render("Namespaces"),
		views.FitBlock(m.namespacesPane.View(), r.LeftContentW, namespacesH),
		clustersHeaderStyle.Render("Clusters"),
		views.FitBlock(m.clusterList.View(), r.LeftContentW, clustersH),
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}
