// Package models
package models

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/tui/styles"
)

// maxLogLines bounds the in-memory scrollback for a live-tailing log
// stream, dropping the oldest lines once exceeded.
const maxLogLines = 5000

// LogPage renders a live-tailing log viewport for a single pod container, in
// the shared bottom Log pane. It holds only render state — opening/reading
// the underlying stream is orchestrated by cmds + MainPage, mirroring how
// ResourceDetailPage stays free of k8s/io concerns.
type LogPage struct {
	viewport viewport.Model

	podName      string
	namespace    string
	context      string
	containers   []string
	containerIdx int

	lines     []string
	streaming bool
	streamErr string

	focused bool
}

func NewLogPage() *LogPage {
	return &LogPage{viewport: viewport.New(0, 0)}
}

func (l *LogPage) Init() tea.Cmd {
	return nil
}

// Reset (re)starts the pane for a new pod, discarding any previous
// scrollback, and points it at the pod's first container.
func (l *LogPage) Reset(podName, namespace, context string, containers []string) {
	l.podName = podName
	l.namespace = namespace
	l.context = context
	l.containers = containers
	l.containerIdx = 0
	l.lines = nil
	l.streaming = true
	l.streamErr = ""
	l.viewport.SetContent(fmt.Sprintf("Connecting to %s...", podName))
	l.viewport.GotoTop()
}

// HasContent reports whether a pod has ever been loaded into this page.
func (l *LogPage) HasContent() bool {
	return l.podName != ""
}

// Matches reports whether the pane is already pinned to the given pod, so
// callers can refocus it instead of restreaming.
func (l *LogPage) Matches(podName, namespace, context string) bool {
	return l.HasContent() && l.podName == podName && l.namespace == namespace && l.context == context
}

// Containers returns the current pod's container names.
func (l *LogPage) Containers() []string {
	return l.containers
}

// PodName, Namespace, and Context identify the pod currently pinned to this
// pane, so callers can restream (e.g. after CycleContainer) without having
// to separately track that identity themselves.
func (l *LogPage) PodName() string   { return l.podName }
func (l *LogPage) Namespace() string { return l.namespace }
func (l *LogPage) Context() string   { return l.context }

// CurrentContainer returns the container currently being streamed, or "" if
// none is loaded.
func (l *LogPage) CurrentContainer() string {
	if l.containerIdx < 0 || l.containerIdx >= len(l.containers) {
		return ""
	}
	return l.containers[l.containerIdx]
}

// CycleContainer advances to the next container (wrapping around) and
// resets the pane's scrollback for it. It's a no-op — ok == false — when
// the pod has zero or one containers, since there's nothing to cycle to.
func (l *LogPage) CycleContainer() (container string, ok bool) {
	if len(l.containers) <= 1 {
		return "", false
	}
	l.containerIdx = (l.containerIdx + 1) % len(l.containers)
	name := l.containers[l.containerIdx]

	l.lines = nil
	l.streaming = true
	l.streamErr = ""
	l.viewport.SetContent(fmt.Sprintf("Connecting to %s (%s)...", l.podName, name))
	l.viewport.GotoTop()

	return name, true
}

// AppendLine adds a line read from the live stream. The viewport only
// auto-follows to the bottom if it was already there — a user who scrolled
// up to read earlier output isn't yanked back down by new lines arriving.
func (l *LogPage) AppendLine(line string) {
	wasAtBottom := l.viewport.AtBottom()

	l.lines = append(l.lines, line)
	if len(l.lines) > maxLogLines {
		l.lines = l.lines[len(l.lines)-maxLogLines:]
	}
	l.viewport.SetContent(strings.Join(l.lines, "\n"))

	if wasAtBottom {
		l.viewport.GotoBottom()
	}
}

// SetStreamEnded records that the stream stopped (server-closed or errored)
// and appends an inline banner line, preserving existing scrollback rather
// than replacing the pane with a full-screen error.
func (l *LogPage) SetStreamEnded(err error) {
	wasAtBottom := l.viewport.AtBottom()

	l.streaming = false
	if err != nil {
		l.streamErr = err.Error()
	} else {
		l.streamErr = "stream ended"
	}

	p := styles.CatppuccinMocha()
	banner := lipgloss.NewStyle().Foreground(p.Red).Render(fmt.Sprintf("⚠ log stream ended: %s", l.streamErr))
	l.lines = append(l.lines, banner)
	l.viewport.SetContent(strings.Join(l.lines, "\n"))

	if wasAtBottom {
		l.viewport.GotoBottom()
	}
}

// Header renders a one-line banner identifying the pod/container being
// tailed and the pane's key hints. width caps the line so it never becomes
// the widest line in the pane at narrow terminal sizes.
func (l *LogPage) Header(width int) string {
	p := styles.CatppuccinMocha()
	title := lipgloss.NewStyle().Foreground(p.Peach).Bold(true)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true)

	label := "Logs"
	if l.podName != "" {
		label = fmt.Sprintf("Pod: %s", l.podName)
	}

	containerInfo := ""
	if len(l.containers) > 0 {
		containerInfo = fmt.Sprintf("  [%s %d/%d]", l.CurrentContainer(), l.containerIdx+1, len(l.containers))
	}

	full := title.Render(fmt.Sprintf("▾ %s%s", label, containerInfo)) + "  " +
		hint.Render(fmt.Sprintf("(%s — c: cycle container, ↑/↓ pgup/pgdn scroll, End: jump+follow, Esc back)", l.context))
	if width <= 0 {
		return full
	}
	return ansi.Truncate(full, width, "…")
}

func (l *LogPage) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "home", "g":
			l.viewport.GotoTop()
			return nil
		case "end", "G":
			l.viewport.GotoBottom()
			return nil
		}
	}

	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return cmd
}

func (l *LogPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	l.viewport.Width = w
	l.viewport.Height = h
}

func (l *LogPage) SetFocused(f bool) {
	l.focused = f
}

func (l *LogPage) View() string {
	p := styles.CatppuccinMocha()
	if !l.HasContent() {
		return lipgloss.NewStyle().Foreground(p.Overlay1).Render("No logs loaded")
	}
	return l.viewport.View()
}
