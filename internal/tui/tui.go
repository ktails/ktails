// Package tui
package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davecgh/go-spew/spew"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
	"github.com/ivyascorp-net/ktails/internal/tui/views"
)

type Mode int

const (
	ModeContextPane Mode = iota // Viewing context pane
	ModePodViewing              // Viewing pod table
	ModeHelp                    // Help screen
)

type layoutDimensions struct {
	leftPane  models.Dimensions
	rightPane models.Dimensions
}

type SimpleTui struct {
	// App state
	mode       Mode
	prevMode   Mode
	mainTabs   int // 0 = left pane, 1..n = right tables
	podPaneIdx int // current pod pane index when in pod viewing mode
	// terminal size
	width  int
	height int
	// k8s client
	client *k8s.Client
	// MasterLayout
	layout views.MasterLayout

	// dump debug msgs
	Dump io.Writer
}

// initialLoadMsg is sent once at startup to trigger table initialization
type initialLoadMsg struct{}

func NewSimpleTui(client *k8s.Client) *SimpleTui {
	return &SimpleTui{
		mode:     ModeContextPane,
		mainTabs: 0,
		width:    0,
		height:   0,
		client:   client,
		layout:   views.NewLayout(client),
	}
}

func (s *SimpleTui) Init() tea.Cmd {
	// initialize the context list model so it has data
	s.layout.ContextPane.Init()
	// start directly in context pane
	s.mode = ModeContextPane
	s.layout.ContextPane.SetFocused(true)
	return nil
}

func (s *SimpleTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// check key press msg and update model accordingly
	if s.Dump != nil {
		spew.Fdump(s.Dump, msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global key handling
		if s.mode == ModeHelp {
			// Any key exits help back to previous mode
			s.mode = s.prevMode
			return s, nil
		}
		switch msg.String() {
		case "?":
			// Enter help and remember previous mode (context or pod view)
			s.prevMode = s.mode
			s.mode = ModeHelp
			return s, nil
		case "q", "ctrl+c":
			return s, tea.Quit

		case "tab":
			n := len(s.layout.PodListPane)
			s.mainTabs = (s.mainTabs + 1) % (n + 1)

			// Blur all
			s.layout.ContextPane.SetFocused(false)
			for _, p := range s.layout.PodListPane {
				p.SetFocused(false)
			}

			if s.mainTabs == 0 {
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)
			} else {
				s.mode = ModePodViewing
				s.podPaneIdx = s.mainTabs - 1
				s.layout.PodListPane[s.podPaneIdx].SetFocused(true)
			}
			return s, nil

		case "shift+tab":
			// Reverse cycle through pod panes
			n := len(s.layout.PodListPane)
			if n == 0 {
				s.mainTabs = 0
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)
				return s, nil
			}

			// Blur all
			s.layout.ContextPane.SetFocused(false)
			for _, p := range s.layout.PodListPane {
				p.SetFocused(false)
			}

			if s.mainTabs == 0 {
				// from contexts, go to last pod pane
				s.mainTabs = n
				s.mode = ModePodViewing
				s.podPaneIdx = n - 1
			} else {
				// from a pod pane, go to previous
				s.mainTabs--
				if s.mainTabs == 0 {
					s.mode = ModeContextPane
					s.layout.ContextPane.SetFocused(true)
				} else {
					s.mode = ModePodViewing
					s.podPaneIdx = s.mainTabs - 1
					s.layout.PodListPane[s.podPaneIdx].SetFocused(true)
				}
			}
			return s, nil
		}

	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height

		// Calculate layout dimensions once
		dims := s.calculateLayoutDimensions()

		// Apply to context pane
		s.layout.ContextPane.SetDimensions(dims.leftPane)

		// Apply to all pod panes
		for _, pane := range s.layout.PodListPane {
			pane.SetDimensions(dims.rightPane)
		}

		return s, nil
		// // capture window size
		// s.width = msg.Width
		// s.height = msg.Height
		// // compute available area after outer doc frame and divider
		// docFW, docFH := styles.DocStyle().GetFrameSize()
		// availW := s.width - docFW
		// availH := s.height - docFH
		// if availW < 10 {
		// 	availW = s.width // fallback
		// }
		// if availH < 5 {
		// 	availH = s.height // fallback
		// }
		// dividerW := 1 // VerticalDivider is a 1-char bar without spacing
		// leftW := (availW - dividerW) / 3
		// rightW := availW - dividerW - leftW
		// if rightW < 0 {
		// 	rightW = 0
		// }
		// // set left pane bounds and forward msg so it can size inner list
		// s.layout.ContextPane.Width = leftW
		// s.layout.ContextPane.Height = availH
		// batchCmds := []tea.Cmd{}
		// ctxPaneCmd := s.layout.ContextPane.Update(msg)
		// // divide right area across tables vertically
		// n := len(s.layout.PodListPane)
		// if n > 0 {
		// 	rowH := availH / n
		// 	if rowH < 3 {
		// 		rowH = 3
		// 	}

		// 	for i := range s.layout.PodListPane {
		// 		s.layout.PodListPane[i].Width = rightW
		// 		s.layout.PodListPane[i].Height = rowH
		// 		batchCmds = append(batchCmds, s.layout.PodListPane[i].Update(msg))
		// 	}
		// }
		// return s, tea.Batch(append(batchCmds, ctxPaneCmd)...)

	case msgs.PodTableMsg:
		for _, pane := range s.layout.PodListPane {
			if pane.ContextName == msg.Context {
				pane.UpdateRows(msg.Rows)
				break
			}
		}
		return s, nil

	case initialLoadMsg:
		// (re)initialize the contexts table when requested
		s.mode = ModeContextPane
		s.layout.ContextPane.SetFocused(true)
		return s, nil

	case msgs.ContextsSelectedMsg:
		return s.handleContextsSelected(msg)

	default:
		// unhandled
	}

	switch s.mode {
	case ModeContextPane:
		// forward to context pane but keep root model
		cmd := s.layout.ContextPane.Update(msg)
		return s, cmd

	case ModePodViewing:
		if s.podPaneIdx >= 0 && s.podPaneIdx < len(s.layout.PodListPane) {
			cmd := s.layout.PodListPane[s.podPaneIdx].Update(msg)
			return s, cmd
		}

	case ModeHelp:
		// handle help mode key presses (handled above), no table updates here
		return s, nil
	}

	return s, nil
}

func (s *SimpleTui) View() string {
	if s.mode == ModeHelp {
		return s.viewHelp()
	}
	rights := make([]string, len(s.layout.PodListPane))
	for i, pane := range s.layout.PodListPane {
		rights[i] = pane.View()
	}
	right := lipgloss.JoinVertical(lipgloss.Left, rights...)
	left := s.layout.ContextPane.View()
	debug := s.debugDimensions()
	content := lipgloss.JoinHorizontal(lipgloss.Left, left, styles.VerticalDivider(), right)
	content = lipgloss.JoinVertical(lipgloss.Left, debug, content)
	return styles.DocStyle().Render(content)
}

// === Help Mode ===

func (s *SimpleTui) viewHelp() string {
	// Guard against zero sizes before first WindowSizeMsg
	w, h := s.width, s.height
	helpText := ""

	switch s.mainTabs {
	case 0:
		// focus left (contexts)
		helpText = s.layout.ContextPane.HelpView()
		helpText += "\n\nPress Tab to switch to Pod List pane"
		helpText += "\nPress Enter to select context(s) and view pods"
	default:
		// focus right (pods)
		helpText = "Arrow keys to navigate tables. Tab to cycle forward, Shift+Tab to cycle backward."
	}
	return lipgloss.Place(
		w,
		h,
		lipgloss.Center,
		lipgloss.Center,
		helpText,
	)
}

// === Handle ContextsSelectedMsg ===
// func (s *SimpleTui) handleContextsSelected(msg msgs.ContextsSelectedMsg) (tea.Model, tea.Cmd) {
// 	if len(msg.Contexts) == 0 {
// 		return s, nil
// 	}

// 	// Create panes for any new contexts and prepare load commands
// 	var batchCmds []tea.Cmd

// 	for _, ctxName := range msg.Contexts {
// 		// Check if context already exists
// 		exists := false
// 		for _, p := range s.layout.PodListPane {
// 			if p.ContextName == ctxName {
// 				exists = true
// 				break
// 			}
// 		}

// 		if !exists {
// 			namespace := s.client.DefaultNamespace(ctxName)
// 			p := models.NewPodsModel(s.client, ctxName, namespace)
// 			s.layout.PodListPane = append(s.layout.PodListPane, p)
// 		}

// 		// Always load data (refresh)
// 		namespace := s.client.DefaultNamespace(ctxName)
// 		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
// 	}

// 	// Size newly added panes if we already know terminal size
// 	if s.width > 0 && s.height > 0 {
// 		docFW, docFH := styles.DocStyle().GetFrameSize()
// 		availW := s.width - docFW
// 		availH := s.height - docFH
// 		dividerW := 1
// 		leftW := (availW - dividerW) / 3
// 		rightW := availW - dividerW - leftW
// 		n := len(s.layout.PodListPane)
// 		rowH := availH
// 		if n > 0 {
// 			rowH = availH / n
// 			if rowH < 3 {
// 				rowH = 3
// 			}
// 		}
// 		for i := range s.layout.PodListPane {
// 			s.layout.PodListPane[i].Width = rightW
// 			s.layout.PodListPane[i].Height = rowH
// 		}
// 	}

// 	// Switch mode and focus first pod pane
// 	s.mode = ModePodViewing
// 	s.layout.ContextPane.SetFocused(false)

// 	// Blur all pod panes first
// 	for _, p := range s.layout.PodListPane {
// 		p.SetFocused(false)
// 	}

// 	// Focus first pane (index 0)
// 	if len(s.layout.PodListPane) > 0 {
// 		s.mainTabs = 1
// 		s.podPaneIdx = 0
// 		s.layout.PodListPane[0].SetFocused(true)
// 	}

//		return s, tea.Batch(batchCmds...)
//	}
//
// handle layout dimensions
func (s *SimpleTui) calculateLayoutDimensions() layoutDimensions {
	// Get frame size from doc style
	docFW, docFH := styles.DocStyle().GetFrameSize()

	// Calculate available space
	availW := max(s.width-docFW, 10)
	availH := max(s.height-docFH, 5)

	// Account for divider (1 char)
	dividerW := 1

	// Left pane is 1/3 of width
	leftW := (availW - dividerW) / 3
	rightW := availW - dividerW - leftW

	// Right pane height divided by number of panes
	n := len(s.layout.PodListPane)
	rightH := availH
	if n > 0 {
		rightH = availH / n
	}

	return layoutDimensions{
		leftPane:  models.NewDimensions(leftW, availH),
		rightPane: models.NewDimensions(rightW, rightH),
	}
}

// === Usage example in handleContextsSelected ===

func (s *SimpleTui) handleContextsSelected(msg msgs.ContextsSelectedMsg) (tea.Model, tea.Cmd) {
	if len(msg.Contexts) == 0 {
		return s, nil
	}

	var batchCmds []tea.Cmd

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
			s.layout.PodListPane = append(s.layout.PodListPane, p)
		}

		namespace := s.client.DefaultNamespace(ctxName)
		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	// Apply dimensions if terminal size is known
	if s.width > 0 && s.height > 0 {
		dims := s.calculateLayoutDimensions()
		for _, pane := range s.layout.PodListPane {
			pane.SetDimensions(dims.rightPane)
		}
	}

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

// ============================================
// DEBUGGING HELPER
// ============================================
// Add this to your tui.go to debug dimension calculations

func (s *SimpleTui) debugDimensions() string {
	dims := s.calculateLayoutDimensions()
	return fmt.Sprintf(
		"Terminal: %dx%d | Left: %dx%d | Right: %dx%d | Panes: %d",
		s.width, s.height,
		dims.leftPane.Width, dims.leftPane.Height,
		dims.rightPane.Width, dims.rightPane.Height,
		len(s.layout.PodListPane),
	)
}
