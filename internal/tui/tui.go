// Package tui
package tui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davecgh/go-spew/spew"
	"github.com/ivyascorp-net/ktails/internal/k8s"
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
			if s.mode == ModePodViewing {
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)

			} else if s.mode == ModeContextPane {
				s.mode = ModePodViewing
				s.layout.ContextPane.SetFocused(false)
			}
			return s, nil

		case "shift+tab":
			if s.mode == ModePodViewing {
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)

			} else if s.mode == ModeContextPane {
				s.mode = ModePodViewing
				s.layout.ContextPane.SetFocused(false)
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
		s.layout.PodPages.SetActivePage(msg.Context)
		_, cmd := s.layout.PodPages.Update(msg)
		return s, cmd

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
		// forward to current pod pane but keep root model
		var cmd tea.Cmd
		_, cmd = s.layout.PodPages.Update(msg)
		return s, cmd

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
	rights := s.layout.PodPages.View()
	right := lipgloss.JoinVertical(lipgloss.Left, rights)
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
