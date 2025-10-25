package styles

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
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

func CatppuccinMochaListStyles() list.Styles {
	p := CatppuccinMocha()
	return list.Styles{
		Title: lipgloss.NewStyle().
			Foreground(p.Text).
			Background(p.Mantle).
			Bold(true),

		HelpStyle: lipgloss.NewStyle().
			Foreground(p.Text).
			Background(p.Mantle).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Mauve).
			Padding(1, 2),
	}
}

// CatppuccinMochaListStylesFocused for focussed-aware list styles.
func CatppuccinMochaListStylesFocused(focused bool) list.Styles {
	p := CatppuccinMocha()
	accent := p.Blue
	muted := p.Overlay0
	border := muted
	if focused {
		border = accent
	}
	return list.Styles{
		Title: lipgloss.NewStyle().
			Foreground(p.Text).
			Background(p.Surface0).
			Border(lipgloss.NormalBorder()).
			BorderForeground(border).
			BorderBottom(true).
			Padding(1).
			Bold(true),
		PaginationStyle: lipgloss.NewStyle().Foreground(p.Subtext0),
		HelpStyle:       lipgloss.NewStyle().Foreground(p.Subtext0),
		FilterPrompt:    lipgloss.NewStyle().Foreground(accent),
		FilterCursor:    lipgloss.NewStyle().Foreground(p.Text),
		StatusBar:       lipgloss.NewStyle().Background(p.Surface0).Foreground(p.Subtext0).Padding(0, 1),
		StatusEmpty:     lipgloss.NewStyle().Foreground(p.Overlay1),
		NoItems:         lipgloss.NewStyle().Foreground(p.Overlay1),
	}
}

// ListPaneStyle provides a bordered container for the contexts list pane.
func ListPaneStyle() lipgloss.Style {
	p := CatppuccinMocha()
	l := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderStyle(lipgloss.DoubleBorder()).BorderForeground(p.Red)
	return l
}

// PaneStyle returns a focus-aware pane style using the palette.
func PaneStyle(focused bool) lipgloss.Style {
	p := CatppuccinMocha()
	border := p.Overlay0
	if focused {
		border = p.Blue
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(p.Mantle).
		Padding(1, 1, 1, 1)
}

// TableStylesFocused returns focus-aware table styles.
func TableStylesFocused(focused bool) table.Styles {
	p := CatppuccinMocha()
	styles := table.DefaultStyles()
	border := p.Overlay0
	accent := p.Blue
	if focused {
		border = accent
	}
	styles.Header = styles.Header.
		Background(p.Surface0).
		Foreground(p.Text).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(border).
		BorderBottom(true).
		Bold(true)
	styles.Cell = styles.Cell.
		Foreground(p.Subtext1)
	styles.Selected = styles.Selected.
		Foreground(p.Base).
		Background(accent).
		Bold(true)
	return styles
}

// VerticalDivider renders a subtle vertical rule to separate panes.
func VerticalDivider() string {
	p := CatppuccinMocha()
	return lipgloss.NewStyle().Foreground(p.Surface1).Render("│")
}

// PaneBodyStyle returns a focus-aware pane style with NO top border (for titled panes).
func PaneBodyStyle(focused bool) lipgloss.Style {
	p := CatppuccinMocha()
	border := p.Overlay0
	if focused {
		border = p.Blue
	}
	b := lipgloss.RoundedBorder()
	return lipgloss.NewStyle().
		BorderStyle(b).
		BorderLeft(true).BorderRight(true).BorderBottom(true).BorderTop(false).
		BorderForeground(border).
		Background(p.Mantle).
		// No vertical padding so content starts right under the title bar
		Padding(1)
}

// TitleBar renders a rounded top border with the title centered between the lines.
func TitleBar(title string, width int, focused bool) string {
	if width < 4 {
		width = 4
	}
	p := CatppuccinMocha()
	accent := p.Overlay0
	if focused {
		accent = p.Blue
	}
	left := "╭"
	right := "╮"
	fill := "─"
	// add spaces around title
	titleText := " " + title + " "
	// compute how many fill chars we can draw
	inner := width - lipgloss.Width(left) - lipgloss.Width(right)
	if inner < lipgloss.Width(titleText) {
		// truncate title if too long
		over := lipgloss.Width(titleText) - inner
		if over > 0 {
			// naive truncation
			if over+3 < len(title) {
				title = title[:len(title)-(over+3)] + "..."
			} else if len(title) > 3 {
				title = title[:len(title)-3] + "..."
			}
			titleText = " " + title + " "
		}
	}
	rem := inner - lipgloss.Width(titleText)
	if rem < 0 {
		rem = 0
	}
	leftFill := rem / 2
	rightFill := rem - leftFill // ensure total equals inner, no overflow
	line := left + strings.Repeat(fill, leftFill) + titleText + strings.Repeat(fill, rightFill) + right
	return lipgloss.NewStyle().Foreground(accent).Render(line)
}

// RenderTitledPane composes TitleBar and PaneBodyStyle into a full titled pane.
func RenderTitledPane(title string, width, height int, content string, focused bool) string {
	if width < 4 {
		width = 4
	}
	if height < 2 {
		height = 2 // title (1) + body (1)
	}
	top := TitleBar(title, width, focused)
	bodyH := height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	body := PaneBodyStyle(focused).Width(width).Height(bodyH).Render(content)
	return lipgloss.JoinVertical(lipgloss.Left, top, body)
}

// PlaceholderMessage renders a centered help message for empty panes
func PlaceholderMessage(title, subtitle string, width, height int) string {
	p := CatppuccinMocha()

	titleStyle := lipgloss.NewStyle().
		Foreground(p.Subtext0).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(p.Overlay1)

	message := lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.Render(title),
		"",
		subtitleStyle.Render(subtitle),
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		message,
	)
}
