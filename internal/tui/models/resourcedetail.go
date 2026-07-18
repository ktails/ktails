// Package models
package models

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

// ResourceDetailPage renders a scrollable Status / Events / YAML view for a
// single Kubernetes resource (Deployment, Pod, ...), shown in the shared
// bottom Detail tab regardless of which top tab it was opened from.
type ResourceDetailPage struct {
	viewport viewport.Model

	loaded  bool
	loading bool
	errMsg  string

	kind    string
	name    string
	context string
	focused bool
}

func NewResourceDetailPage() *ResourceDetailPage {
	return &ResourceDetailPage{
		viewport: viewport.New(0, 0),
	}
}

func (d *ResourceDetailPage) Init() tea.Cmd {
	return nil
}

// StartLoading marks a fetch as in-flight for the given resource, replacing
// any previously rendered content.
func (d *ResourceDetailPage) StartLoading(kind, name, context string) {
	d.loading = true
	d.loaded = false
	d.errMsg = ""
	d.kind = kind
	d.name = name
	d.context = context
	d.viewport.SetContent(fmt.Sprintf("Loading detail for %s %s...", kind, name))
	d.viewport.GotoTop()
}

// SetError records a failed fetch.
func (d *ResourceDetailPage) SetError(err string) {
	d.loading = false
	d.loaded = false
	d.errMsg = err
}

// SetDetail renders the fetched detail into the scrollable viewport.
func (d *ResourceDetailPage) SetDetail(detail k8s.ResourceDetail) {
	d.loading = false
	d.loaded = true
	d.errMsg = ""
	d.viewport.SetContent(d.render(detail))
	d.viewport.GotoTop()
}

// HasContent reports whether a resource has ever been loaded into this page.
func (d *ResourceDetailPage) HasContent() bool {
	return d.loaded || d.loading || d.errMsg != ""
}

// Matches reports whether the pane is already showing (or loading) the given
// resource, so callers can refocus it instead of re-fetching.
func (d *ResourceDetailPage) Matches(kind, name, context string) bool {
	return d.HasContent() && d.kind == kind && d.name == name && d.context == context
}

// Header renders a one-line banner identifying the loaded resource and the
// pane's own close hint, meant to sit above the scrollable viewport so the
// pane reads as a distinct region rather than a peer tab. width caps the
// line so it never becomes the widest line in the pane at narrow terminal
// sizes — an unbounded line here forced the whole block to wrap.
func (d *ResourceDetailPage) Header(width int) string {
	p := styles.CatppuccinMocha()
	title := lipgloss.NewStyle().Foreground(p.Peach).Bold(true)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true)

	label := fmt.Sprintf("%s: %s", d.kind, d.name)
	if label == ": " {
		label = "Detail"
	}
	full := title.Render(fmt.Sprintf("▾ %s", label)) + "  " +
		hint.Render(fmt.Sprintf("(%s — ↑/↓ pgup/pgdn scroll, Home/End jump, Esc back, Ctrl+R return)", d.context))
	if width <= 0 {
		return full
	}
	return ansi.Truncate(full, width, "…")
}

func (d *ResourceDetailPage) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "home", "g":
			d.viewport.GotoTop()
			return nil
		case "end", "G":
			d.viewport.GotoBottom()
			return nil
		}
	}

	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return cmd
}

func (d *ResourceDetailPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	d.viewport.Width = w
	d.viewport.Height = h
}

func (d *ResourceDetailPage) SetFocused(f bool) {
	d.focused = f
}

func (d *ResourceDetailPage) View() string {
	p := styles.CatppuccinMocha()

	if d.loading {
		return lipgloss.NewStyle().Foreground(p.Blue).Render(fmt.Sprintf("Loading detail for %s %s...", d.kind, d.name))
	}
	if d.errMsg != "" {
		return lipgloss.NewStyle().Foreground(p.Red).Render(fmt.Sprintf("⚠ %s", d.errMsg))
	}

	return d.viewport.View()
}

func (d *ResourceDetailPage) render(detail k8s.ResourceDetail) string {
	p := styles.CatppuccinMocha()
	titleStyle := lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(p.Subtext0)
	sepStyle := lipgloss.NewStyle().Foreground(p.Overlay0)
	sep := sepStyle.Render(strings.Repeat("─", 60))

	var b strings.Builder

	fmt.Fprintln(&b, titleStyle.Render(fmt.Sprintf("%s: %s", detail.Kind, detail.Name)))
	fmt.Fprintf(&b, "%s %s   %s %s   %s %s\n",
		labelStyle.Render("Context:"), d.context,
		labelStyle.Render("Namespace:"), detail.Namespace,
		labelStyle.Render("Age:"), detail.Age,
	)
	fmt.Fprintln(&b, detail.Summary)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, titleStyle.Render("Status"))
	fmt.Fprintln(&b, sep)
	if len(detail.Status) == 0 {
		fmt.Fprintln(&b, "—")
	}
	for _, s := range detail.Status {
		fmt.Fprintln(&b, s)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, titleStyle.Render("Events"))
	fmt.Fprintln(&b, sep)
	if len(detail.Events) == 0 {
		fmt.Fprintln(&b, "No events")
	}
	for _, e := range detail.Events {
		typeStyle := lipgloss.NewStyle().Foreground(p.Green)
		if e.Type == "Warning" {
			typeStyle = lipgloss.NewStyle().Foreground(p.Yellow)
		}
		fmt.Fprintf(&b, "%s  %-16s  %-6s  %s (x%d)\n",
			e.Age, e.Reason, typeStyle.Render(e.Type), e.Message, e.Count)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, titleStyle.Render("YAML"))
	fmt.Fprintln(&b, sep)
	fmt.Fprint(&b, detail.YAML)

	return b.String()
}
