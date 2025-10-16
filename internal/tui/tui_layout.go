package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
	"github.com/termkit/skeleton"
)

// calculateLayoutDimensions calculates the dimensions for left and right panes
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

// handleContextsSelected handles the selection of contexts
func (s *SimpleTui) handleContextsSelected(msg msgs.ContextsSelectedMsg) (tea.Model, tea.Cmd) {
	if len(msg.Contexts) == 0 {
		return s, nil
	}

	var batchCmds []tea.Cmd

	// Clear existing pages (except we don't need the "No Pods" check anymore)
	s.layout.PodPages.DeletePage("placeholder")

	// Calculate dimensions first
	dims := s.calculateLayoutDimensions()

	// Create all new panes
	for _, ctxName := range msg.Contexts {
		namespace := s.client.DefaultNamespace(ctxName)

		// Create the pod model
		skel := skeleton.NewSkeleton()
		podPane := models.NewPodsModel(s.client, ctxName, namespace, skel)

		// Set dimensions on the pane
		podPane.SetDimensions(dims.rightPane)

		// Add to skeleton
		s.layout.PodPages.AddPage(ctxName, ctxName, podPane)

		// Queue command to load pod data
		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	// Set skeleton dimensions
	// s.layout.PodPages.SetWidth(dims.rightPane.Width)
	// s.layout.PodPages.SetHeight(dims.rightPane.Height)

	// Apply to context pane
	s.layout.ContextPane.SetDimensions(models.NewDimensions(dims.leftPane.Width, dims.leftPane.Height))

	// Switch mode and focus
	s.mode = ModePodViewing
	s.layout.ContextPane.SetFocused(false)
	// s.layout.PodPages.SetFocused(true)

	// Set first context as active
	// if len(msg.Contexts) > 0 {
	// 	s.layout.PodPages.SwitchPage(msg.Contexts[0])
	// }

	return s, tea.Batch(batchCmds...)
}

// applyPodPaneDimensions applies dimensions to all pod panes in the skeleton
func (s *SimpleTui) applyPodPaneDimensions() {
	if s.width == 0 || s.height == 0 {
		return // Can't calculate dimensions yet
	}

	// dims := s.calculateLayoutDimensions()

	// Update dimensions for all pod panes

}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
