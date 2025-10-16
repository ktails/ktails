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

	// Clear placeholder pane if it exists
	if len(s.layout.PodListPane) == 1 && s.layout.PodListPane[0].ContextName == "" {
		s.layout.PodListPane = []*models.Pods{}
	}

	// Create all new panes
	for _, ctxName := range msg.Contexts {
		// Check if context already exists
		exists := false
		for _, p := range s.layout.PodListPane {
			if p.ContextName == ctxName {
				exists = true
				break
			}
		}

		if !exists {
			namespace := s.client.DefaultNamespace(ctxName)
			p := models.NewPodsModel(s.client, ctxName, namespace)
			// Add to layout
			s.layout.PodListPane = append(s.layout.PodListPane, p)
		}

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

	for _, p := range s.layout.PodListPane {
		p.SetFocused(false)
	}

	if len(s.layout.PodListPane) > 0 {
		s.mainTabs = 1
		s.podPaneIdx = 0
		s.layout.PodListPane[0].SetFocused(true)
	}

	return s, tea.Batch(batchCmds...)
}

// applyPodPaneDimensions applies dimensions to all pod panes
func (s *SimpleTui) applyPodPaneDimensions() {
	if s.width == 0 || s.height == 0 {
		return // Can't calculate dimensions yet
	}

	dims := s.calculateLayoutDimensions()
	n := len(s.layout.PodListPane)

	if n == 0 {
		return
	}

	if n == 1 {
		// Single pane gets full height
		s.layout.PodListPane[0].SetDimensions(models.NewDimensions(dims.rightPane.Width, dims.rightPane.Height))
	} else {
		// Multiple panes: distribute height evenly
		const minHeight = 8 // minimum height for usability (title + header + few rows)
		totalAvailable := dims.rightPane.Height

		// Check if we can give each pane the minimum height
		if n*minHeight > totalAvailable {
			// Not enough space, just divide evenly without enforcing minimum
			baseH := totalAvailable / n
			remainder := totalAvailable % n

			for i, pane := range s.layout.PodListPane {
				h := baseH
				if i < remainder {
					h++
				}
				pane.SetDimensions(models.NewDimensions(dims.rightPane.Width, max(h, 3)))
			}
		} else {
			// Enough space, distribute evenly
			baseH := totalAvailable / n
			remainder := totalAvailable % n

			for i, pane := range s.layout.PodListPane {
				h := baseH
				// Distribute remainder to first panes
				if i < remainder {
					h++
				}
				pane.SetDimensions(models.NewDimensions(dims.rightPane.Width, h))
			}
		}
	}
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
