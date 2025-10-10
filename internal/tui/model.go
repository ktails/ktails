// Package tui
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ivyascorp-net/ktails/internal/k8s"
)

// Mode represents the current application mode
type Mode int

const (
	ModeContextSelect Mode = iota // NEW: Select initial context
	ModeSelection                 // Selecting pods
	ModeViewing                   // Viewing logs
	ModeHelp                      // Help screen
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

	// Context selection
	contexts     []string // Available contexts
	selectedCtx  int      // Currently selected context index
	contextError string   // Error loading contexts

	// Per-selected context clients and pane mappings
	selectedContexts map[string]*k8s.Client // contextName -> client bound to that context
	paneContexts     [2]string              // pane index -> context name
	contextToPane    map[string]int         // context name -> pane index
	paneClients      [2]*k8s.Client         // pane index -> client

	// Multi-select state for context selection screen
	selectedCtxMarked []bool
	contextNotice     string // transient notice shown on context select screen

	// UI
	ready bool
}

// NewModel creates a new main model
func NewModel(client *k8s.Client) Model {
	return Model{
		mode:       ModeContextSelect,
		focusIndex: 0,
		panes: [2]*Pane{
			// TODO: use context-specific clients once contexts are selected
			NewPane(0, client),
			NewPane(1, client),
		},
		selector:          NewSelector(),
		k8sClient:         client,
		contexts:          []string{},
		contextToPane:     make(map[string]int),
		contextNotice:     "",
		selectedContexts:  make(map[string]*k8s.Client),
		selectedCtxMarked: []bool{},
	}
}

func (m Model) Init() tea.Cmd {
	return loadContextsCmd(m.k8sClient)
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
			if len(m.contexts) == 0 {
				return m, nil
			}
			if len(m.selectedCtxMarked) != len(m.contexts) {
				m.selectedCtxMarked = make([]bool, len(m.contexts))
			}
			m.selectedCtxMarked[m.selectedCtx] = !m.selectedCtxMarked[m.selectedCtx]
			m.contextNotice = "Toggled selection"
			return m, nil

		case "ctrl+c":
			return m, tea.Quit
		case "?":
			// Toggle help mode (only if not in context selection)
			if m.mode != ModeContextSelect && m.mode != ModeSelection {
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
				m.selector.SetContexts(m.contexts, m.k8sClient.GetCurrentContext())
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
			ctxName := m.paneContexts[m.focusIndex]
			cli := m.paneClients[m.focusIndex]
			if ctxName == "" || cli == nil {
				return m, nil
			}
			m.mode = ModeSelection
			m.selector.Reset(m.focusIndex)
			m.selector.SetSize(m.width, m.height)
			m.selector.selectedContext = ctxName
			m.selector.step = SelectNamespace
			m.selector.SetLoading(true)
			return m, loadNamespacesForSingleCmd(cli, ctxName)
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
		if msg.Err != nil {
			m.contextError = msg.Err.Error()
		} else {
			m.contexts = msg.Contexts
			// Find current context index
			for i, ctx := range msg.Contexts {
				if ctx == msg.Current {
					m.selectedCtx = i
					break
				}
			}
		}
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
			client := m.selectedContexts[v.Context]
			defaultNS := m.k8sClient.DefaultNamespace(v.Context)
			if client != nil {
				cmds = append(cmds, loadPodsForClientCmd(client, v.Context, defaultNS))
			}
		}
		return m, tea.Batch(cmds...)

	case NamespaceSelectedMsg:
		// Load pods for the selected namespace in the chosen pane
		cli := m.selectedContexts[msg.Context]
		if paneIdx := msg.PaneIndex; paneIdx >= 0 && paneIdx < 2 {
			if cli == nil {
				cli = m.paneClients[paneIdx]
			}
		}
		m.mode = ModeViewing
		if cli != nil {
			return m, loadPodsForClientCmd(cli, msg.Context, msg.Namespace)
		}
		return m, nil

	case PodsLoadedMsg:
		if msg.Err != nil {
			m.selector.SetError(msg.Err)
			return m, nil
		}
		// Determine pane by context and populate pod list
		if m.contextToPane != nil {
			if paneIdx, ok := m.contextToPane[msg.Context]; ok && paneIdx >= 0 && paneIdx < 2 {
				m.panes[paneIdx].SetPodListWithContext(msg.Pods, msg.Context, msg.Namespace)
			}
		}
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
		cli := m.paneClients[msg.PaneIndex]
		return m, startLogStreamForClientCmd(msg.PaneIndex, cli, m.panes[msg.PaneIndex].namespace, m.panes[msg.PaneIndex].podName, container)
	}

	// Route to appropriate handler based on mode
	switch m.mode {
	case ModeContextSelect:
		return m.updateContextSelect(msg)
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
	case ModeContextSelect:
		return m.viewContextSelect()
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

func (m Model) updateContextSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.contextError != "" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				return m, tea.Quit
			}
		}
		return m, nil
	}

	// If no contexts loaded yet, wait
	if len(m.contexts) == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedCtx > 0 {
				m.selectedCtx--
			}
			return m, nil

		case "down", "j":
			if m.selectedCtx < len(m.contexts)-1 {
				m.selectedCtx++
			}
			return m, nil

		case "enter":
			// Proceed only if at least two contexts are marked
			if len(m.selectedCtxMarked) != len(m.contexts) {
				m.contextNotice = "Select at least two contexts (Space to toggle)"
				return m, nil
			}

			// collect marked contexts (keep order of contexts list)
			var picks []string
			for i, marked := range m.selectedCtxMarked {
				if marked {
					picks = append(picks, m.contexts[i])
				}
			}

			if len(picks) < 2 {
				m.contextNotice = "Please choose at least two contexts"
				return m, nil
			}

			// Create dedicated clients for the first two picks
			m.selectedContexts = make(map[string]*k8s.Client, 2)
			m.contextToPane = make(map[string]int, 2)
			for i := 0; i < 2; i++ {
				ctxName := picks[i]
				cli, err := k8s.NewClient("")
				if err != nil {
					m.contextError = fmt.Sprintf("failed to init client for %s: %v", ctxName, err)
					return m, nil
				}
				if err := cli.SwitchContext(ctxName); err != nil {
					m.contextError = fmt.Sprintf("failed to switch to %s: %v", ctxName, err)
					return m, nil
				}
				m.selectedContexts[ctxName] = cli
				m.paneContexts[i] = ctxName
				m.paneClients[i] = cli
				m.contextToPane[ctxName] = i
			}

			// Enter viewing and trigger namespace + pod loading for both panes
			m.mode = ModeViewing
			return m, loadNamespacesCmd(m.selectedContexts)

		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) viewContextSelect() string {
	if m.contextError != "" {
		// Show error
		errorBox := lipgloss.NewStyle().
			Width(m.width-4).
			Height(m.height-4).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("9")). // Red
			Padding(1, 2).
			Render(fmt.Sprintf(`
Error Loading Contexts

%s

Press Ctrl+C or q to quit
`, m.contextError))

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			errorBox,
		)
	}

	if len(m.contexts) == 0 {
		// Loading state
		loading := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("Loading Kubernetes contexts...")

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			loading,
		)
	}

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Render("ktails - Select Kubernetes Context")

	// Instructions
	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("[↑↓ / j/k] Navigate | [Space] Mark | [Enter] Confirm | [Ctrl+C] Quit")

	// Build context list with multi-select markers
	var contextItems []string
	for i, ctx := range m.contexts {
		style := lipgloss.NewStyle().PaddingLeft(2)

		marker := "[ ] "
		if i < len(m.selectedCtxMarked) && m.selectedCtxMarked[i] {
			marker = "[x] "
		}

		if i == m.selectedCtx {
			style = style.Foreground(lipgloss.Color("170")).Bold(true)
			contextItems = append(contextItems, style.Render("→ "+marker+ctx))
		} else {
			style = style.Foreground(lipgloss.Color("250"))
			contextItems = append(contextItems, style.Render("  "+marker+ctx))
		}
	}

	contextList := lipgloss.JoinVertical(lipgloss.Left, contextItems...)

	// Box around context list
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(60)

	contextBox := boxStyle.Render(contextList)

	// Combine everything
	pieces := []string{"", title, "", contextBox, "", instructions}
	if m.contextNotice != "" {
		notice := lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Render(m.contextNotice)
		pieces = append(pieces, "", notice)
	}
	content := lipgloss.JoinVertical(lipgloss.Center, pieces...)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

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
			ctxName := m.paneContexts[m.focusIndex]
			cli := m.paneClients[m.focusIndex]
			if ctxName == "" || cli == nil {
				return m, nil
			}
			m.mode = ModeSelection
			m.selector.Reset(m.focusIndex)
			m.selector.SetSize(m.width, m.height)
			m.selector.selectedContext = ctxName
			m.selector.step = SelectNamespace
			m.selector.SetLoading(true)
			return m, loadNamespacesForSingleCmd(cli, ctxName)
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
