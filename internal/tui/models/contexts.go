package models

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// State icon and colours
	var icon string
	var iconColor, nameColor lipgloss.Color

	switch {
	case ctx.IsLoading:
		icon = "⏳"
		iconColor = p.Blue
		nameColor = p.Blue
	case ctx.IsError:
		icon = "✗"
		iconColor = p.Red
		nameColor = p.Maroon
	case ctx.IsLoaded:
		icon = "✓"
		iconColor = p.Green
		nameColor = p.Text
	case ctx.Selected:
		icon = "◉"
		iconColor = p.Mauve
		nameColor = p.Lavender
	default:
		icon = "○"
		iconColor = p.Overlay1
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

	iconStr := lipgloss.NewStyle().Foreground(iconColor).Render(icon)
	nameStr := lipgloss.NewStyle().Foreground(nameColor).Bold(ctx.IsLoaded || ctx.Selected).Render(ctx.Name)
	descStr := lipgloss.NewStyle().Foreground(p.Overlay1).Render(ns + " · " + cluster)

	titleContent := " " + iconStr + " " + nameStr + currentMark
	descContent := "    " + descStr // indent to align under name

	if isCursor {
		// Mauve bg + Base fg — canonical Catppuccin selection, matches the pane border accent
		titleLine := lipgloss.NewStyle().
			Background(p.Mauve).
			Foreground(p.Base).
			Bold(true).
			Width(paneWidth).
			Render(" " + icon + " " + ctx.Name + currentMark)
		descLine := lipgloss.NewStyle().
			Background(p.Mauve).
			Foreground(p.Base).
			Width(paneWidth).
			Render("    " + ns + " · " + cluster)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	} else {
		titleLine := lipgloss.NewStyle().Width(paneWidth).Render(titleContent)
		descLine := lipgloss.NewStyle().Foreground(p.Overlay0).Width(paneWidth).Render(descContent)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	}
}

// stripANSI removes ANSI escape sequences for width calculation.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			result.WriteRune(r)
		}
	}
	return result.String()
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
	c.list.SetHeight(c.height - 1) // -1 for custom title line
}

func (c *ContextsInfo) GetDimensions() (w, h int) {
	return c.width, c.height
}

func NewContextInfo(client *k8s.Client) *ContextsInfo {
	newList := list.New([]list.Item{}, contextDelegate{}, 0, 0)
	newList.SetShowStatusBar(false)
	newList.SetShowHelp(false)
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
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			c.list, cmd = c.list.Update(msg)
			return cmd
		case "down", "j":
			c.list, cmd = c.list.Update(msg)
			return cmd
		case " ":
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

// getAllContextStates returns currently selected contexts and contexts that were deselected.
func (c *ContextsInfo) getAllContextStates() msgs.ContextsStateMsg {
	selected := []msgs.ContextsSelectedMsg{}
	currentSelected := make(map[string]bool)

	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			selected = append(selected, msgs.ContextsSelectedMsg{
				ContextName:      ctx.Name,
				DefaultNamespace: ctx.DefaultNamespace,
			})
			currentSelected[ctx.Name] = true
		}
	}

	deselected := []string{}
	for prevContext := range c.previouslySelected {
		if !currentSelected[prevContext] {
			deselected = append(deselected, prevContext)
		}
	}

	c.previouslySelected = currentSelected

	return msgs.ContextsStateMsg{
		Selected:   selected,
		Deselected: deselected,
	}
}

func (c *ContextsInfo) confirmSelection() tea.Cmd {
	state := c.getAllContextStates()

	// If nothing selected via Space, treat the focused item as a quick-select
	if len(state.Selected) == 0 && len(state.Deselected) == 0 {
		idx := c.list.Index()
		if idx >= 0 && idx < len(c.list.Items()) {
			if item, ok := c.list.Items()[idx].(contextList); ok {
				state.Selected = append(state.Selected, msgs.ContextsSelectedMsg{
					ContextName:      item.Name,
					DefaultNamespace: item.DefaultNamespace,
				})
				c.previouslySelected[item.Name] = true
			}
		}
	}

	if len(state.Selected) == 0 && len(state.Deselected) == 0 {
		return nil
	}

	return func() tea.Msg { return state }
}

func (c *ContextsInfo) View() string {
	if c.isLoading {
		return ""
	}
	p := styles.CatppuccinMocha()
	title := lipgloss.NewStyle().
		Foreground(p.Flamingo).
		Bold(true).
		Padding(0, 1).
		Width(c.width).
		Render("Contexts")
	return lipgloss.JoinVertical(lipgloss.Left, title, c.list.View())
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
