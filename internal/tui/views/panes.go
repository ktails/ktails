// Package views, this is panes.go it will be used to draw two main panes, Left Pane with around 1/3 width and rigth pane with 2/3 width of the window,
// future to divide right pane into rightTopPane and rightBottomPane
package views

import "github.com/ktails/ktails/internal/tui/styles"

// lipgloss v2's Style.Width()/Height() set the block's total rendered size
// including the border, unlike v1 where the border was added on top of the
// declared size (padding, in both versions, counts toward the declared
// size). w/h here are still meant the old v1 way (content+padding, border
// added after) so callers computing layout budgets don't need to know each
// style's border size — that border size is added back before calling
// Width/Height.

func RenderLeftPane(data string, w, h int) string {
	lp := styles.LeftPane.
		Width(w + styles.LeftPane.GetHorizontalBorderSize()).
		Height(h + styles.LeftPane.GetVerticalBorderSize()).
		Render(data)
	return lp
}

func RenderLeftPaneBlur(data string, w, h int) string {
	lp := styles.LeftPaneBlur.
		Width(w + styles.LeftPaneBlur.GetHorizontalBorderSize()).
		Height(h + styles.LeftPaneBlur.GetVerticalBorderSize()).
		Render(data)
	return lp
}
