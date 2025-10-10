package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "k8s.io/api/core/v1"
)

// SelectionStep represents the current step in pod selection
type SelectionStep int

const (
	SelectContext SelectionStep = iota
	SelectNamespace
	SelectPod
	SelectionComplete
)

// Selector handles the multi-step pod selection process
type Selector struct {
	// Current state
	step       SelectionStep
	targetPane int // Which pane we're selecting for (0 or 1)

	// Selected values
	selectedContext   string
	selectedNamespace string
	selectedPod       string

	// Data
	contexts   []string
	namespaces []string
	pods       []PodItem

	// UI state
	selectedIndex int // Currently highlighted item
	loading       bool
	errorMsg      string
	width         int
	height        int
}

// PodItem represents a pod in the selection list
type PodItem struct {
	Name      string
	Namespace string
	Status    string
	Ready     string // e.g., "2/2"
	Phase     v1.PodPhase
}

// NewSelector creates a new selector
func NewSelector() *Selector {
	return &Selector{
		step:          SelectContext,
		targetPane:    0,
		contexts:      make([]string, 0),
		namespaces:    make([]string, 0),
		pods:          make([]PodItem, 0),
		selectedIndex: 0,
		loading:       false,
	}
}

// Reset resets the selector for a new selection
func (s *Selector) Reset(paneIndex int) {
	s.step = SelectContext
	s.targetPane = paneIndex
	s.selectedContext = ""
	s.selectedNamespace = ""
	s.selectedPod = ""
	s.selectedIndex = 0
	s.loading = false
	s.errorMsg = ""
}

// SetSize updates the selector dimensions
func (s *Selector) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// SetContexts sets available contexts
func (s *Selector) SetContexts(contexts []string, currentContext string) {
	s.contexts = contexts

	// Pre-select current context
	for i, ctx := range contexts {
		if ctx == currentContext {
			s.selectedIndex = i
			break
		}
	}
}

// SetNamespaces sets available namespaces
func (s *Selector) SetNamespaces(namespaces []string) {
	s.namespaces = namespaces
	s.selectedIndex = 0
	s.loading = false
}

// SetPods sets available pods
func (s *Selector) SetPods(pods []PodItem) {
	s.pods = pods
	s.selectedIndex = 0
	s.loading = false
}

// SetError sets an error message
func (s *Selector) SetError(err error) {
	s.errorMsg = err.Error()
	s.loading = false
}

// SetLoading sets loading state
func (s *Selector) SetLoading(loading bool) {
	s.loading = loading
}

// Update handles messages for the selector
func (s *Selector) Update(msg tea.Msg) tea.Cmd {
	if s.loading {
		// Ignore input while loading
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.selectedIndex > 0 {
				s.selectedIndex--
			}
			return nil

		case "down", "j":
			maxIndex := s.getMaxIndex()
			if s.selectedIndex < maxIndex {
				s.selectedIndex++
			}
			return nil

		case "enter":
			return s.handleEnter()

		case "esc":
			// Go back or cancel
			if s.step == SelectContext {
				return nil // Will be handled by parent
			}
			s.goBack()
			return nil
		}
	}

	return nil
}

// handleEnter processes Enter key based on current step
func (s *Selector) handleEnter() tea.Cmd {
	switch s.step {
	case SelectContext:
		if len(s.contexts) > 0 && s.selectedIndex < len(s.contexts) {
			s.selectedContext = s.contexts[s.selectedIndex]
			s.step = SelectionComplete
			s.selectedIndex = 0
			s.loading = true
			return loadPodsCmd(s.selectedContext, "default") // Default namespace
		}

	case SelectNamespace:
		if len(s.namespaces) > 0 && s.selectedIndex < len(s.namespaces) {
			s.selectedNamespace = s.namespaces[s.selectedIndex]
			// Notify model about selected namespace for this pane
			return func() tea.Msg {
				return NamespaceSelectedMsg{
					PaneIndex: s.targetPane,
					Context:   s.selectedContext,
					Namespace: s.selectedNamespace,
				}
			}
		}

	case SelectPod:
		if len(s.pods) > 0 && s.selectedIndex < len(s.pods) {
			s.selectedPod = s.pods[s.selectedIndex].Name
			s.step = SelectionComplete

			// Send selection complete message
			return func() tea.Msg {
				return PodSelectedMsg{
					PaneIndex: s.targetPane,
					Context:   s.selectedContext,
					Namespace: s.selectedNamespace,
					Pod:       s.selectedPod,
				}
			}
		}
	}

	return nil
}

// goBack goes to the previous selection step
func (s *Selector) goBack() {
	s.selectedIndex = 0
	s.errorMsg = ""

	switch s.step {
	case SelectNamespace:
		s.step = SelectContext
		s.selectedNamespace = ""
	case SelectPod:
		s.step = SelectNamespace
		s.selectedPod = ""
	}
}

// getMaxIndex returns the maximum valid index for current step
func (s *Selector) getMaxIndex() int {
	switch s.step {
	case SelectContext:
		return len(s.contexts) - 1
	case SelectNamespace:
		return len(s.namespaces) - 1
	case SelectPod:
		return len(s.pods) - 1
	}
	return 0
}

// View renders the selector
func (s *Selector) View() string {
	// Title based on current step
	var title string
	switch s.step {
	case SelectContext:
		title = fmt.Sprintf("Select Context (Pane %d)", s.targetPane+1)
	case SelectNamespace:
		title = fmt.Sprintf("Select Namespace (Pane %d)", s.targetPane+1)
	case SelectPod:
		title = fmt.Sprintf("Select Pod (Pane %d)", s.targetPane+1)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Render(title)

	// Breadcrumb showing progress
	breadcrumb := s.renderBreadcrumb()

	// Instructions
	instructions := s.renderInstructions()

	// Content based on state
	var content string
	if s.errorMsg != "" {
		content = s.renderError()
	} else if s.loading {
		content = s.renderLoading()
	} else {
		content = s.renderList()
	}

	// Combine all elements
	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		titleStyle,
		breadcrumb,
		"",
		content,
		"",
		instructions,
		"",
	)
}

func (s *Selector) renderBreadcrumb() string {
	parts := []string{}

	if s.selectedContext != "" {
		parts = append(parts, s.selectedContext)
	}
	if s.selectedNamespace != "" {
		parts = append(parts, s.selectedNamespace)
	}
	if s.selectedPod != "" {
		parts = append(parts, s.selectedPod)
	}

	if len(parts) == 0 {
		return ""
	}

	breadcrumbText := strings.Join(parts, " → ")
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Render(breadcrumbText)
}

func (s *Selector) renderInstructions() string {
	baseInstructions := "[↑↓/j/k] Navigate | [Enter] Select"

	if s.step != SelectContext {
		baseInstructions += " | [Esc] Back"
	} else {
		baseInstructions += " | [Esc] Cancel"
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(baseInstructions)
}

func (s *Selector) renderError() string {
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9")).
		Padding(1, 2).
		Width(s.width - 10)

	return errorStyle.Render(fmt.Sprintf("Error: %s\n\nPress Esc to go back", s.errorMsg))
}

func (s *Selector) renderLoading() string {
	var loadingText string
	switch s.step {
	case SelectNamespace:
		loadingText = "Loading namespaces..."
	case SelectPod:
		loadingText = "Loading pods..."
	default:
		loadingText = "Loading..."
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Render(loadingText)
}

func (s *Selector) renderList() string {
	var items []string
	var maxItems int

	switch s.step {
	case SelectContext:
		items = s.contexts
	case SelectNamespace:
		items = s.namespaces
	case SelectPod:
		items = s.renderPodItems()
	}

	if len(items) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("No items available")
	}

	// Calculate how many items we can show
	availableHeight := s.height - 10
	if availableHeight < 5 {
		availableHeight = 5
	}
	maxItems = availableHeight

	// Render items with scrolling
	var displayItems []string
	startIdx := 0
	endIdx := len(items)

	// If list is too long, calculate scroll window
	if len(items) > maxItems {
		// Center selected item in view
		startIdx = s.selectedIndex - maxItems/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + maxItems
		if endIdx > len(items) {
			endIdx = len(items)
			startIdx = endIdx - maxItems
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	// Show scroll indicator if needed
	if startIdx > 0 {
		displayItems = append(displayItems, lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("  ↑ more items above"))
	}

	// Render visible items
	for i := startIdx; i < endIdx; i++ {
		style := lipgloss.NewStyle().PaddingLeft(2)

		if i == s.selectedIndex {
			// Selected item
			style = style.
				Foreground(lipgloss.Color("170")).
				Bold(true)
			displayItems = append(displayItems, style.Render("→ "+items[i]))
		} else {
			// Unselected item
			style = style.Foreground(lipgloss.Color("250"))
			displayItems = append(displayItems, style.Render("  "+items[i]))
		}
	}

	// Show scroll indicator if needed
	if endIdx < len(items) {
		displayItems = append(displayItems, lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("  ↓ more items below"))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left, displayItems...)

	// Box around list
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(s.width - 10)

	return boxStyle.Render(listContent)
}

func (s *Selector) renderPodItems() []string {
	items := make([]string, len(s.pods))
	for i, pod := range s.pods {
		// Format: podName (Status) [Ready]
		statusColor := "250"
		switch pod.Phase {
		case v1.PodRunning:
			statusColor = "10" // Green
		case v1.PodPending:
			statusColor = "11" // Yellow
		case v1.PodFailed:
			statusColor = "9" // Red
		}

		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
		status := statusStyle.Render(string(pod.Phase))

		items[i] = fmt.Sprintf("%s (%s) [%s]", pod.Name, status, pod.Ready)
	}
	return items
}

// IsComplete returns true if selection is complete
func (s *Selector) IsComplete() bool {
	return s.step == SelectionComplete
}

// GetSelection returns the complete selection
func (s *Selector) GetSelection() (context, namespace, pod string) {
	return s.selectedContext, s.selectedNamespace, s.selectedPod
}
