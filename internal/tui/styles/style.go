package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette defines a set of Catppuccin colors for styling.
type Palette struct {
	Base      color.Color
	Mantle    color.Color
	Crust     color.Color
	Text      color.Color
	Subtext1  color.Color
	Subtext0  color.Color
	Overlay2  color.Color
	Overlay1  color.Color
	Overlay0  color.Color
	Surface2  color.Color
	Surface1  color.Color
	Surface0  color.Color
	Blue      color.Color
	Lavender  color.Color
	Sapphire  color.Color
	Sky       color.Color
	Teal      color.Color
	Green     color.Color
	Yellow    color.Color
	Peach     color.Color
	Maroon    color.Color
	Red       color.Color
	Mauve     color.Color
	Pink      color.Color
	Flamingo  color.Color
	Rosewater color.Color
}

// mocha is built once; Palette is a small value type (26 color interface
// values) so returning a copy is cheap and callers can't mutate the shared
// instance.
var mocha = Palette{
	Base:      lipgloss.Color("#1e1e2e"),
	Mantle:    lipgloss.Color("#181825"),
	Crust:     lipgloss.Color("#11111b"),
	Text:      lipgloss.Color("#cdd6f4"),
	Subtext1:  lipgloss.Color("#bac2de"),
	Subtext0:  lipgloss.Color("#a6adc8"),
	Overlay2:  lipgloss.Color("#9399b2"),
	Overlay1:  lipgloss.Color("#7f849c"),
	Overlay0:  lipgloss.Color("#6c7086"),
	Surface2:  lipgloss.Color("#585b70"),
	Surface1:  lipgloss.Color("#45475a"),
	Surface0:  lipgloss.Color("#313244"),
	Blue:      lipgloss.Color("#89b4fa"),
	Lavender:  lipgloss.Color("#b4befe"),
	Sapphire:  lipgloss.Color("#74c7ec"),
	Sky:       lipgloss.Color("#89dceb"),
	Teal:      lipgloss.Color("#94e2d5"),
	Green:     lipgloss.Color("#a6e3a1"),
	Yellow:    lipgloss.Color("#f9e2af"),
	Peach:     lipgloss.Color("#fab387"),
	Maroon:    lipgloss.Color("#eba0ac"),
	Red:       lipgloss.Color("#f38ba8"),
	Mauve:     lipgloss.Color("#cba6f7"),
	Pink:      lipgloss.Color("#f5c2e7"),
	Flamingo:  lipgloss.Color("#f2cdcd"),
	Rosewater: lipgloss.Color("#f5e0dc"),
}

// CatppuccinMocha returns the Mocha palette.
func CatppuccinMocha() Palette { return mocha }

// BubbleTableStyle bundles the header/highlight/base styles applied to the
// Pods/Deployments/svc tables (evertras/bubble-table) — the bubble-table
// equivalent of the old CatppuccinTableStyles for bubbles/table.
type BubbleTableStyle struct {
	Header    lipgloss.Style
	Highlight lipgloss.Style
	Base      lipgloss.Style
}

// bubbleTableStyle is built once; lipgloss.Style has value semantics, so
// returning a copy from CatppuccinBubbleTableStyle is safe — callers can't
// mutate the shared instance through it.
var bubbleTableStyle = BubbleTableStyle{
	Header: lipgloss.NewStyle().
		Background(mocha.Surface0).
		Foreground(mocha.Text).
		Bold(true),
	Highlight: lipgloss.NewStyle().
		Foreground(mocha.Base).
		Background(FocusColor).
		Bold(true),
	Base: lipgloss.NewStyle().
		Foreground(mocha.Subtext1),
}

// CatppuccinBubbleTableStyle returns bubble-table styles using the
// Catppuccin Mocha palette.
func CatppuccinBubbleTableStyle() BubbleTableStyle { return bubbleTableStyle }

// helpBoxStyle is built once — see bubbleTableStyle.
var helpBoxStyle = lipgloss.NewStyle().
	Foreground(mocha.Text).
	Background(mocha.Mantle).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(mocha.Mauve).
	Padding(1, 2)

// HelpBoxStyle returns a styled lipgloss style for help overlays using the palette.
func HelpBoxStyle() lipgloss.Style { return helpBoxStyle }
