// Package views owns the master layout of ktails: the Context List box on
// the left, the Tab Area box on the right, and the status bar beneath them.
// Solve (layout.go) is the single source of the width/height budget.
package views

const (
	// MinHeight is the minimum height of the TUI.
	MinHeight = 24
	// MinContentWidth is the minimum width of the TUI.
	MinContentWidth = 80
	// FooterHeight is the height of the status bar at the bottom of the TUI.
	FooterHeight = 1
	// MaxLeftPaneWidth caps how wide the left (context list) pane is allowed
	// to grow on wide terminals — its content (short context names) never
	// needs a fixed fraction of a very wide window, so beyond this the extra
	// space goes to the tab area instead.
	MaxLeftPaneWidth = 40
)
