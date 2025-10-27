package views

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/ktails/ktails/internal/tui/styles"
)

func RenderTabHeaders(activeTab int, tabs []string, w int, focus bool) string {
	var renderedTabs []string
	width := w / len(tabs)
	for i, t := range tabs {
		var style lipgloss.Style
		isFirst, isLast, isActive := i == 0, i == len(tabs)-1, i == activeTab
		if isActive && focus {
			style = styles.ActiveTabStyle
		} else if focus {
			style = styles.ActiveTabStyle
		} else if !isActive && !focus {
			style = styles.ActiveTabBlurStyle
		} else {
			style = styles.InactiveTabBlurStyle
		}

		border, _, _, _, _ := style.GetBorder()
		if isFirst && isActive {
			border.BottomLeft = "│"
		} else if isFirst && !isActive {
			border.BottomLeft = "├"
		} else if isLast && isActive {
			border.BottomRight = "│"
		} else if isLast && !isActive {
			border.BottomRight = "┤"
		}

		style = style.Border(border).Width(width)
		renderedTabs = append(renderedTabs, style.Render(t))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	return row
}
