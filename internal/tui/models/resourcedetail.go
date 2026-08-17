// Package models
package models

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/styles"
)

// ResourceDetailPage renders a scrollable, tabbed Status / Conditions /
// Events / YAML view for a single Kubernetes resource (Deployment, Pod,
// ...), shown in the shared bottom Detail tab regardless of which top tab it
// was opened from. The Conditions tab only appears for kinds that actually
// populate detail.Conditions (see k8s.ResourceDetail) — most ConfigMaps/
// Secrets/Ingresses/CronJobs never do.
type ResourceDetailPage struct {
	viewport viewport.Model

	loaded  bool
	loading bool
	errMsg  string

	kind    string
	name    string
	context string
	focused bool

	// detail is the last successfully loaded resource, kept around (rather
	// than only its rendered form) so switching tabs can re-derive that
	// tab's content without re-fetching, and so tabLabels() can decide
	// whether Conditions applies.
	detail k8s.ResourceDetail
	// activeTab indexes into tabLabels() — Status is always first, YAML
	// always last; Conditions/Events sit between depending on tabLabels().
	activeTab int

	// rawContent is the active tab's full, un-sliced rendered text — the
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
		viewport: viewport.New(),
	}
}

func (d *ResourceDetailPage) Init() tea.Cmd {
	return nil
}

// StartLoading marks a fetch as in-flight for the given resource, replacing
// any previously rendered content. The horizontal scroll offset and active
// tab are kept when re-loading the same resource (e.g. re-pressing Enter on
// the still-selected row) and reset when a different resource is opened.
func (d *ResourceDetailPage) StartLoading(kind, name, context string) {
	if !d.Matches(kind, name, context) {
		d.hOffset = 0
		d.activeTab = 0
		d.detail = k8s.ResourceDetail{}
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

// SetDetail records the fetched detail and renders the active tab into the
// scrollable viewport, preserving whatever horizontal scroll offset
// StartLoading left in place (clamped to the new content's width).
func (d *ResourceDetailPage) SetDetail(detail k8s.ResourceDetail) {
	d.loading = false
	d.loaded = true
	d.errMsg = ""
	d.detail = detail
	d.clampActiveTab()
	d.applyActiveTab()
	d.viewport.GotoTop()
}

// tabLabels returns this resource's visible tab set in display order.
// Conditions is included only when the resource actually has any — kinds
// without a `.status.conditions` field (ConfigMaps, Secrets, Ingresses,
// CronJobs, ...) never populate k8s.ResourceDetail.Conditions, so the tab
// stays hidden rather than showing an always-empty table.
func (d *ResourceDetailPage) tabLabels() []string {
	labels := []string{"Status"}
	if len(d.detail.Conditions) > 0 {
		labels = append(labels, "Conditions")
	}
	return append(labels, "Events", "YAML")
}

// clampActiveTab keeps activeTab within tabLabels()'s current bounds — the
// tab set can shrink by one (Conditions) between StartLoading and SetDetail
// clearing/repopulating d.detail.
func (d *ResourceDetailPage) clampActiveTab() {
	if n := len(d.tabLabels()); d.activeTab >= n {
		d.activeTab = n - 1
	}
	if d.activeTab < 0 {
		d.activeTab = 0
	}
}

// yamlTabIndex locates the YAML tab's current index — always last, but
// computed rather than hardcoded so it stays correct regardless of whether
// Conditions is present.
func (d *ResourceDetailPage) yamlTabIndex() int {
	return len(d.tabLabels()) - 1
}

// switchTab moves to tab idx, re-rendering its content and resetting scroll
// position — tabs are independent screens, not scroll-linked, so there's no
// per-tab position worth preserving across a switch.
func (d *ResourceDetailPage) switchTab(idx int) {
	if !d.loaded || idx == d.activeTab {
		return
	}
	d.activeTab = idx
	d.hOffset = 0
	d.applyActiveTab()
	d.viewport.GotoTop()
}

// cycleTab moves delta tabs forward/back, wrapping — bound to "["/"]" in
// Update, mirroring the same keys' meaning for NamespacesInfo's left-pane
// sections and MainPage's own resource-kind tabs ("cycle whichever is
// focused").
func (d *ResourceDetailPage) cycleTab(delta int) {
	if !d.loaded {
		return
	}
	n := len(d.tabLabels())
	d.switchTab((d.activeTab + delta + n) % n)
}

// applyActiveTab re-renders the active tab's content into rawContent/
// rawLineWidth and re-applies the current horizontal scroll offset.
func (d *ResourceDetailPage) applyActiveTab() {
	d.rawContent = d.renderActiveTab()
	d.rawLineWidth = maxLineWidth(d.rawContent)
	d.clampHOffset()
	d.applyHOffset()
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
	maxOffset := d.rawLineWidth - d.viewport.Width()
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
// sequences (Status/Conditions/Events coloring) survive the horizontal crop
// intact.
func (d *ResourceDetailPage) applyHOffset() {
	if d.hOffset == 0 {
		d.viewport.SetContent(d.rawContent)
		return
	}
	lines := strings.Split(d.rawContent, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, d.hOffset, d.hOffset+d.viewport.Width())
	}
	d.viewport.SetContent(strings.Join(lines, "\n"))
}

// HScrollStatus reports the current horizontal scroll position as a
// percentage, for the status bar's "◂ 40% ▸" indicator. ok is false when the
// indicator should be hidden — no overflow to scroll, or nothing loaded.
func (d *ResourceDetailPage) HScrollStatus() (percent int, ok bool) {
	maxOffset := d.rawLineWidth - d.viewport.Width()
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

// Context returns the loaded resource's context name, so a caller can look
// up its identity colour (state.AppState.Snapshot().ContextColors) to pass
// into Header.
func (d *ResourceDetailPage) Context() string {
	return d.context
}

// Header renders a one-line banner identifying the loaded resource, its tab
// strip, and the pane's own key hints, meant to sit above the scrollable
// viewport so the pane reads as a distinct region rather than a peer tab.
// width caps the line so it never becomes the widest line in the pane at
// narrow terminal sizes — an unbounded line here forced the whole block to
// wrap. dotColor is the resource's context's identity colour (nil if not yet
// assigned) — shown here because this pane can replace the list entirely
// (overlay mode, or a narrow terminal), so its own header is the only place
// the Context column's identity swatch is still visible while it's open.
func (d *ResourceDetailPage) Header(width int, dotColor color.Color) string {
	p := styles.CatppuccinMocha()
	titleColor := styles.BlurColor
	if d.focused {
		titleColor = styles.FocusColor
	}
	title := lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true)
	if dotColor == nil {
		dotColor = p.Overlay1
	}
	dot := lipgloss.NewStyle().Foreground(dotColor).Render("●")

	label := fmt.Sprintf("%s: %s", d.kind, d.name)
	if label == ": " {
		label = "Detail"
	}
	tabStrip := RenderTabStrip(d.tabLabels(), d.activeTab, d.focused)
	full := title.Render(fmt.Sprintf("▾ %s", label)) + "  " + dot + " " + hint.Render(d.context) +
		"  " + tabStrip + "  " +
		hint.Render("([/]: tab  y: yaml  ↑/↓ pgup/pgdn scroll  Home/End jump  Esc back  Ctrl+R return)")
	if width <= 0 {
		return full
	}
	return ansi.Truncate(full, width, "…")
}

func (d *ResourceDetailPage) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "home", "g":
			d.viewport.GotoTop()
			return nil
		case "end", "G":
			d.viewport.GotoBottom()
			return nil
		case "y":
			d.switchTab(d.yamlTabIndex())
			return nil
		case "]":
			d.cycleTab(1)
			return nil
		case "[":
			d.cycleTab(-1)
			return nil
		case "shift+left":
			if !d.loaded {
				return nil
			}
			d.hOffset -= halfViewportStep(d.viewport.Width())
			d.clampHOffset()
			d.applyHOffset()
			return nil
		case "shift+right":
			if !d.loaded {
				return nil
			}
			d.hOffset += halfViewportStep(d.viewport.Width())
			d.clampHOffset()
			d.applyHOffset()
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
	if w != d.lastWidth {
		d.hOffset = 0
		d.lastWidth = w
	}
	d.viewport.SetWidth(w)
	d.viewport.SetHeight(h)
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

// renderActiveTab dispatches to the active tab's own renderer by label —
// tabLabels()'s order is the display order, but which index a given tab
// lands on shifts depending on whether Conditions is present, so dispatch is
// by name rather than a hardcoded index.
func (d *ResourceDetailPage) renderActiveTab() string {
	labels := d.tabLabels()
	if d.activeTab < 0 || d.activeTab >= len(labels) {
		return ""
	}
	switch labels[d.activeTab] {
	case "Status":
		return d.renderStatus()
	case "Conditions":
		return d.renderConditions()
	case "Events":
		return d.renderEvents()
	case "YAML":
		return d.renderYAML()
	}
	return ""
}

// sectionStyles are shared across every tab renderer.
func sectionStyles() (title, label, sep lipgloss.Style) {
	p := styles.CatppuccinMocha()
	title = lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	label = lipgloss.NewStyle().Foreground(p.Subtext0)
	sep = lipgloss.NewStyle().Foreground(p.Overlay0)
	return
}

func sectionRule() string {
	_, _, sep := sectionStyles()
	return sep.Render(strings.Repeat("─", 60))
}

// renderStatus renders the resource's identity block (kind/name/context/
// namespace/age/summary) followed by its Status conditions — the identity
// block lives only here, not repeated on every tab, since Status is the
// default/most-common tab and Header() already carries the kind/name/
// context identity for the other tabs.
func (d *ResourceDetailPage) renderStatus() string {
	titleStyle, labelStyle, _ := sectionStyles()

	var b strings.Builder
	fmt.Fprintln(&b, titleStyle.Render(fmt.Sprintf("%s: %s", d.detail.Kind, d.detail.Name)))
	fmt.Fprintf(&b, "%s %s   %s %s   %s %s\n",
		labelStyle.Render("Context:"), d.context,
		labelStyle.Render("Namespace:"), d.detail.Namespace,
		labelStyle.Render("Age:"), d.detail.Age,
	)
	fmt.Fprintln(&b, d.detail.Summary)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, titleStyle.Render("Status"))
	fmt.Fprintln(&b, sectionRule())
	if len(d.detail.Status) == 0 {
		fmt.Fprintln(&b, "—")
	}
	for _, s := range d.detail.Status {
		fmt.Fprintln(&b, s)
	}
	return b.String()
}

// renderConditions renders detail.Conditions as a TYPE/STATUS/REASON/
// MESSAGE/AGE table, coloring Status by a simple kind-agnostic rule: "True"
// is healthy (green), "False" is a problem (red), "Unknown" stays dim —
// there's no single semantic table of which condition types invert that
// rule (e.g. Deployment's Progressing vs Available), so this doesn't try to
// guess beyond the literal status value.
func (d *ResourceDetailPage) renderConditions() string {
	titleStyle, _, _ := sectionStyles()
	p := styles.CatppuccinMocha()
	headerStyle := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true)

	var b strings.Builder
	fmt.Fprintln(&b, titleStyle.Render("Conditions"))
	fmt.Fprintln(&b, sectionRule())
	if len(d.detail.Conditions) == 0 {
		fmt.Fprintln(&b, "—")
		return b.String()
	}
	fmt.Fprintln(&b, headerStyle.Render(fmt.Sprintf("%-24s %-8s %-20s %-40s %s", "TYPE", "STATUS", "REASON", "MESSAGE", "AGE")))
	for _, c := range d.detail.Conditions {
		statusStyle := lipgloss.NewStyle().Foreground(p.Overlay1)
		switch c.Status {
		case "True":
			statusStyle = lipgloss.NewStyle().Foreground(p.Green)
		case "False":
			statusStyle = lipgloss.NewStyle().Foreground(p.Red)
		}
		fmt.Fprintf(&b, "%-24s %-8s %-20s %-40s %s\n",
			c.Type, statusStyle.Render(c.Status), c.Reason, c.Message, c.Age)
	}
	return b.String()
}

// renderEvents renders detail.Events, or the EventsError/"no events"
// fallback — unchanged from the pre-tabs render() output beyond no longer
// being concatenated with Status/YAML.
func (d *ResourceDetailPage) renderEvents() string {
	titleStyle, _, _ := sectionStyles()
	p := styles.CatppuccinMocha()

	var b strings.Builder
	fmt.Fprintln(&b, titleStyle.Render("Events"))
	fmt.Fprintln(&b, sectionRule())
	switch {
	case d.detail.EventsError != "":
		// A fetch failure is not the same thing as the resource genuinely
		// having no events — say so, dim, rather than the two looking
		// identical (see k8s.ResourceDetail.EventsError's doc comment).
		fmt.Fprintln(&b, lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("events unavailable: "+d.detail.EventsError))
	case len(d.detail.Events) == 0:
		fmt.Fprintln(&b, "No events")
	}
	for _, e := range d.detail.Events {
		typeStyle := lipgloss.NewStyle().Foreground(p.Green)
		if e.Type == "Warning" {
			typeStyle = lipgloss.NewStyle().Foreground(p.Yellow)
		}
		fmt.Fprintf(&b, "%s  %-16s  %-6s  %s (x%d)\n",
			e.Age, e.Reason, typeStyle.Render(e.Type), e.Message, e.Count)
	}
	return b.String()
}

// renderYAML renders the resource's full rendered YAML, syntax-highlighted.
func (d *ResourceDetailPage) renderYAML() string {
	titleStyle, _, _ := sectionStyles()
	var b strings.Builder
	fmt.Fprintln(&b, titleStyle.Render("YAML"))
	fmt.Fprintln(&b, sectionRule())
	fmt.Fprint(&b, highlightYAML(d.detail.YAML))
	return b.String()
}
