package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ivyascorp-net/ktails/internal/k8s"
)

// PaneStatus represents the status of a pane
type PaneStatus int

const (
	PaneEmpty     PaneStatus = iota // No pod selected
	PaneLoading                     // Loading pod info
	PaneStreaming                   // Streaming logs
	PanePaused                      // Stream paused
	PaneError                       // Error state
)

// Pane represents a single log viewing pane
type Pane struct {
	// Identity
	index int // 0 = left, 1 = right

	// K8s context
	context   string
	namespace string
	podName   string
	client    *k8s.Client

	// Pod info
	podInfo *k8s.PodInfo

	// Logs
	logLines  []string
	maxLines  int  // Max lines to keep in memory
	following bool // Auto-scroll to bottom

	// Pod list mode
	podList         []PodItem
	filteredPodList []PodItem
	showPodList     bool
	listSelected    int

	// Search state
	searchInput textinput.Model
	searching   bool
	filterTerm  string
	matchCount  int

	// UI
	viewport viewport.Model
	status   PaneStatus

	// State
	lastUpdate  time.Time
	lastRefresh time.Time
	errorMsg    string
}

// PodInfo contains pod metadata
type PodInfo struct {
	Name      string
	Namespace string
	Status    string // Running, Pending, Failed, etc.
	Restarts  int
	Age       time.Duration
	Image     string
	Container string // For multi-container pods
	Node      string
}

// NewPane creates a new pane
func NewPane(index int, client *k8s.Client) *Pane {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 200
	ti.Width = 20

	p := &Pane{
		index:       index,
		client:      client,
		logLines:    make([]string, 0),
		maxLines:    1000,
		following:   true,
		status:      PaneEmpty,
		viewport:    vp,
		searchInput: ti,
	}

	return p
}

// SetPodInfo updates pod information
func (p *Pane) SetPodInfo(info *k8s.PodInfo) { // CHANGE: Do not modify context/namespace here
	p.podInfo = info
	// keep existing p.context and p.namespace; they change only on context/namespace selection
	p.podName = info.Name
	p.status = PaneStreaming
}

// SetSize updates the pane dimensions
func (p *Pane) SetSize(width, height int) {
	// Subtract 2 for left/right borders so viewport content doesn't clip borders
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}
	p.viewport.Width = innerWidth
	p.viewport.Height = height - 7 // Reserve space for pod info header
	// Adjust search input width to inner width with small padding
	if innerWidth > 4 {
		p.searchInput.Width = innerWidth - 2
	}
	p.updateViewportContent()
}

// StartSearch enables search mode for the pane
func (p *Pane) StartSearch() {
	p.searching = true
	p.searchInput.CursorStart()
	p.searchInput.Focus()
	p.applyFilter(p.searchInput.Value())
}

// EndSearch disables search mode and clears filter
func (p *Pane) EndSearch() {
	p.searching = false
	p.searchInput.Blur()
	p.filterTerm = ""
	p.matchCount = 0
	p.filteredPodList = nil
}

// applyFilter filters podList based on the term
func (p *Pane) applyFilter(term string) {
	p.filterTerm = strings.ToLower(strings.TrimSpace(term))
	if p.filterTerm == "" {
		p.filteredPodList = nil
		p.matchCount = len(p.podList)
		return
	}
	var out []PodItem
	for _, pod := range p.podList {
		name := strings.ToLower(pod.Name)
		ns := strings.ToLower(pod.Namespace)
		if strings.Contains(name, p.filterTerm) || strings.Contains(ns, p.filterTerm) {
			out = append(out, pod)
		}
	}
	p.filteredPodList = out
	p.matchCount = len(out)
	// Clamp selection within range
	maxIdx := len(p.getVisibleList()) - 1
	if p.listSelected > maxIdx {
		p.listSelected = maxIdx
	}
	if p.listSelected < 0 {
		p.listSelected = 0
	}
}

// getVisibleList returns filtered or full list depending on search
func (p *Pane) getVisibleList() []PodItem {
	if p.searching && p.filteredPodList != nil {
		return p.filteredPodList
	}
	return p.podList
}

// Update handles messages for this pane
func (p *Pane) Update(msg tea.Msg) (*Pane, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If showing pod list, handle navigation and selection
		if p.showPodList {
			// Start search with 'S'
			if msg.String() == "S" || msg.String() == "s" && !p.searching {
				p.StartSearch()
				return p, nil
			}

			// If searching, first update text input
			if p.searching {
				// Exit search on ESC
				if msg.String() == "esc" {
					p.EndSearch()
					return p, nil
				}
				p.searchInput, _ = p.searchInput.Update(msg)
				p.applyFilter(p.searchInput.Value())
				// Do not return yet; allow navigation keys to also work
			}

			switch msg.String() {
			case "up", "k":
				if p.listSelected > 0 {
					p.listSelected--
				}
				return p, nil

			case "down", "j":
				visible := p.getVisibleList()
				if p.listSelected < len(visible)-1 {
					p.listSelected++
				}
				return p, nil

			case "enter":
				// Select pod and start streaming logs
				visible := p.getVisibleList()
				if p.listSelected >= 0 && p.listSelected < len(visible) {
					sel := visible[p.listSelected]
					// keep existing context (set when pods were loaded for this pane)
					p.namespace = sel.Namespace
					p.podName = sel.Name

					targetPane := p.index
					if p.index == 0 {
						targetPane = 1
					} else {
						p.showPodList = false
					}

					return p, tea.Batch(
						loadPodInfoCmd(p.client, targetPane, p.context, p.namespace, p.podName),
					)
				}
				return p, nil
			}
		}

		// Otherwise, handle viewport scrolling
		p.viewport, cmd = p.viewport.Update(msg)
		return p, cmd
	}

	return p, nil
}

// View renders the pane“
func (p *Pane) View(width, height int, focused bool) string {
	// Update size if needed (compare against inner width)
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}
	if p.viewport.Width != innerWidth || p.viewport.Height != height-7 {
		p.SetSize(innerWidth, height)
	}

	// Render pod info header
	ctxBox := p.renderContextBox(width)
	var header string
	if p.podInfo != nil {
		podBox := p.renderPodInfoBox(innerWidth)
		header = lipgloss.JoinVertical(lipgloss.Left, ctxBox, podBox)
	} else {
		header = ctxBox
	}

	// Render logs viewport or pod list
	var body string
	if p.showPodList {
		body = p.renderPodList(innerWidth)
		// Prepend search input if searching
		if p.searching {
			inputLine := lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Render("Search: " + p.searchInput.View())
			body = lipgloss.JoinVertical(lipgloss.Left, inputLine, body)
		}
	} else {
		body = p.viewport.View()
	}

	// Combine header and body
	content := lipgloss.JoinVertical(lipgloss.Left, header, body)

	// Apply border style based on focus and status
	borderColor := lipgloss.Color("240") // Default gray
	if focused {
		borderColor = lipgloss.Color("170") // Purple when focused
	}

	style := lipgloss.NewStyle().
		Width(innerWidth).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	return style.Render(content)
}

// renderContextBox renders a small box with Context and Namespace
func (p *Pane) renderContextBox(width int) string {
	inner := width - 4
	if inner < 0 {
		inner = 0
	}
	ctx := p.context
	if ctx == "" {
		ctx = "-"
	}
	ns := p.namespace
	if ns == "" {
		ns = "-"
	}
	contextLine := fmt.Sprintf("Context:   %s", ctx)
	nsLine := fmt.Sprintf("Namespace: %s", ns)
	content := lipgloss.JoinVertical(lipgloss.Left, contextLine, nsLine)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(inner)
	return box.Render(content)
}

// renderPodInfoBox renders detailed pod info if available
func (p *Pane) renderPodInfoBox(width int) string {
	inner := width - 2
	if inner < 0 {
		inner = 0
	}
	if p.podInfo == nil {
		return ""
	}
	podLine := fmt.Sprintf("Pod:       %s", p.podName)
	statusText := p.podInfo.Status
	ageText := p.podInfo.Age
	restarts := int(p.podInfo.Restarts)

	var statusStyle lipgloss.Style
	switch statusText {
	case "Running":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	case "Pending":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	default:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	}
	statusLine := fmt.Sprintf("Status:    %s", statusStyle.Render(statusText))
	ageLine := fmt.Sprintf("Age:       %s", ageText)
	restartsStyle := lipgloss.NewStyle()
	if restarts > 0 {
		restartsStyle = restartsStyle.Foreground(lipgloss.Color("11"))
	}
	restartsLine := fmt.Sprintf("Restarts:  %s", restartsStyle.Render(fmt.Sprintf("%d", restarts)))

	infoText := lipgloss.JoinVertical(
		lipgloss.Left,
		podLine,
		statusLine,
		ageLine,
		restartsLine,
	)

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", inner))

	content := lipgloss.JoinVertical(lipgloss.Left, infoText, divider)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(inner)
	return box.Render(content)
}

// AddLogLine adds a log line to the pane
func (p *Pane) AddLogLine(line string) {
	p.logLines = append(p.logLines, line)

	// Trim old lines if exceeding max
	if len(p.logLines) > p.maxLines {
		p.logLines = p.logLines[len(p.logLines)-p.maxLines:]
	}

	p.lastUpdate = time.Now()
	p.updateViewportContent()

	// Auto-scroll to bottom if following
	if p.following {
		p.viewport.GotoBottom()
	}
}

// SetError sets the pane to error state
func (p *Pane) SetError(err error) {
	p.status = PaneError
	p.errorMsg = err.Error()
}

// ClearError clears the error state
func (p *Pane) ClearError() {
	p.status = PaneEmpty
	p.errorMsg = ""
}

// ToggleFollow toggles auto-scroll
func (p *Pane) ToggleFollow() {
	p.following = !p.following
}

// Clear clears all logs
func (p *Pane) Clear() {
	p.logLines = make([]string, 0)
	p.updateViewportContent()
}

// SetPodList populates the pane with a list of pods (for master/detail browsing)
func (p *Pane) SetPodList(pods []PodItem) {
	p.podList = pods
	p.listSelected = 0
}

// SetPodListWithContext sets pods and remembers their context/namespace
func (p *Pane) SetPodListWithContext(pods []PodItem, context, namespace string) {
	p.podList = pods
	p.filteredPodList = nil
	p.listSelected = 0
	p.context = context
	p.namespace = namespace
	p.showPodList = true
	p.lastRefresh = time.Now()
	// Reapply filter if searching
	if p.searching {
		p.applyFilter(p.searchInput.Value())
	}
}

// TogglePodList toggles displaying the pod list instead of logs
func (p *Pane) TogglePodList(show bool) {
	p.showPodList = show
}

// renderPodList renders the pod list UI
func (p *Pane) renderPodList(width int) string {
	visible := p.getVisibleList()
	if len(visible) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("No pods available in this namespace")
	}

	var items []string
	for i, pod := range visible {
		style := lipgloss.NewStyle().PaddingLeft(2)
		if i == p.listSelected {
			style = style.Foreground(lipgloss.Color("170")).Bold(true)
			items = append(items, style.Render("→ "+pod.Name))
		} else {
			style = style.Foreground(lipgloss.Color("250"))
			items = append(items, style.Render("  "+pod.Name))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(width - 2)

	return box.Render(content)
}

// updateViewportContent updates the viewport with current log lines
func (p *Pane) updateViewportContent() {
	content := strings.Join(p.logLines, "\n")
	p.viewport.SetContent(content)
}

// IsEmpty returns true if no pod is selected
func (p *Pane) IsEmpty() bool {
	return p.status == PaneEmpty
}

// IsActive returns true if actively streaming
func (p *Pane) IsActive() bool {
	return p.status == PaneStreaming
}

// ClearDummyData removes the dummy data
func (p *Pane) ClearDummyData() {
	p.logLines = make([]string, 0)
	p.podInfo = nil
	p.status = PaneEmpty
	p.updateViewportContent()
}

// Matches returns the current match count for the pane
func (p *Pane) Matches() int {
	if !p.showPodList {
		return 0
	}
	if p.searching {
		return p.matchCount
	}
	return len(p.podList)
}
