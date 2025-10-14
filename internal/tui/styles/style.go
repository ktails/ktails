package tbl

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// DefaultTableStyles returns a basic style set with a highlighted selected row
func DefaultTableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57")).
		Bold(true)
	return styles
}

// Palette defines a set of Catppuccin colors for styling.
type Palette struct {
	Base      lipgloss.Color
	Mantle    lipgloss.Color
	Crust     lipgloss.Color
	Text      lipgloss.Color
	Subtext1  lipgloss.Color
	Subtext0  lipgloss.Color
	Overlay2  lipgloss.Color
	Overlay1  lipgloss.Color
	Overlay0  lipgloss.Color
	Surface2  lipgloss.Color
	Surface1  lipgloss.Color
	Surface0  lipgloss.Color
	Blue      lipgloss.Color
	Lavender  lipgloss.Color
	Sapphire  lipgloss.Color
	Sky       lipgloss.Color
	Teal      lipgloss.Color
	Green     lipgloss.Color
	Yellow    lipgloss.Color
	Peach     lipgloss.Color
	Maroon    lipgloss.Color
	Red       lipgloss.Color
	Mauve     lipgloss.Color
	Pink      lipgloss.Color
	Flamingo  lipgloss.Color
	Rosewater lipgloss.Color
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

// CatppuccinTableStyles returns Bubbles table styles using the Catppuccin Mocha palette.
func CatppuccinTableStyles() table.Styles {
	p := CatppuccinMocha()
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Background(p.Surface0).
		Foreground(p.Text).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(p.Overlay0).
		BorderBottom(true).
		Bold(true)
	styles.Cell = styles.Cell.
		Foreground(p.Subtext1)
	styles.Selected = styles.Selected.
		Foreground(p.Base).
		Background(p.Blue).
		Bold(true)
	return styles
}

// HelpBoxStyle returns a styled lipgloss style for help overlays using the palette.
func HelpBoxStyle() lipgloss.Style {
	p := CatppuccinMocha()
	return lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Mantle).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Mauve).
		Padding(1, 2)
}
