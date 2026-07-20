package styles

import "charm.land/lipgloss/v2"

const (
	DefaultHeaderMargin int = 5
	DefaultFooterMargin int = 3
)

// ASCIIBorder is a lipgloss.Border built entirely from plain ASCII
// (-, |, +). Unicode box-drawing characters (U+2500-U+257F) carry the
// "Ambiguous" East Asian Width property, so terminals that treat ambiguous
// runes as double-width (e.g. Ghostty's default grapheme-width-method =
// unicode) disagree with lipgloss's own (narrow) width accounting on how
// many cells a border line occupies. That mismatch compounds across a
// frame and eventually pushes rows — including the bottom status bar —
// out of the viewport. ASCII characters have no such ambiguity on any
// terminal, so every border in the app is built from this one.
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

var (
	// Use a single palette across the TUI for consistency
	// Focused elements use a vibrant accent; blurred elements use a subtle overlay
	focusColor = CatppuccinMocha().Mauve
	blurColor  = CatppuccinMocha().Overlay0
	LeftPane   = lipgloss.NewStyle().Border(ASCIIBorder(), true).
			Padding(2, 0).BorderForeground(focusColor)
	LeftPaneBlur = lipgloss.NewStyle().Border(ASCIIBorder(), true).
			Padding(2, 0).BorderForeground(blurColor)

	DefaultTabs          = []string{"Deployments", "Pods"}
	InactiveTabBorder    = TabBorderWithBottom("+", "-", "+")
	ActiveTabBorder      = TabBorderWithBottom("+", " ", "+")
	DocStyle             = lipgloss.NewStyle().Padding(1, 2, 1, 2).BorderStyle(ASCIIBorder())
	InactiveTabStyle     = lipgloss.NewStyle().Border(InactiveTabBorder, true).BorderForeground(focusColor).Padding(0, 1)
	ActiveTabStyle       = InactiveTabStyle.Border(ActiveTabBorder, true)
	WindowStyle          = lipgloss.NewStyle().BorderForeground(focusColor).Padding(2, 0).Align(lipgloss.Center).Border(ASCIIBorder()).UnsetBorderTop()
	InactiveTabBlurStyle = lipgloss.NewStyle().Border(InactiveTabBorder, true).BorderForeground(blurColor).Padding(0, 1)
	ActiveTabBlurStyle   = InactiveTabBlurStyle.Border(ActiveTabBorder, true)
	WindowBlurStyle      = lipgloss.NewStyle().BorderForeground(blurColor).Padding(2, 0).Align(lipgloss.Center).Border(ASCIIBorder()).UnsetBorderTop()

	// status bar
	StatusBar = lipgloss.NewStyle()
)

func TabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := ASCIIBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}
