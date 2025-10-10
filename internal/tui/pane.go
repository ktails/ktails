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
}

// EndSearch disables search mode and clears filter
func (p *Pane) EndSearch() {
	p.searching = false
	p.searchInput.Blur()
	p.filterTerm = ""
	p.matchCount = 0
}

// Update handles messages for this pane
func (p *Pane) Update(msg tea.Msg) (*Pane, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
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

	body = p.viewport.View()

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

// SetPodListWithContext sets pods and remembers their context/namespace
func (p *Pane) SetPodListWithContext(pods []PodItem, context, namespace string) {
	p.context = context
	p.namespace = namespace
	p.lastRefresh = time.Now()
	// Reapply filter if searching
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
	if p.searching {
		return p.matchCount
	}
	return 0
}
