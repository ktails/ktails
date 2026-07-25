// Package views owns the master layout of ktails: the Context List box on
// the left, the Tab Area box on the right, and the status bar beneath them.
// Solve (layout.go) is the single source of the width/height budget.
package views

const (
	// MinHeight is the minimum height of the TUI.
	MinHeight = 24
	// MinContentWidth is the minimum width of the TUI.
	MinContentWidth = 80
	// FooterHeight is the height of the status bar at the bottom of the TUI.
	FooterHeight = 1
	// MaxLeftPaneWidth caps how wide the left (context list) pane is allowed
	// to grow on wide terminals — its content (short context names) never
	// needs a fixed fraction of a very wide window, so beyond this the extra
	// space goes to the tab area instead.
	MaxLeftPaneWidth = 32
	// MinLeftPaneWidth floors the left pane's width on narrow terminals, so
	// it stays readable instead of shrinking in lockstep with the whole
	// window.
	MinLeftPaneWidth = 18
)

// Tier is one of the three adaptive-layout width bands (§8.1): a pass/fail
// "resize prompt or one fixed layout" gate has no room between broken and
// fully rendered, so components that want intermediate behavior (a
// narrower sidebar, fewer table columns) key off this instead of the raw
// pixel width directly.
type Tier int

const (
	// TierCompact is 80-109 cols — the floor (MinContentWidth) up to just
	// under Standard.
	TierCompact Tier = iota
	// TierStandard is 110-159 cols — today's default rendering.
	TierStandard
	// TierWide is 160+ cols — room for extras beyond Standard (see §8.4's
	// optional Detail side-panel).
	TierWide
)

// tierStandardMin and tierWideMin are the tier boundaries (§8.1's table).
const (
	tierStandardMin = 110
	tierWideMin     = 160
)

// WidthTier returns which adaptive-layout tier a terminal width falls into.
// Below MinContentWidth is the hard floor (unchanged resize-prompt
// behavior, see MinContentWidth) — WidthTier still returns TierCompact for
// it since callers only consult this once past that floor.
func WidthTier(width int) Tier {
	switch {
	case width >= tierWideMin:
		return TierWide
	case width >= tierStandardMin:
		return TierStandard
	default:
		return TierCompact
	}
}
