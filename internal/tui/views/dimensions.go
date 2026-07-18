// Package views contains master layout of KTail, Left Pane(k8s context), Right Top Pane(pod list view), Right Bottom Pane(Pod Details views)
package views

const (
	// MinHeight is the minimum height of the TUI.
	MinHeight = 24
	// Height of prompt including borders
	PromptHeight = 3
	// FooterHeight is the height of the footer at the bottom of the TUI.
	FooterHeight = 1
	// Height of help widget, including borders
	HelpWidgetHeight = 12
	// MinContentHeight is the minimum height of content above the footer.
	MinContentHeight = MinHeight - FooterHeight
	// MinContentWidth is the minimum width of the content.
	MinContentWidth = 80
	// minimum height of each pane
	minPaneHeight = 4
	// minimum width of each pane
	minPaneWidth = 20
	// defaultTopRightPaneHeight is the default height of the top right pane.
	defaultTopRightPaneHeight = 15
	// MaxLeftPaneWidth caps how wide the left (context list) pane is allowed
	// to grow on wide terminals — its content (short context names) never
	// needs a fixed fraction of a very wide window, so beyond this the extra
	// space goes to the tab area instead.
	MaxLeftPaneWidth = 40
)

func init() {
	if (minPaneHeight*2)+PromptHeight+HelpWidgetHeight > MinContentHeight {
		panic("mininum heights of panes, prompt, footer, and help cannot exceed overall minimum height")
	}
	if minPaneWidth*2 > MinContentWidth {
		panic("minimum width of panes must be no more than half of the minimum content width")
	}
	if minPaneHeight > defaultTopRightPaneHeight {
		panic("default top right pane height must not be lower than the overall minimum height")
	}
	if minPaneWidth > MaxLeftPaneWidth {
		panic("default left pane width must not be lower than the overall minimum width")
	}
}
