package views

import (
	"charm.land/lipgloss/v2"
	"github.com/ktails/ktails/internal/tui/styles"
)

// RenderTabHeaders renders tab headers with clear focus/blur styling.
// If blur is true, the tabs are shown in their blurred styles; otherwise focused styles.
func RenderTabHeaders(activeTab int, tabs []string, w int, blur bool) string {
	var renderedTabs []string
	width := w / len(tabs)
	for i, t := range tabs {
		var style lipgloss.Style
		isFirst, isLast, isActive := i == 0, i == len(tabs)-1, i == activeTab
		if blur {
			if isActive {
				style = styles.ActiveTabBlurStyle
			} else {
				style = styles.InactiveTabBlurStyle
			}
		} else {
			if isActive {
				style = styles.ActiveTabStyle
			} else {
				style = styles.InactiveTabStyle
			}
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

		style = style.Border(border)
		// lipgloss v2's Width() sets the total rendered width including the
		// border, unlike v1 where border was added on top of the declared
		// width — add the style's own horizontal border size back so each
		// tab still renders at `width` cells of border+padding+content,
		// matching the layout budget computed by callers of RenderTabHeaders.
		style = style.Width(width + style.GetHorizontalBorderSize())
		renderedTabs = append(renderedTabs, style.Render(t))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	return row
}
