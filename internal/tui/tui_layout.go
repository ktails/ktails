package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
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

	// Clear existing pages and tracking
	s.layout.PodPages.DeletePage("placeholder")
	// Clear the tracking map
	s.podPanes = make(map[string]*models.Pods)

	// Calculate dimensions first
	dims := s.calculateLayoutDimensions()

	// Set skeleton dimensions BEFORE adding pages
	s.layout.PodPages.SetTerminalViewportHeight(dims.rightPane.Width)
	s.layout.PodPages.SetTerminalViewportWidth(dims.rightPane.Height)

	// Create all new panes
	for _, ctxName := range msg.Contexts {
		namespace := s.client.DefaultNamespace(ctxName)

		// Create the pod model (removed unused 's' parameter)
		podPane := models.NewPodsModel(s.client, ctxName, namespace)

		// Set dimensions on the pane BEFORE adding to skeleton
		podPane.SetDimensions(dims.rightPane)

		// Track the pane in our map
		s.podPanes[ctxName] = podPane

		// Add to skeleton - skeleton will handle it as tea.Model
		s.layout.PodPages.AddPage(ctxName, ctxName, podPane)

		// Queue command to load pod data
		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	// Apply to context pane
	s.layout.ContextPane.SetDimensions(models.NewDimensions(dims.leftPane.Width, dims.leftPane.Height))

	// Switch mode and focus
	s.mode = ModePodViewing
	s.layout.ContextPane.SetFocused(false)

	// Set first context as active
	if len(msg.Contexts) > 0 {
		s.layout.PodPages.SetActivePage(msg.Contexts[0])
		// Update focus for the active pane
		s.updatePodFocus()
	}

	return s, tea.Batch(batchCmds...)
}

// applyPodPaneDimensions applies dimensions to all pod panes in the skeleton
func (s *SimpleTui) applyPodPaneDimensions() {
	if s.width == 0 || s.height == 0 {
		return
	}

	dims := s.calculateLayoutDimensions()

	// Set skeleton dimensions
	s.layout.PodPages.SetTerminalViewportWidth(dims.rightPane.Width)
	s.layout.PodPages.SetTerminalViewportHeight(dims.rightPane.Height)

	// Update dimensions for all tracked pod panes
	for _, podPane := range s.podPanes {
		podPane.SetDimensions(dims.rightPane)
	}
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// updatePodFocus updates the focus state of the currently active pod pane
func (s *SimpleTui) updatePodFocus() {
	focused := s.mode == ModePodViewing

	// Get the currently active page name from skeleton
	activePage := s.layout.PodPages.GetActivePage()

	// Update focus for all panes (unfocus inactive, focus active)
	for ctxName, pane := range s.podPanes {
		pane.SetFocused(focused && ctxName == activePage)
	}
}
