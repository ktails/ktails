package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// ASCIIBorder is a lipgloss.Border built entirely from plain ASCII
// (-, |, +). Unicode box-drawing characters (U+2500-U+257F) and several
// decorative glyphs used elsewhere in this package carry the "Ambiguous"
// East Asian Width property, so terminals that treat ambiguous runes as
// double-width (e.g. Ghostty's default grapheme-width-method = unicode)
// disagree with lipgloss's own (narrow) width accounting on how many cells
// a line occupies — that mismatch compounds across a frame and can push
// rows off-screen. The app defaults to the fancier Unicode borders/glyphs
// for a nicer look; if that turns out to overflow on a given terminal,
// swap the relevant Border(...) call over to ASCIIBorder() (and the
// affected glyphs to ASCII) as a compatibility fallback — kept here
// specifically for that.
func ASCIIBorder() lipgloss.Border {
	return lipgloss.Border{
		Top:         "-",
		Bottom:      "-",
		Left:        "|",
		Right:       "|",
		TopLeft:     "+",
		TopRight:    "+",
		BottomLeft:  "+",
		BottomRight: "+",
	}
}

// FocusColor/BlurColor are the single accent pair used across the TUI:
// focused panes get the vibrant accent, blurred panes the subtle overlay.
// FocusColor is Blue exclusively — Blue never appears in IdentityColors or
// the status set (see below), so "focused" can never be confused with
// "which cluster" or "what's broken".
var (
	FocusColor = CatppuccinMocha().Blue
	BlurColor  = CatppuccinMocha().Overlay0
)

// IdentityColors is the context/cluster identity rotation: the first
// selected context gets IdentityColors[0], the second IdentityColors[1],
// and so on. Deliberately excludes Red/Yellow/Green (status, see below) and
// Blue (focus accent) so the three colour systems never collide.
var IdentityColors = []color.Color{
	CatppuccinMocha().Mauve,     // 1
	CatppuccinMocha().Teal,      // 2
	CatppuccinMocha().Peach,     // 3
	CatppuccinMocha().Pink,      // 4
	CatppuccinMocha().Sapphire,  // 5
	CatppuccinMocha().Flamingo,  // 6
	CatppuccinMocha().Rosewater, // 7
	CatppuccinMocha().Maroon,    // 8 and fallback for every context beyond it
}

// IdentityColor returns the identity colour for the n-th selected context
// (0-indexed), repeating the last (Maroon) colour for every context beyond
// the rotation's length rather than reusing an earlier context's colour.
func IdentityColor(n int) color.Color {
	if n < 0 {
		n = 0
	}
	if n >= len(IdentityColors) {
		n = len(IdentityColors) - 1
	}
	return IdentityColors[n]
}

// Status colors (semantic, fixed — never reassigned to identity or focus).
var (
	StatusHealthy = CatppuccinMocha().Green
	StatusError   = CatppuccinMocha().Red
)

// StatusBar is the one-line bar at the bottom of the TUI. The background
// fill is what makes it read as a bar rather than floating text — segments
// rendered onto it must carry the same background.
var StatusBar = lipgloss.NewStyle().Background(CatppuccinMocha().Mantle)
