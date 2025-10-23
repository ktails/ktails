// Package styles is contains layout and styling of the KTails
package styles

import "github.com/charmbracelet/lipgloss"

var header lipgloss.Style

func GetHeader() lipgloss.Style {
	header = lipgloss.NewStyle().Height(DefaultHeaderMargin)
	return header
}

func GetFooter() lipgloss.Style {
	header = lipgloss.NewStyle().Height(DefaultHeaderMargin)
	return header
}
