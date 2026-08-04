package models

import (
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/ktails/ktails/internal/tui/styles"
)

// paneRowFallbackWidth is the width a left-pane row falls back to before its
// list.Model has ever received a real WindowSizeMsg-derived width.
const paneRowFallbackWidth = 30

// paneRowWidth returns m.Width(), or paneRowFallbackWidth if the list hasn't
// been sized yet — shared by contextDelegate/clusterDelegate/
// namespaceDelegate's Render methods.
func paneRowWidth(m list.Model) int {
	if w := m.Width(); w > 0 {
		return w
	}
	return paneRowFallbackWidth
}

// paneRowContent is what one left-pane row needs rendered, split into the
// two variants renderPaneRow needs:
//
//   - plainTitle/plainDesc carry no embedded ANSI — used only when the row
//     is both the cursor and the pane has keyboard focus, where a single
//     flat Background+Foreground covers the whole line and embedded color
//     codes would fight it.
//   - styledTitle/styledDesc carry each piece's own inline color (dot,
//     name, etc.) — used everywhere else, with only the row's own
//     background (or none) wrapped around them.
//
// contextDelegate's description never actually differs between the two
// (it sets both fields to the same pre-styled string); clusterDelegate's
// does — see its Render for why. styledDesc == "" means this pane's rows
// have no description line at all (namespaceDelegate).
type paneRowContent struct {
	plainTitle  string
	styledTitle string
	plainDesc   string
	styledDesc  string
	// boldOnCursor bolds the *entire* title line while the cursor sits on
	// it (whether or not the pane has focus), regardless of any boldness
	// styledTitle already carries internally. contextDelegate wants this
	// unconditionally; clusterDelegate instead leaves title boldness
	// entirely up to styledTitle's own embedded Bold(g.AllSelected), so it
	// passes false here.
	boldOnCursor bool
}

// renderPaneRow renders one left-pane list row in the shared three-way
// cursor/focus shape used by contextDelegate, clusterDelegate, and
// namespaceDelegate: a bright FocusColor highlight when the row is both the
// cursor and the pane has keyboard focus, a muted Surface0 highlight when
// it's just the cursor, or no background at all otherwise. Only what goes
// into the title/description content differs per pane — see
// paneRowContent's doc comment for the two fine-grained behavioral
// differences (bold-on-cursor, plain-vs-styled description) this
// preserves rather than papering over.
func renderPaneRow(width int, c paneRowContent, isCursor, focused bool) string {
	p := styles.CatppuccinMocha()

	var titleLine, desc string
	switch {
	case isCursor && focused:
		titleLine = lipgloss.NewStyle().Background(styles.FocusColor).Foreground(p.Base).Bold(true).Width(width).Render(c.plainTitle)
		desc = c.plainDesc
	case isCursor:
		titleLine = lipgloss.NewStyle().Background(p.Surface0).Bold(c.boldOnCursor).Width(width).Render(c.styledTitle)
		desc = c.styledDesc
	default:
		titleLine = lipgloss.NewStyle().Width(width).Render(c.styledTitle)
		desc = c.styledDesc
	}

	if c.styledDesc == "" {
		return titleLine
	}
	descColor := p.Overlay0
	if isCursor {
		descColor = p.Overlay1
	}
	descLine := lipgloss.NewStyle().Foreground(descColor).Width(width).Render(desc)
	return titleLine + "\n" + descLine
}

// newPaneList builds a list.Model with the option set common to the three
// left-pane widgets (Contexts, Clusters, Namespaces): no built-in status
// bar/help/pagination/filtering/quit-keybindings (each pane draws its own
// section header and, where needed, its own "/" filter — see rowFilter —
// since bubbles/list's built-ins can't be escaped cleanly through
// MainPage's global Esc handling), and no title bar (cleared both via
// SetShowTitle(false) and Title = "" — SetShowTitle(false) alone still
// leaves an empty colored title-bar box; see each pane's own constructor
// comment prior to this extraction for that history).
func newPaneList(delegate list.ItemDelegate) list.Model {
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.Title = ""
	l.SetShowTitle(false)
	return l
}
