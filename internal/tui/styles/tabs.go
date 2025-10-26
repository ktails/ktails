package styles

import (
	"github.com/charmbracelet/lipgloss"
)

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

var (
	DefaultTabs       = []string{"Kubernetes Contexts", "Deployments", "Pods"}
	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")
	activeTabBorder   = tabBorderWithBottom("┘", " ", "└")
	DocStyle          = lipgloss.NewStyle().Padding(1, 2, 1, 2).BorderStyle(lipgloss.InnerHalfBlockBorder())
	highlightColor    = lipgloss.AdaptiveColor{Light: string(CatppuccinMocha().Flamingo), Dark: string(CatppuccinMocha().Mauve)}
	inactiveTabStyle  = lipgloss.NewStyle().Border(inactiveTabBorder, true).BorderForeground(highlightColor).Padding(0, 1)
	activeTabStyle    = inactiveTabStyle.Border(activeTabBorder, true)
	WindowStyle       = lipgloss.NewStyle().BorderForeground(highlightColor).Padding(2, 0).Align(lipgloss.Center).Border(lipgloss.NormalBorder()).UnsetBorderTop()
)

func RenderTabHeaders(activeTab int, tabs []string, w, h int) string {
	var renderedTabs []string
	width := w / len(tabs)
	for i, t := range tabs {
		var style lipgloss.Style
		isFirst, isLast, isActive := i == 0, i == len(tabs)-1, i == activeTab
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
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
