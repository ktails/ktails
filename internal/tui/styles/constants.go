package styles

import "github.com/charmbracelet/lipgloss"

const (
	DefaultHeaderMargin int = 5
	DefaultFooterMargin int = 3
)

var (
	focusColor = CatppuccinLatte().Surface0
	blurColor  = CatppuccinLatte().Overlay1
	LeftPane   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).
			Padding(2, 0).BorderForeground(focusColor)
	LeftPaneBlur = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).
			Padding(2, 0).BorderForeground(blurColor)

	DefaultTabs          = []string{"Deployments", "Pods"}
	InactiveTabBorder    = TabBorderWithBottom("┴", "─", "┴")
	ActiveTabBorder      = TabBorderWithBottom("┘", " ", "└")
	DocStyle             = lipgloss.NewStyle().Padding(1, 2, 1, 2).BorderStyle(lipgloss.InnerHalfBlockBorder())
	InactiveTabStyle     = lipgloss.NewStyle().Border(InactiveTabBorder, true).BorderForeground(focusColor).Padding(0, 1)
	ActiveTabStyle       = InactiveTabStyle.Border(ActiveTabBorder, true)
	WindowStyle          = lipgloss.NewStyle().BorderForeground(focusColor).Padding(2, 0).Align(lipgloss.Center).Border(lipgloss.NormalBorder()).UnsetBorderTop()
	InactiveTabBlurStyle = lipgloss.NewStyle().Border(InactiveTabBorder, true).BorderForeground(blurColor).Padding(0, 1)
	ActiveTabBlurStyle   = InactiveTabStyle.Border(ActiveTabBorder, true)
	WindowBlurStyle      = lipgloss.NewStyle().BorderForeground(blurColor).Padding(2, 0).Align(lipgloss.Center).Border(lipgloss.NormalBorder()).UnsetBorderTop()
)

func TabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}
