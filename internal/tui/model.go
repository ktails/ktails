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
	focusIndex int // 0 = left pane, 1 = right pane
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
		focusIndex:  0,
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
	s.layout.PodListPane.SetColumns(views.PodTableColumns())
	// ensure header shows by setting empty rows
	s.layout.PodListPane.SetRows([]table.Row{})
	// style to ensure visibility
	s.layout.PodListPane.SetStyles(tbl.CatppuccinTableStyles())
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
		case "enter":
			switch s.mode {
			case ModeContextPane:
				// In context pane, enter selects the context and switches to pod viewing
				i := s.layout.ContextPane.Index()
				if i < 0 || i >= len(s.contextInfo) {
					// invalid index, ignore
					return s, nil
				}
				ctx := s.contextInfo[i]
				// Update k8s client current context
				if s.client != nil {
					s.client.SwitchContext(ctx.Name)
				}
				// choose a namespace (prefer context’s namespace, else default)
				ns := ctx.Namespace
				if ns == "" && s.client != nil {
					ns = s.client.DefaultNamespace(ctx.Name)
				}
				return s, cmds.LoadPodInfoCmd(s.client, ctx.Name, ns)

			case ModePodViewing:
				return s, nil
			}
		case "tab":
			// Switch focus between left and right panes
			s.focusIndex = (s.focusIndex + 1) % 2
			if s.focusIndex == 0 {
				// focus left (contexts)
				s.mode = ModeContextPane
				s.layout.PodListPane.Blur()
			} else {
				// focus right (pods)
				s.mode = ModePodViewing
				s.layout.PodListPane.Focus()
			}
			return s, nil

		default:
			// unhandled keys will be processed by the table in the mode switch below
		}

	case tea.WindowSizeMsg:
		// capture window size so views (help, table) can use the dimensions
		s.width = msg.Width
		s.height = msg.Height
		s.layout.ContextPane.SetWidth(s.width / 3)
		s.layout.ContextPane.SetHeight(s.height)
		s.layout.PodListPane.SetWidth(s.width * 2 / 3)
		s.layout.PodListPane.SetHeight(s.height)
		// important: update viewport after resize so table computes layout
		s.layout.PodListPane.UpdateViewport()
		return s, nil
	case msgs.PodTableMsg:
		// Update pod table with new rows and switch to pod viewing mode
		s.layout.PodListPane.SetRows(msg.Rows)
		// ensure the pod table is focused to receive key events
		s.layout.PodListPane.Focus()
		s.focusIndex = 1
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
		s.layout.PodListPane, cmd = s.layout.PodListPane.Update(msg)
		return s, cmd
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
	// Show both panes side by side without extra newline
	left := s.layout.ContextPane.View()
	right := s.layout.PodListPane.View()
	v := lipgloss.JoinHorizontal(lipgloss.Left, left, right)
	return v
}

// === Help Mode ===

func (s *SimpleTui) viewHelp() string {
	// Guard against zero sizes before first WindowSizeMsg
	w, h := s.width, s.height
	return lipgloss.Place(
		w,
		h,
		lipgloss.Center,
		lipgloss.Center,
		s.layout.ContextPane.Help.FullHelpView(s.layout.ContextPane.FullHelp()),
	)
}
