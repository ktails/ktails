package tui

import (
	"io"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davecgh/go-spew/spew"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/cmds"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/tbl"
)

type Mode int

const (
	ModeListContexts Mode = iota // Loading initial table
	ModeViewing                  // Viewing table
	ModePodViewing               // Viewing pod table
	ModeHelp                     // Help screen
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
	// table Model
	table table.Model
	// dump debug msgs
	Dump io.Writer
}

// initialLoadMsg is sent once at startup to trigger table initialization
type initialLoadMsg struct{}

func NewSimpleTui(client *k8s.Client) *SimpleTui {
	return &SimpleTui{
		// start in loading mode so we can initialize the table after we learn the
		// terminal size (WindowSizeMsg) and populate rows from the k8s client
		mode:       ModeListContexts,
		focusIndex: 0,
		width:      0,
		height:     0,
		client:     client,
		table:      table.Model{},
	}
}

// initContextTable builds the contexts table (columns, rows, size, styles)
func (s *SimpleTui) initContextTable() {
	// build rows
	rows := []table.Row{}
	if s.client != nil {
		if contexts, err := s.client.ListContexts(); err == nil {
			rows = make([]table.Row, 0, len(contexts))
			current := s.client.GetCurrentContext()
			for _, ctx := range contexts {
				currentMarker := ""
				if ctx == current {
					currentMarker = "*"
				}
				// Columns: Context Name, Current, Cluster, Auth Info, Namespace, Extensions, Cluster Endpoint
				ns := ""
				if s.client != nil {
					ns = s.client.DefaultNamespace(ctx)
				}
				rows = append(rows, table.Row{ctx, currentMarker, "", "", ns, "", ""})
			}
		}
	}

	// compute size with sane defaults before first resize
	tableHeight := s.height - 4
	if tableHeight < 3 {
		tableHeight = 10
	}
	tableWidth := s.width - 4
	if tableWidth < 20 {
		tableWidth = 80
	}

	// create table
	s.table = table.New(
		table.WithColumns(tbl.ContextTableColumns()),
		table.WithRows(rows),
		table.WithHeight(tableHeight),
		table.WithWidth(tableWidth),
		table.WithFocused(true),
		table.WithStyles(tbl.CatppuccinTableStyles()),
	)

	// position cursor at current context row if possible
	current := ""
	if s.client != nil {
		current = s.client.GetCurrentContext()
	}
	for i, r := range s.table.Rows() {
		if len(r) > 0 && r[0] == current {
			s.table.SetCursor(i)
			break
		}
	}
}

func (s *SimpleTui) Init() tea.Cmd {
	// initialize the contexts table once at startup
	s.initContextTable()
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
			case ModeViewing:
				// Select current row: mark it as current and optionally switch context
				row := s.table.SelectedRow()
				if len(row) > 0 {
					selectedCtx := row[0]
					// update star in Current column for all rows
					rows := s.table.Rows()
					for i := range rows {
						if len(rows[i]) > 1 {
							if rows[i][0] == selectedCtx {
								rows[i][1] = "*"
							} else {
								rows[i][1] = ""
							}
						}
					}
					s.table.SetRows(rows)
					// Switch k8s client context to keep in sync (best-effort)
					if s.client != nil {
						_ = s.client.SwitchContext(selectedCtx)
					}
					return s, cmds.LoadPodInfoCmd(s.client, selectedCtx, s.client.DefaultNamespace(selectedCtx))
				}

			case ModePodViewing:
				return s, nil
			}

		case "esc":
			if s.mode == ModePodViewing {
				// Go back to context viewing mode
				s.mode = ModeViewing
				// Reinitialize table to show contexts again
				s.table = table.Model{}
				s.mode = ModeListContexts // will trigger re-initialization below
				return s, func() tea.Msg { return initialLoadMsg{} }
			}
		default:
			// unhandled keys will be processed by the table in the mode switch below
		}

	case tea.WindowSizeMsg:
		// capture window size so views (help, table) can use the dimensions
		s.width = msg.Width
		s.height = msg.Height
		// update table size to current window (full height for table view)
		tableHeight := s.height - 4
		if tableHeight < 3 {
			tableHeight = 10
		}
		s.table.SetHeight(tableHeight)
		s.table.SetWidth(s.width - 4)
		// let the table also process the window size (discard any cmd here)
		// so we can continue to the mode initialization below
		s.table.UpdateViewport()
		return s, func() tea.Msg { return initialLoadMsg{} }
	case msgs.PodTableMsg:
		s.table.SetColumns(tbl.PodTableColumns())
		s.table.SetRows(msg.Rows)
		// ensure size is sensible after switching columns/rows
		s.table.SetWidth(s.width - 4)
		// height already set on resize; keep as-is
		s.table.UpdateViewport()
		s.mode = ModePodViewing
		return s, nil
	case initialLoadMsg:
		// (re)initialize the contexts table when requested
		s.initContextTable()
		s.mode = ModeViewing
		return s, nil
	}

	switch s.mode {
	case ModeListContexts:

		s.mode = ModeViewing
		return s, nil
	case ModeViewing:
		s.table, cmd = s.table.Update(msg)
		return s, cmd
	case ModePodViewing:
		s.table, cmd = s.table.Update(msg)
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
	// Show only the table for all non-help modes (contexts and pods)
	body := s.table.View() + "\n"
	return body
}

// === Help Mode ===

func (s *SimpleTui) viewHelp() string {
	// Guard against zero sizes before first WindowSizeMsg
	w, h := s.width, s.height

	tableHelp := s.table.HelpView()
	if tableHelp == "" {
		tableHelp = "No help available"
	}

	helpText := tbl.HelpBoxStyle().
		Width(w - 4).
		Height(h - 4).
		Render(tableHelp)

	return lipgloss.Place(
		w,
		h,
		lipgloss.Center,
		lipgloss.Center,
		helpText,
	)
}
