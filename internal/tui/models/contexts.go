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
}

func (c *ContextsInfo) initContextPane() {
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

func (c *ContextsInfo) setDimensions() {
	c.list.SetWidth(c.width)
	c.list.SetHeight(c.height)
}

func (c *ContextsInfo) GetDimensions() (w, h int) {
	return c.width, c.height
}

func NewContextInfo(client *k8s.Client) *ContextsInfo {
	newListDelegate := list.NewDefaultDelegate()
	newListDelegate.Styles.SelectedTitle = styles.CatppuccinMochaListStyles().Title
	newList := list.New([]list.Item{}, newListDelegate, 0, 0)
	return &ContextsInfo{
		Client:    client,
		PaneTitle: "Kubernetes Contexts",
		list:      newList,
		isLoading: true,
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	// defer c.Skel.TriggerUpdate()
	// c.initContextPane()
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

// Add new method to get all contexts (selected and deselected)
func (c *ContextsInfo) getAllContextStates() msgs.ContextsStateMsg {
	selected := []msgs.ContextsSelectedMsg{}
	deselected := []string{}

	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok {
			if ctx.Selected {
				selected = append(selected, msgs.ContextsSelectedMsg{
					ContextName:      ctx.Name,
					DefaultNamespace: ctx.DefaultNamespace,
				})
			} else {
				deselected = append(deselected, ctx.Name)
			}
		}
	}

	return msgs.ContextsStateMsg{
		Selected:   selected,
		Deselected: deselected,
	}
}

// Replace confirmSelection with this version:
func (c *ContextsInfo) confirmSelection() tea.Cmd {
	state := c.getAllContextStates()

	// If nothing selected, use focused item
	if len(state.Selected) == 0 {
		idx := c.list.Index()
		if idx >= 0 && idx < len(c.list.Items()) {
			if item, ok := c.list.Items()[idx].(contextList); ok {
				state.Selected = append(state.Selected, msgs.ContextsSelectedMsg{
					ContextName:      item.Name,
					DefaultNamespace: item.DefaultNamespace,
				})
			}
		}
	}

	if len(state.Selected) == 0 {
		return nil
	}

	cmd := func() tea.Msg {
		return state
	}

	return cmd
}

func (c *ContextsInfo) View() string {
	c.list.Styles = styles.CatppuccinMochaListStyles()
	c.list.SetShowStatusBar(false)
	c.list.SetShowHelp(false)
	switch c.isLoading {
	case true:
	case false:
		return c.list.View()

	}
	return ""
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
