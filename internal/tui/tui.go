// Package tui
package tui

import (
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
	width    int
	height   int
	tooSmall bool
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
	layout := views.NewLayout(client)

	// Create initial placeholder pane for the right side
	placeholder := models.NewPodsModel(client, "", "")
	placeholder.PaneTitle = "Select a context to view pods"
	layout.PodListPane = []*models.Pods{placeholder}
	return &SimpleTui{
		mode:     ModeContextPane,
		mainTabs: 0,
		width:    0,
		height:   0,
		client:   client,
		layout:   layout,
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

		// Check minimum terminal size
		if s.width < 80 || s.height < 24 {
			s.tooSmall = true
			return s, nil
		}
		s.tooSmall = false

		// Calculate layout dimensions once
		dims := s.calculateLayoutDimensions()

		// Apply to context pane (always full available height)
		s.layout.ContextPane.SetDimensions(models.NewDimensions(dims.leftPane.Width, dims.leftPane.Height))

		// Apply to all pod panes using helper
		s.applyPodPaneDimensions()

		return s, nil

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
	if s.tooSmall {
		return styles.DocStyle().Render("Terminal too small. Please resize to at least 80x24.")
	}
	if s.mode == ModeHelp {
		return s.viewHelp()
	}
	rights := make([]string, len(s.layout.PodListPane))
	for i, pane := range s.layout.PodListPane {
		rights[i] = pane.View()
	}
	right := lipgloss.JoinVertical(lipgloss.Left, rights...)
	left := s.layout.ContextPane.View()
	content := lipgloss.JoinHorizontal(lipgloss.Left, left, styles.VerticalDivider(), right)
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

// handle layout dimensions
func (s *SimpleTui) calculateLayoutDimensions() layoutDimensions {
	// Get frame size from doc style
	docFW, docFH := styles.DocStyle().GetFrameSize()

	// Calculate available space
	availW := max(s.width-docFW, 10)
	availH := max(s.height-docFH, 2)

	// Account for divider (1 char)
	dividerW := 1

	// Left pane is 1/3 of width
	leftW := (availW - dividerW) / 3
	rightW := availW - dividerW - leftW

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

	// Clear placeholder pane if it exists (empty context name means placeholder)
	if len(s.layout.PodListPane) == 1 && s.layout.PodListPane[0].ContextName == "" {
		s.layout.PodListPane = []*models.Pods{}
	}

	// Track which panes are newly created
	newPanes := make([]*models.Pods, 0)

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

			// Apply dimensions IMMEDIATELY before adding to layout
			if s.width > 0 && s.height > 0 {
				dims := s.calculateLayoutDimensions()
				// Temporarily give it full right pane dimensions
				// We'll redistribute after all panes are created
				p.SetDimensions(models.NewDimensions(dims.rightPane.Width, dims.rightPane.Height))
			}

			s.layout.PodListPane = append(s.layout.PodListPane, p)
			newPanes = append(newPanes, p)
		}

		namespace := s.client.DefaultNamespace(ctxName)
		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	// Now redistribute dimensions across ALL panes if terminal size is known
	if s.width > 0 && s.height > 0 {
		s.applyPodPaneDimensions()
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
		// Multiple panes: distribute height evenly with remainder
		baseH := dims.rightPane.Height / n
		remainder := dims.rightPane.Height % n

		for i, pane := range s.layout.PodListPane {
			h := baseH
			if i < remainder {
				h++
			}
			if h < 3 {
				h = 3
			}
			pane.SetDimensions(models.NewDimensions(dims.rightPane.Width, h))
		}
	}
}
