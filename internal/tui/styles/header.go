// Package styles is contains layout and styling of the KTails
package styles

import "github.com/charmbracelet/lipgloss"

// DocStyle is an outer wrapper similar to lipgloss layout example.
func NewHeaderStyle() lipgloss.Style {
	p := CatppuccinMocha()
	return lipgloss.NewStyle().Height(DefaultHeaderMargin).
		Background(p.Sapphire).
		Padding(1)
}
func NewFooterStyle() lipgloss.Style {
	p := CatppuccinMocha()
	return lipgloss.NewStyle().Height(DefaultFooterMargin).
		Background(p.Overlay2).
		Padding(1)
}
