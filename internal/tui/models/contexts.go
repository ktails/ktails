package models

import (
	"fmt"
	"image/color"
	"io"
	"log"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

// contextList holds per-item display state for a single Kubernetes context.
type contextList struct {
	Name             string
	Cluster          string
	DefaultNamespace string
	Selected         bool
	IsCurrent        bool
	IsLoading        bool
	IsError          bool
	IsLoaded         bool
	// Color is this context's identity colour (styles.IdentityColor),
	// assigned by AppState.AddContext once the context is confirmed
	// (Enter), pushed down via SetContextColors. Nil until then — a
	// checked-but-unconfirmed row shows a neutral pending dot instead of
	// guessing at a colour that may not match once confirmed.
	Color color.Color
}

func (cl contextList) Title() string       { return cl.Name }
func (cl contextList) Description() string { return cl.DefaultNamespace }
func (cl contextList) FilterValue() string { return cl.Name }

// contextDelegate is a custom list.ItemDelegate that renders each context with
// icon-based state indicators and per-item colour coding.
type contextDelegate struct{}

func (d contextDelegate) Height() int                             { return 2 }
func (d contextDelegate) Spacing() int                            { return 0 }
func (d contextDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d contextDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ctx, ok := item.(contextList)
	if !ok {
		return
	}

	p := styles.CatppuccinMocha()
	isCursor := index == m.Index()

	paneWidth := m.Width()
	if paneWidth <= 0 {
		paneWidth = 30
	}

	// The identity dot is this context's stable per-session colour (§2.1),
	// set once it's confirmed (Enter). A checked-but-unconfirmed row shows
	// a neutral pending dot rather than guessing; an untouched row shows a
	// hollow gray dot.
	dot := "○"
	dotColor := p.Overlay1
	switch {
	case ctx.Selected && ctx.Color != nil:
		// Colour persists in AppState across a deselect (so re-selecting
		// keeps the same identity), but the dot itself must not — a
		// deselected row falls through to the hollow default below.
		dot = "●"
		dotColor = ctx.Color
	case ctx.Selected:
		dot = "◉"
		dotColor = p.Subtext0
	}

	// Status icon and colours — semantic, fixed (§2.2), and takes visual
	// priority over identity when it signals a problem (error outranks the
	// identity dot for attention; loading/loaded are quieter).
	var icon string
	var iconColor color.Color
	nameColor := p.Text

	switch {
	case ctx.IsLoading:
		icon = "⏳"
		iconColor = p.Subtext0
	case ctx.IsError:
		icon = "✗"
		iconColor = styles.StatusError
		nameColor = styles.StatusError
	case ctx.IsLoaded:
		icon = "✓"
		iconColor = styles.StatusHealthy
	default:
		icon = ""
		nameColor = p.Subtext0
	}

	currentMark := ""
	if ctx.IsCurrent {
		currentMark = " " + lipgloss.NewStyle().Foreground(p.Yellow).Render("★")
	}

	ns := ctx.DefaultNamespace
	if ns == "" {
		ns = "default"
	}
	cluster := ctx.Cluster
	if cluster == "" {
		cluster = "—"
	}

	statusSuffix := ""
	if icon != "" {
		statusSuffix = " " + icon
	}

	// Fixed-width rows must never let lipgloss wrap a long, unbroken context
	// name onto extra lines — truncate with an ellipsis instead, since the
	// pane column is a fixed and often narrow (~24-32 col) width.
	prefixW := 1 + lipgloss.Width(dot) + 1                                // " " + dot + " "
	suffixW := lipgloss.Width(currentMark) + lipgloss.Width(statusSuffix) // mark + " " + icon
	name := ansi.TruncateWc(ctx.Name, max(paneWidth-prefixW-suffixW, 0), "…")

	descFixed := 4 // "    " indent
	descText := ns + " · " + cluster
	descText = ansi.TruncateWc(descText, max(paneWidth-descFixed, 0), "…")

	dotStr := lipgloss.NewStyle().Foreground(dotColor).Render(dot)
	statusStr := lipgloss.NewStyle().Foreground(iconColor).Render(statusSuffix)
	nameStr := lipgloss.NewStyle().Foreground(nameColor).Bold(ctx.IsLoaded || ctx.Selected).Render(name)
	descStr := lipgloss.NewStyle().Foreground(p.Overlay1).Render(descText)

	titleContent := " " + dotStr + " " + nameStr + currentMark + statusStr
	descContent := "    " + descStr // indent to align under name

	if isCursor {
		// Focus accent bg + Base fg — the one selection/focus colour used
		// everywhere (sidebar cursor, table row, active tab, pane border).
		titleLine := lipgloss.NewStyle().
			Background(styles.FocusColor).
			Foreground(p.Base).
			Bold(true).
			Width(paneWidth).
			Render(" " + dot + " " + name + currentMark + statusSuffix)
		descLine := lipgloss.NewStyle().
			Background(styles.FocusColor).
			Foreground(p.Base).
			Width(paneWidth).
			Render("    " + descText)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	} else {
		titleLine := lipgloss.NewStyle().Width(paneWidth).Render(titleContent)
		descLine := lipgloss.NewStyle().Foreground(p.Overlay0).Width(paneWidth).Render(descContent)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	}
}

// ContextsInfo is the left-pane model for selecting Kubernetes contexts.
type ContextsInfo struct {
	Client    *k8s.Client
	Focused   bool
	PaneTitle string
	list      list.Model
	width     int
	height    int
	isLoading bool
	// Track what was previously confirmed/selected for diff calculation
	previouslySelected map[string]bool
}

func (c *ContextsInfo) setDimensions() {
	c.list.SetWidth(c.width)
	c.list.SetHeight(c.height)
}

func (c *ContextsInfo) GetDimensions() (w, h int) {
	return c.width, c.height
}

func NewContextInfo(client *k8s.Client) *ContextsInfo {
	newList := list.New([]list.Item{}, contextDelegate{}, 0, 0)
	newList.SetShowStatusBar(false)
	newList.SetShowHelp(false)
	// The pagination dot row reads as a stray glyph under the pane title;
	// the list still pages, it just doesn't draw the indicator.
	newList.SetShowPagination(false)
	return &ContextsInfo{
		Client:             client,
		PaneTitle:          "Kubernetes Contexts",
		list:               newList,
		isLoading:          true,
		previouslySelected: make(map[string]bool),
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	c.initContextPane()
	return nil
}

func (c *ContextsInfo) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.setDimensions()
		return nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			c.list, cmd = c.list.Update(msg)
			return cmd
		case "down", "j":
			c.list, cmd = c.list.Update(msg)
			return cmd
		case "space":
			c.toggleSelection()
			return nil
		case "enter":
			return c.confirmSelection()
		default:
			c.list, cmd = c.list.Update(msg)
			return cmd
		}
	}
	return nil
}

func (c *ContextsInfo) toggleSelection() {
	idx := c.list.Index()
	if idx < 0 {
		return
	}
	items := c.list.Items()
	if idx >= len(items) {
		return
	}
	if item, ok := items[idx].(contextList); ok {
		item.Selected = !item.Selected
		items[idx] = item
	}
	c.list.SetItems(items)
	c.list.Select(idx)
}

// SetSelected sets every named context's checked (pending) state, mirroring
// what pressing Space on each of those rows individually would do — used by
// the Clusters pane to bulk-toggle every context under one cluster without
// duplicating the Contexts pane's own selection bookkeeping.
func (c *ContextsInfo) SetSelected(names []string, selected bool) {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	items := c.list.Items()
	updated := false
	for idx, item := range items {
		ctx, ok := item.(contextList)
		if !ok || !nameSet[ctx.Name] {
			continue
		}
		if ctx.Selected != selected {
			ctx.Selected = selected
			items[idx] = ctx
			updated = true
		}
	}
	if updated {
		c.list.SetItems(items)
	}
}

// contextsSnapshot returns every known context's current display state, for
// panes (like Clusters) that need to group or read fields ContextsInfo
// already tracks without a second data fetch.
func (c *ContextsInfo) contextsSnapshot() []contextList {
	items := c.list.Items()
	out := make([]contextList, 0, len(items))
	for _, item := range items {
		if ctx, ok := item.(contextList); ok {
			out = append(out, ctx)
		}
	}
	return out
}

// anySelected reports whether any context is currently checked (toggled
// with Space), regardless of whether it's newly checked or already
// confirmed from a previous Enter.
func (c *ContextsInfo) anySelected() bool {
	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			return true
		}
	}
	return false
}

// SetContextStates updates loading, error, and loaded state for each context in the list.
func (c *ContextsInfo) SetContextStates(loading map[string]bool, errors map[string]string, loaded map[string]bool) {
	items := c.list.Items()
	updated := false

	for idx, item := range items {
		ctx, ok := item.(contextList)
		if !ok {
			continue
		}

		newLoading := loading[ctx.Name]
		newError := errors[ctx.Name] != ""
		newLoaded := loaded[ctx.Name]

		if ctx.IsLoading == newLoading && ctx.IsError == newError && ctx.IsLoaded == newLoaded {
			continue
		}

		ctx.IsLoading = newLoading
		ctx.IsError = newError
		ctx.IsLoaded = newLoaded
		items[idx] = ctx
		updated = true
	}

	if updated {
		c.list.SetItems(items)
	}
}

// SetContextColors pushes each confirmed context's identity colour
// (state.AppState.Snapshot().ContextColors) down into the list so the
// sidebar dot and the resource table's Context swatch agree on colour.
func (c *ContextsInfo) SetContextColors(colors map[string]color.Color) {
	items := c.list.Items()
	updated := false

	for idx, item := range items {
		ctx, ok := item.(contextList)
		if !ok {
			continue
		}
		newColor := colors[ctx.Name]
		if ctx.Color == newColor {
			continue
		}
		ctx.Color = newColor
		items[idx] = ctx
		updated = true
	}

	if updated {
		c.list.SetItems(items)
	}
}

// getAllContextStates diffs the current checked contexts against
// previouslySelected (as of the last confirm) and returns exactly what
// changed: newly checked contexts as Added, no-longer-checked ones as
// Deselected. This is the one place that diff happens — callers never need
// their own "is this genuinely new" bookkeeping.
func (c *ContextsInfo) getAllContextStates() msgs.ContextsStateMsg {
	currentSelected := make(map[string]bool)
	var added []msgs.ContextsSelectedMsg

	for _, item := range c.list.Items() {
		ctx, ok := item.(contextList)
		if !ok || !ctx.Selected {
			continue
		}
		currentSelected[ctx.Name] = true
		if !c.previouslySelected[ctx.Name] {
			added = append(added, msgs.ContextsSelectedMsg{
				ContextName:      ctx.Name,
				DefaultNamespace: ctx.DefaultNamespace,
			})
		}
	}

	var deselected []string
	for prevContext := range c.previouslySelected {
		if !currentSelected[prevContext] {
			deselected = append(deselected, prevContext)
		}
	}

	c.previouslySelected = currentSelected
	return msgs.ContextsStateMsg{Added: added, Deselected: deselected}
}

func (c *ContextsInfo) confirmSelection() tea.Cmd {
	if !c.anySelected() {
		// Nothing was toggled with Space — treat the row under the cursor as
		// a quick-select shortcut so Enter alone can confirm a single
		// context.
		c.toggleSelection()
	}
	return c.ConfirmChanges()
}

// ConfirmChanges diffs whatever is currently checked — however it got
// checked, directly via Space or in bulk via the Clusters pane's
// SetSelected — against the last confirm, and returns the resulting
// ContextsStateMsg command, or nil if nothing changed. Unlike
// confirmSelection (the Contexts pane's own Enter), this never falls back
// to auto-checking a row: the Clusters pane has no "row under cursor" that
// maps to a single context.
func (c *ContextsInfo) ConfirmChanges() tea.Cmd {
	state := c.getAllContextStates()
	if len(state.Added) == 0 && len(state.Deselected) == 0 {
		return nil
	}
	return func() tea.Msg { return state }
}

// View renders just the context list — the pane's "Contexts" title lives in
// the surrounding box border (see views.TitledBox), not in the content.
func (c *ContextsInfo) View() string {
	if c.isLoading {
		return ""
	}
	return c.list.View()
}

func (c *ContextsInfo) initContextPane() {
	rawContextsList, err := c.Client.ListContexts()
	if err != nil {
		log.Printf("unable to fetch contexts from client: %v", err)
	}

	currentCtx := c.Client.GetCurrentContext()
	itemList := make([]list.Item, 0, len(rawContextsList))

	for _, ctxInfo := range rawContextsList {
		itemList = append(itemList, contextList{
			Name:             ctxInfo.Name,
			Cluster:          ctxInfo.Cluster,
			DefaultNamespace: ctxInfo.DefaultNamespace,
			Selected:         false,
			IsCurrent:        ctxInfo.Name == currentCtx,
		})
	}

	c.list.SetItems(itemList)
	c.list.Title = "" // title rendered manually in View()
	c.isLoading = false
}

func (c *ContextsInfo) HelpView() string {
	return c.list.Help.FullHelpView(c.list.FullHelp())
}

func (c *ContextsInfo) SetFocused(f bool) {
	c.Focused = f
}
