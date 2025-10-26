package models

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type contextList struct {
	Name             string
	Cluster          string
	DefaultNamespace string
	Selected         bool
	IsCurrent        bool
}

type ContextsInfo struct {
	Client    *k8s.Client
	Focused   bool
	PaneTitle string
	// bubbles
	list list.Model
	// Dimensions
	width  int
	height int
	// states
	isLoading bool
	// Track what was previously confirmed/selected
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
	newListDelegate := list.NewDefaultDelegate()
	newListDelegate.Styles.SelectedTitle = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Flamingo)
	newListDelegate.Styles.SelectedDesc = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Rosewater)
	newList := list.New([]list.Item{}, newListDelegate, 0, 0)
	return &ContextsInfo{
		Client:             client,
		PaneTitle:          "Kubernetes Contexts",
		list:               newList,
		isLoading:          true,
		previouslySelected: make(map[string]bool),
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	return nil
}

func (c *ContextsInfo) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.setDimensions()
	// don't return here; allow init to run below on first update
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
		case "enter":
			selectCmd := c.confirmSelection()
			if selectCmd == nil {
				return nil
			}
			return selectCmd
		default:
			c.list, cmd = c.list.Update(msg)
			return cmd
		}

	default:

	}
	switch c.isLoading {
	case true:
		c.initContextPane()
		return nil
	}
	return nil
}

func (c *ContextsInfo) toggleSelection() tea.Cmd {
	idx := c.list.Index()
	if idx < 0 {
		return nil
	}

	items := c.list.Items()
	if idx >= len(items) {
		return nil
	}

	if item, ok := items[idx].(contextList); ok {
		item.Selected = !item.Selected
		items[idx] = item
	}

	c.list.SetItems(items)
	c.list.Select(idx)

	return nil
}

// getAllContextStates returns currently selected contexts and contexts that were deselected
func (c *ContextsInfo) getAllContextStates() msgs.ContextsStateMsg {
	selected := []msgs.ContextsSelectedMsg{}
	currentSelected := make(map[string]bool)

	// Gather currently selected contexts
	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			selected = append(selected, msgs.ContextsSelectedMsg{
				ContextName:      ctx.Name,
				DefaultNamespace: ctx.DefaultNamespace,
			})
			currentSelected[ctx.Name] = true
		}
	}

	// Find what was deselected (was selected before, not selected now)
	deselected := []string{}
	for prevContext := range c.previouslySelected {
		if !currentSelected[prevContext] {
			deselected = append(deselected, prevContext)
		}
	}

	// Update tracking for next time
	c.previouslySelected = currentSelected

	return msgs.ContextsStateMsg{
		Selected:   selected,
		Deselected: deselected,
	}
}

func (c *ContextsInfo) confirmSelection() tea.Cmd {
	state := c.getAllContextStates()

	// If nothing selected, use focused item
	// Only auto-select the focused item when there are no deselections.
	// This allows users to truly clear all selections: when contexts are
	// deselected, we should not re-add the focused item implicitly.
	if len(state.Selected) == 0 && len(state.Deselected) == 0 {
		idx := c.list.Index()
		if idx >= 0 && idx < len(c.list.Items()) {
			if item, ok := c.list.Items()[idx].(contextList); ok {
				state.Selected = append(state.Selected, msgs.ContextsSelectedMsg{
					ContextName:      item.Name,
					DefaultNamespace: item.DefaultNamespace,
				})
				// Also update tracking since we're adding this
				c.previouslySelected[item.Name] = true
			}
		}
	}

	if len(state.Selected) == 0 && len(state.Deselected) == 0 {
		return nil
	}

	cmd := func() tea.Msg {
		return state
	}

	return cmd
}

func (c *ContextsInfo) View() string {
	c.list.Styles = styles.CatppuccinMochaListStyles()
	c.list.Styles.Title = styles.CatppuccinMochaListStyles().Title.Faint(true).Width(c.width).Padding(0, 1)
	c.list.SetShowStatusBar(false)
	c.list.SetShowHelp(false)
	switch c.isLoading {
	case true:
		return ""
	case false:
		return c.list.View()
	}
	return ""
}

func (c *ContextsInfo) initContextPane() {
	newListDelegate := list.NewDefaultDelegate()
	newListDelegate.Styles.SelectedTitle = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Flamingo).Width(c.width).Padding(0, 1)
	newListDelegate.Styles.SelectedDesc = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Rosewater).Width(c.width).Padding(0, 1)
	newListDelegate.Styles.NormalTitle = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Flamingo).Width(c.width).Padding(0, 1).Faint(true)
	newListDelegate.Styles.NormalDesc = styles.CatppuccinMochaListStyles().Title.Foreground(styles.CatppuccinLatte().Rosewater).Width(c.width).Padding(0, 1).Faint(true)
	c.list.SetDelegate(newListDelegate)
	rawContextsList, err := c.Client.ListContexts()
	if err != nil {
		log.Printf("unable to fetch context from client: %v", err)
	}

	currentCtx := c.Client.GetCurrentContext()
	itemList := make([]list.Item, 0, len(rawContextsList))

	for _, ctxInfo := range rawContextsList {
		ctx := contextList{
			Name:             ctxInfo.Name,
			Cluster:          ctxInfo.Cluster,
			DefaultNamespace: ctxInfo.DefaultNamespace,
			Selected:         false,
			IsCurrent:        ctxInfo.Name == currentCtx,
		}
		itemList = append(itemList, ctx)
	}

	c.list.SetItems(itemList)
	c.list.Title = "Select Kubernetes Contexts"
	c.isLoading = false
}

func (cl contextList) Title() string {
	checkbox := "[ ]"
	if cl.Selected {
		checkbox = "[x]"
	}
	star := ""
	if cl.IsCurrent {
		star = " ★"
	}
	return fmt.Sprintf("%s %s%s", checkbox, cl.Name, star)
}

func (cl contextList) Description() string {
	return cl.DefaultNamespace + "\n" + cl.Cluster
}

func (cl contextList) FilterValue() string {
	return cl.Name
}

func (c *ContextsInfo) HelpView() string {
	return c.list.Help.FullHelpView(c.list.FullHelp())
}

func (c *ContextsInfo) SetFocused(f bool) {
	c.Focused = f
}
