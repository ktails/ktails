package styles

import "charm.land/lipgloss/v2"

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
var (
	FocusColor = CatppuccinMocha().Mauve
	BlurColor  = CatppuccinMocha().Overlay0
)

// StatusBar is the one-line bar at the bottom of the TUI. The background
// fill is what makes it read as a bar rather than floating text — segments
// rendered onto it must carry the same background.
var StatusBar = lipgloss.NewStyle().Background(CatppuccinMocha().Mantle)
