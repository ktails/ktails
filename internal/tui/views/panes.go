// Package views, this is panes.go it will be used to draw two main panes, Left Pane with around 1/3 width and rigth pane with 2/3 width of the window,
// future to divide right pane into rightTopPane and rightBottomPane
package views

import "github.com/charmbracelet/lipgloss"

var (
	leftPane = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Padding(2, 0)
)

func RenderLeftPane(data string, w, h int) string {

	lp := leftPane.Width(w).Height(h).Render(data)
	return lp

}
