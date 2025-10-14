// Package tui
package tui

import (
	"io"

	"github.com/charmbracelet/bubbles/table"
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

type SimpleTui struct {
	// App state
	mode       Mode
	prevMode   Mode
	mainTabs   int // 0 = left pane, 1..n = right tables
	podPaneIdx int
	width      int
	height     int
	// k8s client
	client *k8s.Client
	// Contexts Info List
	contextInfo []models.ContextsInfo
	// MasterLayout
	layout views.MasterLayout

	// dump debug msgs
	Dump io.Writer
}

// initialLoadMsg is sent once at startup to trigger table initialization
type initialLoadMsg struct{}

func NewSimpleTui(client *k8s.Client) *SimpleTui {
	return &SimpleTui{
		// start in loading mode so we can initialize the table after we learn the
		// terminal size (WindowSizeMsg) and populate rows from the k8s client
		mode:        ModeContextPane,
		mainTabs:    0,
		width:       0,
		height:      0,
		client:      client,
		contextInfo: []models.ContextsInfo{},
		layout:      views.NewLayout(client),
	}
}

// initContextPane

func (s *SimpleTui) initPodListPane() {
	// leave any default panes created by the layout; ensure at least one table exists
	if len(s.layout.PodListPane) == 0 {
		// start with an empty table with sensible defaults
		t := table.New(
			table.WithColumns(views.PodTableColumns()),
			table.WithRows([]table.Row{}),
		)
		t.SetWidth(60)
		t.SetHeight(10)
		t.SetStyles(styles.TableStylesFocused(false))
		s.layout.PodListPane = []table.Model{t}
	} else {
		// apply default blurred styles
		for i := range s.layout.PodListPane {
			s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(false))
		}
	}
}

func (s *SimpleTui) Init() tea.Cmd {
	// initialize panes
	s.initPodListPane()
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
	var cmd tea.Cmd
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
			// Switch focus cycles: 0=contexts, then each table on the right
			n := len(s.layout.PodListPane)
			s.mainTabs = (s.mainTabs + 1) % (n + 1)
			if s.mainTabs == 0 {
				// back to contexts; blur all tables
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)
				for i := range s.layout.PodListPane {
					s.layout.PodListPane[i].Blur()
					s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(false))
				}
				return s, nil
			}
			// focus one of the right tables
			s.mode = ModePodViewing
			s.layout.ContextPane.SetFocused(false)
			s.podPaneIdx = s.mainTabs - 1
			for i := range s.layout.PodListPane {
				if i == s.podPaneIdx {
					s.layout.PodListPane[i].Focus()
					s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(true))
				} else {
					s.layout.PodListPane[i].Blur()
					s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(false))
				}
			}
			return s, nil
		case "shift+tab":
			// Reverse cycle through pod panes; if in contexts and panes exist, jump to last pane
			n := len(s.layout.PodListPane)
			if n == 0 {
				s.mainTabs = 0
				s.mode = ModeContextPane
				s.layout.ContextPane.SetFocused(true)
				return s, nil
			}
			if s.mainTabs == 0 {
				// from contexts, go to last pod pane
				s.mainTabs = n
				s.mode = ModePodViewing
				s.layout.ContextPane.SetFocused(false)
				s.podPaneIdx = n - 1
			} else {
				// from a pod pane, go to previous (wrapping to last)
				if s.mainTabs <= 1 {
					s.mainTabs = n
				} else {
					s.mainTabs--
				}
				s.mode = ModePodViewing
				s.layout.ContextPane.SetFocused(false)
				s.podPaneIdx = s.mainTabs - 1
			}
			for i := range s.layout.PodListPane {
				if i == s.podPaneIdx {
					s.layout.PodListPane[i].Focus()
					s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(true))
				} else {
					s.layout.PodListPane[i].Blur()
					s.layout.PodListPane[i].SetStyles(styles.TableStylesFocused(false))
				}
			}
			return s, nil
		}

	case tea.WindowSizeMsg:
		// capture window size
		s.width = msg.Width
		s.height = msg.Height
		// compute available area after outer doc frame and divider
		docFW, docFH := styles.DocStyle().GetFrameSize()
		availW := s.width - docFW
		availH := s.height - docFH
		if availW < 10 {
			availW = s.width // fallback
		}
		if availH < 5 {
			availH = s.height // fallback
		}
		dividerW := 1 // VerticalDivider is a 1-char bar without spacing
		leftW := (availW - dividerW) / 3
		rightW := availW - dividerW - leftW
		if rightW < 0 {
			rightW = 0
		}
		// set left pane bounds and forward msg so it can size inner list
		s.layout.ContextPane.Width = leftW
		s.layout.ContextPane.Height = availH
		cmd = s.layout.ContextPane.Update(msg)
		// divide right area across tables vertically
		n := len(s.layout.PodListPane)
		if n > 0 {
			rowH := availH / n
			if rowH < 3 {
				rowH = 3
			}
			for i := range s.layout.PodListPane {
				s.layout.PodListPane[i].SetWidth(rightW)
				s.layout.PodListPane[i].SetHeight(rowH)
				s.layout.PodListPane[i].UpdateViewport()
			}
		}
		return s, cmd
	case msgs.PodTableMsg:
		// route rows to all current tables (TODO: map by context)
		for i := range s.layout.PodListPane {
			s.layout.PodListPane[i].SetRows(msg.Rows)
		}
		s.mode = ModePodViewing
		s.layout.ContextPane.SetFocused(false)
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
		cmd = s.layout.ContextPane.Update(msg)
		return s, cmd
	case ModePodViewing:
		// forward key events to the focused right table if any
		n := len(s.layout.PodListPane)
		if n > 0 && s.mainTabs > 0 {
			idx := s.mainTabs - 1
			if idx >= 0 && idx < n {
				s.layout.PodListPane[idx], cmd = s.layout.PodListPane[idx].Update(msg)
				return s, cmd
			}
		}
		return s, nil
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
	// build right side by joining all pod tables vertically (one per selected context)
	rights := make([]string, 0, len(s.layout.PodListPane))
	for i := range s.layout.PodListPane {
		rights = append(rights, s.layout.PodListPane[i].View())
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
	case 1:
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
func (s *SimpleTui) handleContextsSelected(msg msgs.ContextsSelectedMsg) (tea.Model, tea.Cmd) {
	if len(msg.Contexts) == 0 {
		return s, nil
	}

	var localCmds []tea.Cmd
	for _, ctxName := range msg.Contexts {
		namespace := s.client.DefaultNamespace(ctxName)
		localCmds = append(localCmds, cmds.LoadPodInfoCmd(s.client, ctxName, namespace))
	}

	s.mode = ModePodViewing
	s.layout.ContextPane.SetFocused(false)
	if len(s.layout.PodListPane) > 0 {
		s.mainTabs = 1
		s.podPaneIdx = 0
		s.layout.PodListPane[0].Focus()
		s.layout.PodListPane[0].SetStyles(styles.TableStylesFocused(true))
	}

	return s, tea.Batch(localCmds...)
}
