package tui

import (
	"io"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davecgh/go-spew/spew"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	tbl "github.com/ivyascorp-net/ktails/internal/tui/styles"
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
		layout:      views.NewLayout(),
	}
}

// initContextPane
func (s *SimpleTui) initContextPane() {
	// build rows
	if s.client != nil {

		ctxs, _ := s.client.ListContexts()

		for _, v := range ctxs {
			s.contextInfo = append(s.contextInfo, models.ContextsInfo{
				Name:      v,
				Namespace: s.client.DefaultNamespace(v),
				IsCurrent: v == s.client.GetCurrentContext(),
			})
		}

		s.layout.ContextPane.Title = "Kubernetes Contexts"
		// convert to []list.Item
		items := make([]list.Item, 0, len(s.contextInfo))
		for i := range s.contextInfo {
			items = append(items, s.contextInfo[i])
		}
		s.layout.ContextPane.SetItems(items)
		s.layout.ContextPane.SetShowStatusBar(false)
		s.layout.ContextPane.SetFilteringEnabled(false)
		s.layout.ContextPane.SetShowHelp(false)
		s.layout.ContextPane.SetShowPagination(false)
		s.layout.ContextPane.SetWidth(s.width / 3)
		s.layout.ContextPane.SetHeight(s.height)
		// set the current context with a star
		currentCtx := s.client.GetCurrentContext()
		its := s.layout.ContextPane.Items()
		for i := range its {
			if its[i].FilterValue() == currentCtx {
				s.layout.ContextPane.Select(i)
				break
			}
		}
	}
}

func (s *SimpleTui) initPodListPane() {
	// start with an empty slice of tables
	s.layout.PodListPane = []table.Model{}
}

func (s *SimpleTui) Init() tea.Cmd {
	// initialize the contexts table once at startup
	s.initContextPane()
	s.initPodListPane()
	// start directly in context pane so Enter works immediately
	s.mode = ModeContextPane
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
		case " ":
			// Toggle selection of the current context
			if s.mode == ModeContextPane {
				i := s.layout.ContextPane.Index()
				if i >= 0 && i < len(s.contextInfo) {
					// flip Selected flag
					s.contextInfo[i].Selected = !s.contextInfo[i].Selected
					// rebuild items so Title() reflects selection
					items := make([]list.Item, 0, len(s.contextInfo))
					for k := range s.contextInfo {
						items = append(items, s.contextInfo[k])
					}
					s.layout.ContextPane.SetItems(items)
					// maintain cursor position
					s.layout.ContextPane.Select(i)
				}
				return s, nil
			}

		case "enter":
			switch s.mode {
			case ModeContextPane:
				// gather selected contexts; if none, use highlighted
				selected := make([]models.ContextsInfo, 0)
				for _, c := range s.contextInfo {
					if c.Selected {
						selected = append(selected, c)
					}
				}
				if len(selected) == 0 {
					i := s.layout.ContextPane.Index()
					if i < 0 || i >= len(s.contextInfo) {
						return s, nil
					}
					selected = append(selected, s.contextInfo[i])
				}
				// create a table per selected context (or reuse if exists) and load rows
				cmdsBatch := make([]tea.Cmd, 0, len(selected))
				// reset tables for a fresh view
				s.layout.PodListPane = make([]table.Model, 0, len(selected))
				for _, ctx := range selected {
					// initialize a new table for this context
					t := table.New(
						table.WithColumns(views.PodTableColumns()),
						table.WithRows([]table.Row{}),
					)
					t.SetStyles(tbl.CatppuccinTableStyles())
					// Will set sizes on WindowSizeMsg below
					s.layout.PodListPane = append(s.layout.PodListPane, t)
					// queue command to load rows
					ns := ctx.Namespace
					if ns == "" && s.client != nil {
						ns = s.client.DefaultNamespace(ctx.Name)
					}
					cmdsBatch = append(cmdsBatch, cmds.LoadPodInfoCmd(s.client, ctx.Name, ns))
				}
				// focus pod view and run all load commands
				s.mode = ModePodViewing
				s.mainTabs = 1
				s.podPaneIdx = 0
				// focus first table, blur others
				for i := range s.layout.PodListPane {
					if i == 0 {
						s.layout.PodListPane[i].Focus()
					} else {
						s.layout.PodListPane[i].Blur()
					}
				}
				return s, tea.Batch(cmdsBatch...)

			case ModePodViewing:
				return s, nil
			}
		case "tab":
			// Switch focus cycles: 0=contexts, then each table on the right
			n := len(s.layout.PodListPane)
			s.mainTabs = (s.mainTabs + 1) % (n + 1)
			if s.mainTabs == 0 {
				// back to contexts; blur all tables
				s.mode = ModeContextPane
				for i := range s.layout.PodListPane {
					s.layout.PodListPane[i].Blur()
				}
				return s, nil
			}
			// focus one of the right tables
			s.mode = ModePodViewing
			s.podPaneIdx = s.mainTabs - 1
			for i := range s.layout.PodListPane {
				if i == s.podPaneIdx {
					s.layout.PodListPane[i].Focus()
				} else {
					s.layout.PodListPane[i].Blur()
				}
			}
			return s, nil
		case "shift+tab":
			// Reverse cycle through pod panes; if in contexts and panes exist, jump to last pane
			n := len(s.layout.PodListPane)
			if n == 0 {
				s.mainTabs = 0
				s.mode = ModeContextPane
				return s, nil
			}
			if s.mainTabs == 0 {
				// from contexts, go to last pod pane
				s.mainTabs = n
				s.mode = ModePodViewing
				s.podPaneIdx = n - 1
			} else {
				// from a pod pane, go to previous (wrapping to last)
				if s.mainTabs <= 1 {
					s.mainTabs = n
				} else {
					s.mainTabs--
				}
				s.mode = ModePodViewing
				s.podPaneIdx = s.mainTabs - 1
			}
			for i := range s.layout.PodListPane {
				if i == s.podPaneIdx {
					s.layout.PodListPane[i].Focus()
				} else {
					s.layout.PodListPane[i].Blur()
				}
			}
			return s, nil
		
		default:
			// unhandled keys will be processed by the components below
		}

	case tea.WindowSizeMsg:
		// capture window size
		s.width = msg.Width
		s.height = msg.Height
		s.layout.ContextPane.SetWidth(s.width / 3)
		s.layout.ContextPane.SetHeight(s.height)
		// divide right area across tables vertically
		rightW := s.width - s.width/3
		n := len(s.layout.PodListPane)
		if n == 0 {
			return s, nil
		}
		colW := rightW
		rowH := s.height / n
		if rowH < 3 {
			rowH = 3
		}
		for i := range s.layout.PodListPane {
			s.layout.PodListPane[i].SetWidth(colW)
			s.layout.PodListPane[i].SetHeight(rowH)
			s.layout.PodListPane[i].UpdateViewport()
		}
		return s, nil
	case msgs.PodTableMsg:
		// route rows to all current tables (TODO: map by context)
		for i := range s.layout.PodListPane {
			s.layout.PodListPane[i].SetRows(msg.Rows)
		}
		s.mode = ModePodViewing
		return s, nil
	case initialLoadMsg:
		// (re)initialize the contexts table when requested
		s.initContextPane()
		s.mode = ModeContextPane
		return s, nil
	}

	switch s.mode {
	case ModeContextPane:
		s.layout.ContextPane, cmd = s.layout.ContextPane.Update(msg)
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
	v := lipgloss.JoinHorizontal(lipgloss.Left, left, right)
	return v
}

// === Help Mode ===

func (s *SimpleTui) viewHelp() string {
	// Guard against zero sizes before first WindowSizeMsg
	w, h := s.width, s.height
	helpText := ""

	switch s.mainTabs {
	case 0:
		// focus left (contexts)
		helpText = s.layout.ContextPane.Help.FullHelpView(s.layout.ContextPane.FullHelp())
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
