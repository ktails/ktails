// Package tui
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ivyascorp-net/ktails/internal/k8s"
)

// Mode represents the current application mode
type Mode int

const (
	ModeSelection Mode = iota // Selecting pods
	ModeViewing               // Viewing logs
	ModeHelp                  // Help screen
)

// Model is the main Bubble Tea model
type Model struct {
	// App state
	mode       Mode
	focusIndex int // 0 = left pane, 1 = right pane
	width      int
	height     int

	// Panes
	panes [2]*Pane

	// Selection state (when mode == ModeSelection)
	selector *Selector

	// K8s client (base)
	k8sClient *k8s.Client

	// UI
	ready bool
}

// NewModel creates a new main model
func NewModel(client *k8s.Client) Model {
	return Model{
		mode:       ModeSelection,
		focusIndex: 0,
		panes: [2]*Pane{
			// TODO: use context-specific clients once contexts are selected
			NewPane(0, client),
			NewPane(1, client),
		},
		selector:  NewSelector(),
		k8sClient: client,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	// type namespacesMsg []NamespacesLoadedMsg

	// Handle global keys first
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case " ":
			// Toggle mark for current highlighted context
			return m, nil

		case "ctrl+c":
			return m, tea.Quit
		case "?":
			// Toggle help mode (only if not in context selection)
			if m.mode != ModeSelection {
				if m.mode == ModeHelp {
					m.mode = ModeViewing
				} else {
					m.mode = ModeHelp
				}
				return m, nil
			}
		case "l":
			// Open selector for focused pane
			if m.mode == ModeViewing {
				m.mode = ModeSelection
				m.selector.Reset(m.focusIndex)
				m.selector.SetSize(m.width, m.height)
				return m, nil
			}
		case "f":
			// Toggle follow mode in focused pane
			m.panes[m.focusIndex].ToggleFollow()
			return m, nil

		case "ctrl+l":
			// Clear logs in focused pane
			m.panes[m.focusIndex].Clear()
			return m, nil

		case "n":
			// Open namespace selector for the focused pane
			m.mode = ModeSelection
			m.selector.Reset(m.focusIndex)
			m.selector.SetSize(m.width, m.height)
			m.selector.step = SelectNamespace
			m.selector.SetLoading(true)
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update selector size
		m.selector.SetSize(msg.Width, msg.Height)

		// Update pane viewports with exact fitting widths
		sepWidth := 1
		leftWidth := (msg.Width - sepWidth) / 2
		rightWidth := msg.Width - sepWidth - leftWidth
		paneHeight := msg.Height - 6
		m.panes[0].SetSize(leftWidth, paneHeight)
		m.panes[1].SetSize(rightWidth, paneHeight)
		return m, nil

	case ContextsLoadedMsg:

		return m, nil

	case []NamespacesLoadedMsg:
		// If we are selecting a namespace, populate selector and stop here
		if m.mode == ModeSelection && m.selector.step == SelectNamespace {
			for _, v := range msg {
				if v.Err != nil {
					m.selector.SetError(v.Err)
				} else {
					m.selector.SetNamespaces(v.Namespaces)
				}
			}
			return m, nil
		}

		// Otherwise, initial load: for each context, decide a default namespace and load pods for that pane
		for _, v := range msg {
			if v.Err != nil {
				m.selector.SetError(v.Err)
				continue
			}

			// issue load pods for correct client
			client := m.k8sClient
			defaultNS := m.k8sClient.DefaultNamespace(v.Context)
			if client != nil {
				cmds = append(cmds, loadPodsForClientCmd(client, v.Context, defaultNS))
			}
		}
		return m, tea.Batch(cmds...)

	case NamespaceSelectedMsg:
		// Load pods for the selected namespace in the chosen pane

		m.mode = ModeViewing
		return m, loadPodsForClientCmd(m.k8sClient, msg.Context, msg.Namespace)

	case PodsLoadedMsg:
		if msg.Err != nil {
			m.selector.SetError(msg.Err)
			return m, nil
		}
		// Determine pane by context and populate pod list

		m.mode = ModeViewing
		return m, nil

	case PodInfoMsg:
		// Pod info arrived for pane
		if msg.Err != nil {
			m.panes[msg.PaneIndex].SetError(msg.Err)
			return m, nil
		}

		m.panes[msg.PaneIndex].ClearDummyData()
		if msg.Info != nil {
			// Do not change context/namespace here to keep them static
			m.panes[msg.PaneIndex].podName = msg.Info.Name
			m.panes[msg.PaneIndex].SetPodInfo(msg.Info)
		}

		m.mode = ModeViewing

		// Start streaming logs for this pod into the pane, using per-pane client
		container := ""
		if msg.Info != nil {
			container = msg.Info.Container
		}
		cli := m.k8sClient
		return m, startLogStreamForClientCmd(msg.PaneIndex, cli, m.panes[msg.PaneIndex].namespace, m.panes[msg.PaneIndex].podName, container)
	}

	// Route to appropriate handler based on mode
	switch m.mode {
	case ModeSelection:
		return m.updateSelection(msg)
	case ModeViewing:
		return m.updateViewing(msg)
	case ModeHelp:
		return m.updateHelp(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing ktails..."
	}

	switch m.mode {
	case ModeSelection:
		return m.viewSelection()
	case ModeViewing:
		return m.viewLogs()
	case ModeHelp:
		return m.viewHelp()
	}

	return ""
}

// === Context Selection Mode ===

// === Selection Mode ===

func (m Model) updateSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel selection, go back to viewing
			m.mode = ModeViewing
			return m, nil
		}
	}

	// Update selector
	cmd := m.selector.Update(msg)
	return m, cmd
}

func (m Model) viewSelection() string {
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		m.selector.View(),
	)
}

// === Viewing Mode ===

func (m Model) updateViewing(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Switch focus between panes
			m.focusIndex = (m.focusIndex + 1) % 2
			return m, nil

		case "shift+tab":
			// Switch focus backwards
			m.focusIndex = (m.focusIndex - 1 + 2) % 2
			return m, nil

		case "ctrl+1":
			m.focusIndex = 0
			return m, nil

		case "ctrl+2":
			m.focusIndex = 1
			return m, nil

		case "f":
			// Toggle follow mode in focused pane
			m.panes[m.focusIndex].ToggleFollow()
			return m, nil

		case "c":
			// Clear logs in focused pane
			m.panes[m.focusIndex].Clear()
			return m, nil

		case "n":
			// Open namespace selector for the focused pane
			m.mode = ModeSelection
			m.selector.Reset(m.focusIndex)
			m.selector.SetSize(m.width, m.height)
			m.selector.step = SelectNamespace
			m.selector.SetLoading(true)
			return m, loadNamespacesForSingleCmd(m.k8sClient, "")
		}

	case LogLineMsg:
		// Route log line to appropriate pane and request next chunk
		if msg.PaneIndex >= 0 && msg.PaneIndex < 2 {
			m.panes[msg.PaneIndex].AddLogLine(msg.Line)
			return m, continueLogStreamCmd(msg.PaneIndex)
		}
		return m, nil

	case ErrorMsg:
		// Show error on pane; try to continue to drain until channel closes
		if msg.PaneIndex >= 0 && msg.PaneIndex < 2 && msg.Err != nil {
			m.panes[msg.PaneIndex].SetError(msg.Err)
		}
		return m, continueLogStreamCmd(msg.PaneIndex)

	case StreamEndedMsg:
		// Stream ended; cleanup channel
		if ch, ok := activeStreamChans[msg.PaneIndex]; ok {
			close(ch)
			delete(activeStreamChans, msg.PaneIndex)
		}
		return m, nil

	case PodInfoMsg:
		// Update pod info for pane
		if msg.PaneIndex >= 0 && msg.PaneIndex < 2 && msg.Info != nil {
			m.panes[msg.PaneIndex].SetPodInfo(msg.Info)
		}
		return m, nil
	}

	// Update focused pane (for scrolling, etc.)
	updatedPane, cmd := m.panes[m.focusIndex].Update(msg)
	m.panes[m.focusIndex] = updatedPane
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) viewLogs() string {
	// Header
	header := m.renderHeader()

	// Calculate pane dimensions (fit exactly with separator)
	sepWidth := 1
	leftWidth := (m.width - sepWidth) / 2
	rightWidth := m.width - sepWidth - leftWidth
	paneHeight := m.height - 6 // -6 for header and footer

	// Render panes
	leftPane := m.panes[0].View(leftWidth, paneHeight, m.focusIndex == 0)
	rightPane := m.panes[1].View(rightWidth, paneHeight, m.focusIndex == 1)

	// Join panes horizontally with separator
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("│")

	panesView := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		separator,
		rightPane,
	)

	// Footer
	footer := m.renderFooter()

	// Combine all
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panesView,
		footer,
	)
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Render("ktails - Multi-Context Log Viewer")

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("Press ? for help | q to quit")

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-len(title)-len(stats)).Render(""),
		stats,
	)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(m.width).
		Render(lipgloss.NewStyle().Width(m.width).Render("─"))

	return lipgloss.JoinVertical(lipgloss.Left, header, divider)
}

func (m Model) renderFooter() string {
	leftKeys := "[Tab] Switch Pane"
	rightKeys := "[s] Search | [n] Namespace | [f] Follow | [c] Clear"

	if m.focusIndex == 0 {
		leftKeys = lipgloss.NewStyle().Bold(true).Render(leftKeys)
	}

	footer := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftKeys,
		lipgloss.NewStyle().Width(m.width-len(leftKeys)-len(rightKeys)).Render(""),
		rightKeys,
	)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(m.width).
		Render("─")

	return lipgloss.JoinVertical(lipgloss.Left, divider, footer)
}

// === Help Mode ===

func (m Model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Any key press exits help mode
	if _, ok := msg.(tea.KeyMsg); ok {
		m.mode = ModeViewing
		return m, nil
	}
	return m, nil
}

func (m Model) viewHelp() string {
	helpText := lipgloss.NewStyle().
		Width(m.width-4).
		Height(m.height-4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(1, 2).
		Render(`
ktails - Keyboard Shortcuts

Navigation:
  Tab / Shift+Tab    Switch between panes
  Ctrl+1, Ctrl+2     Jump to specific pane
  ↑ ↓ / j k          Scroll logs
  g / G              Jump to top/bottom

Actions:
  s                  Select pod for current pane
  f                  Toggle follow mode (auto-scroll)
  c                  Clear logs in current pane
  r                  Reconnect/refresh

View:
  ?                  Show this help
  q / Ctrl+C         Quit

Press any key to return...
`)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		helpText,
	)
}
