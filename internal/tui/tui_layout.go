package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
	"github.com/termkit/skeleton"
)

// import (
// 	tea "github.com/charmbracelet/bubbletea"
// 	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
// 	"github.com/ivyascorp-net/ktails/internal/tui/models"
// 	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
// 	"github.com/ivyascorp-net/ktails/internal/tui/styles"
// )

func newPodPanes() {

}

// // calculateLayoutDimensions calculates the dimensions for left and right panes
func (s *SimpleTui) calculateLayoutDimensions() layoutDimensions {
	// Get frame size from doc style
	docFW, docFH := styles.DocStyle().GetFrameSize()

	// Calculate available space
	availW := max(s.width-docFW, 10)
	availH := max(s.height-docFH, 2)

	// Account for divider (1 char width)
	dividerW := 1

	// Left pane is 1/3 of width, right pane is 2/3
	leftW := max((availW-dividerW)/3, 20)    // minimum 20 for readability
	rightW := max(availW-dividerW-leftW, 30) // minimum 30 for tables

	return layoutDimensions{
		leftPane:  models.NewDimensions(leftW, availH),
		rightPane: models.NewDimensions(rightW, availH),
	}
}

// // handleContextsSelected handles the selection of contexts
func (s *SimpleTui) handleContextsSelected(msg msgs.ContextsSelectedMsg) (tea.Model, tea.Cmd) {
	if len(msg.Contexts) == 0 {
		return s, nil
	}

	var batchCmds []tea.Cmd

	if s.layout.PodPages.GetActivePage() == "No Pods" {
		s.layout.PodPages.DeletePage("No Pods")
	}
	s.layout.PodPages = skeleton.NewSkeleton()

	// Create all new panes
	for _, ctxName := range msg.Contexts {
		s.layout.PodPages.AddPage(ctxName, "", models.NewPodsModel(s.client, ctxName, s.client.DefaultNamespace(ctxName), s.layout.PodPages))

		namespace := s.client.DefaultNamespace(ctxName)
		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	// Apply dimensions to ALL panes AFTER they're all added
	// This ensures correct sizing on first render
	dims := s.calculateLayoutDimensions()
	// Apply to context pane (always full available height)
	s.layout.ContextPane.SetDimensions(models.NewDimensions(dims.leftPane.Width, dims.leftPane.Height))
	s.applyPodPaneDimensions()

	// Switch mode and focus
	s.mode = ModePodViewing
	s.layout.ContextPane.SetFocused(false)
	s.layout.PodPages.SetActivePage(msg.Contexts[0])
	return s, tea.Batch(batchCmds...)
}

// applyPodPaneDimensions applies dimensions to all pod panes
func (s *SimpleTui) applyPodPaneDimensions() {
	if s.width == 0 || s.height == 0 {
		return // Can't calculate dimensions yet
	}

	// dims := s.calculateLayoutDimensions()
	
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
