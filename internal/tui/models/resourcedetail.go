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

	// rawContent is the full, un-sliced rendered detail text — the
	// source-of-truth for horizontal scrolling. Each render of the
	// viewport's content re-slices rawContent's lines at hOffset rather
	// than mutating it, so scrolling back left never loses content.
	rawContent   string
	rawLineWidth int // widest line in rawContent, in cells
	hOffset      int // horizontal scroll offset, in cells
	lastWidth    int // viewport width as of the last SetSize, to detect resize
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
// any previously rendered content. The horizontal scroll offset is kept when
// re-loading the same resource (e.g. re-pressing Enter on the still-selected
// row) and reset when a different resource is opened.
func (d *ResourceDetailPage) StartLoading(kind, name, context string) {
	if !d.Matches(kind, name, context) {
		d.hOffset = 0
	}
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

// SetDetail renders the fetched detail into the scrollable viewport,
// preserving whatever horizontal scroll offset StartLoading left in place
// (clamped to the new content's width).
func (d *ResourceDetailPage) SetDetail(detail k8s.ResourceDetail) {
	d.loading = false
	d.loaded = true
	d.errMsg = ""
	d.rawContent = d.render(detail)
	d.rawLineWidth = maxLineWidth(d.rawContent)
	d.clampHOffset()
	d.applyHOffset()
	d.viewport.GotoTop()
}

// maxLineWidth returns the widest line in s, in display cells, ANSI escapes
// excluded.
func maxLineWidth(s string) int {
	widest := 0
	for _, line := range strings.Split(s, "\n") {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	return widest
}

// clampHOffset keeps hOffset within [0, rawLineWidth-viewport.Width], so a
// narrower resource/terminal never leaves the view stuck past the content.
func (d *ResourceDetailPage) clampHOffset() {
	maxOffset := d.rawLineWidth - d.viewport.Width
	if maxOffset < 0 {
		maxOffset = 0
	}
	if d.hOffset > maxOffset {
		d.hOffset = maxOffset
	}
	if d.hOffset < 0 {
		d.hOffset = 0
	}
}

// applyHOffset re-slices every line of rawContent at the current hOffset and
// pushes the result into the viewport. ansi.Cut is ANSI-aware, so escape
// sequences (Status/Events coloring) survive the horizontal crop intact.
func (d *ResourceDetailPage) applyHOffset() {
	if d.hOffset == 0 {
		d.viewport.SetContent(d.rawContent)
		return
	}
	lines := strings.Split(d.rawContent, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, d.hOffset, d.hOffset+d.viewport.Width)
	}
	d.viewport.SetContent(strings.Join(lines, "\n"))
}

// HScrollStatus reports the current horizontal scroll position as a
// percentage, for the status bar's "◂ 40% ▸" indicator. ok is false when the
// indicator should be hidden — no overflow to scroll, or nothing loaded.
func (d *ResourceDetailPage) HScrollStatus() (percent int, ok bool) {
	maxOffset := d.rawLineWidth - d.viewport.Width
	if !d.loaded || maxOffset <= 0 {
		return 0, false
	}
	return d.hOffset * 100 / maxOffset, true
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
		case "shift+left":
			if !d.loaded {
				return nil
			}
			d.hOffset -= halfViewportStep(d.viewport.Width)
			d.clampHOffset()
			d.applyHOffset()
			return nil
		case "shift+right":
			if !d.loaded {
				return nil
			}
			d.hOffset += halfViewportStep(d.viewport.Width)
			d.clampHOffset()
			d.applyHOffset()
			return nil
		}
	}

	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return cmd
}

// halfViewportStep is the Shift+Left/Right scroll distance: half the
// viewport's width, so a couple of presses reach far-right content
// regardless of terminal size. Minimum 1 to stay useful at very narrow
// widths.
func halfViewportStep(viewportWidth int) int {
	step := viewportWidth / 2
	if step < 1 {
		step = 1
	}
	return step
}

func (d *ResourceDetailPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	if w != d.lastWidth {
		d.hOffset = 0
		d.lastWidth = w
	}
	d.viewport.Width = w
	d.viewport.Height = h
	// Loading/error placeholders bypass rawContent (see StartLoading/
	// SetError) — only re-slice once real content is loaded, so this
	// doesn't clobber a placeholder just set by StartLoading.
	if d.loaded {
		d.clampHOffset()
		d.applyHOffset()
	}
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
