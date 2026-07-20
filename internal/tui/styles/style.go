package styles

import (
	"image/color"

	"charm.land/bubbles/v2/list"
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

// CatppuccinMocha returns the Mocha palette.
func CatppuccinMocha() Palette {
	return Palette{
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
}

// CatppuccinMocha returns the Mocha palette.
func CatppuccinLatte() Palette {
	return Palette{
		Base:      lipgloss.Color("#eff1f5"),
		Mantle:    lipgloss.Color("#e6e9ef"),
		Crust:     lipgloss.Color("#dce0e8"),
		Text:      lipgloss.Color("#4c4f69"),
		Subtext1:  lipgloss.Color("#5c5f77"),
		Subtext0:  lipgloss.Color("#6c6f85"),
		Overlay2:  lipgloss.Color("#7c7f93"),
		Overlay1:  lipgloss.Color("#8c8fa1"),
		Overlay0:  lipgloss.Color("#9ca0b0"),
		Surface2:  lipgloss.Color("#acb0be"),
		Surface1:  lipgloss.Color("#bcc0cc"),
		Surface0:  lipgloss.Color("#ccd0da"),
		Blue:      lipgloss.Color("#1e66f5"),
		Lavender:  lipgloss.Color("#7287fd"),
		Sapphire:  lipgloss.Color("#209fb5"),
		Sky:       lipgloss.Color("#04a5e5"),
		Teal:      lipgloss.Color("#179299"),
		Green:     lipgloss.Color("#40a02b"),
		Yellow:    lipgloss.Color("#df8e1d"),
		Peach:     lipgloss.Color("#fe640b"),
		Maroon:    lipgloss.Color("#e64553"),
		Red:       lipgloss.Color("#d20f39"),
		Mauve:     lipgloss.Color("#8839ef"),
		Pink:      lipgloss.Color("#ea76cb"),
		Flamingo:  lipgloss.Color("#dd7878"),
		Rosewater: lipgloss.Color("#dc8a78"),
	}
}

// BubbleTableStyle bundles the header/highlight/base styles applied to the
// Pods/Deployments/svc tables (evertras/bubble-table) — the bubble-table
// equivalent of the old CatppuccinTableStyles for bubbles/table.
type BubbleTableStyle struct {
	Header    lipgloss.Style
	Highlight lipgloss.Style
	Base      lipgloss.Style
}

// CatppuccinBubbleTableStyle returns bubble-table styles using the
// Catppuccin Mocha palette.
func CatppuccinBubbleTableStyle() BubbleTableStyle {
	p := CatppuccinMocha()
	return BubbleTableStyle{
		Header: lipgloss.NewStyle().
			Background(p.Surface0).
			Foreground(p.Text).
			Bold(true),
		Highlight: lipgloss.NewStyle().
			Foreground(p.Base).
			Background(p.Blue).
			Bold(true),
		Base: lipgloss.NewStyle().
			Foreground(p.Subtext1),
	}
}

// HelpBoxStyle returns a styled lipgloss style for help overlays using the palette.
func HelpBoxStyle() lipgloss.Style {
	p := CatppuccinMocha()
	return lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Mantle).
		Border(ASCIIBorder()).
		BorderForeground(p.Mauve).
		Padding(1, 2)
}

func CatppuccinMochaListStyles() list.Styles {
	p := CatppuccinMocha()
	return list.Styles{
		Title: lipgloss.NewStyle().
			Foreground(p.Flamingo).
			Background(p.Mantle).
			Bold(true),
		NoItems: lipgloss.NewStyle().
			Background(p.Mantle).
			Bold(false),

		HelpStyle: lipgloss.NewStyle().
			Foreground(p.Text).
			Background(p.Mantle).
			Border(ASCIIBorder()).
			BorderForeground(p.Mauve).
			Padding(1, 2),
	}
}

// ListPaneStyle provides a bordered container for the contexts list pane.
func ListPaneStyle() lipgloss.Style {
	p := CatppuccinMocha()
	l := lipgloss.NewStyle().
		Border(ASCIIBorder()).BorderStyle(ASCIIBorder()).BorderForeground(p.Red)
	return l
}
