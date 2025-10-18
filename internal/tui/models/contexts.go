package models

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/termkit/skeleton"
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
	Skel      *skeleton.Skeleton
	Focused   bool
	PaneTitle string
	list      list.Model
	// Dimensions
	dimensions Dimensions
}
type pageSwitched struct{}

func (c *ContextsInfo) SetDimensions(d Dimensions) {
	c.dimensions = d
	frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
	inner := d.GetInnerDimensions(frameW, frameH, true)
	c.list.SetSize(inner.Width, inner.Height)
}

func (c *ContextsInfo) GetDimensions() Dimensions {
	return c.dimensions
}

func NewContextInfo(s *skeleton.Skeleton, client *k8s.Client) *ContextsInfo {
	newListDelegate := list.NewDefaultDelegate()
	newList := list.New([]list.Item{}, newListDelegate, 0, 0)
	return &ContextsInfo{
		Client:     client,
		PaneTitle:  "Kubernetes Contexts",
		dimensions: Dimensions{Width: 30, Height: 10}, // Default dimensions
		Skel:       s,
		list:       newList,
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	// defer c.Skel.TriggerUpdate()
	c.initContextPane()
	return nil
}

func (c *ContextsInfo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "up", "j":
				c.list, cmd = c.list.Update(msg)
				return c, cmd
			case "down", "k":
				c.list, cmd = c.list.Update(msg)
				return c, cmd
		case " ":
			c.toggleSelection()
		case "enter":
			selectCmd := c.confirmSelection()
			if selectCmd == nil {
				return c, nil
			}

			switchCmd := func() tea.Msg {
				// Replace with the correct skeleton API if different:
				// e.g. SetActivePage, GoToPage, SetActiveTabByKey, etc.
				c.Skel.SetActivePage("pod")
				c.Skel.TriggerUpdate()
				return pageSwitched{}
			}
			selectedCmdSeq := tea.Batch(selectCmd...)

			// Ensure we switch to the pod page before sending the selection.
			return c, tea.Sequence(
				switchCmd,
				selectedCmdSeq)

		}

	// case msgs.ContextsSelectedMsg:
	// 	batchCmds := []tea.Cmd{}
	// 	for _, v := range msg.Contexts {
	// 		namespace := c.Client.DefaultNamespace(v)
	// 		batchCmds = append(batchCmds, cmds.LoadPodInfoCmd(c.Client, v, namespace))
	// 	}
	// 	return c, tea.Batch(batchCmds...)
	default:

	}
	return c, nil
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

func (c *ContextsInfo) confirmSelection() []tea.Cmd {
	selected := c.getSelectedContexts()

	cmds := []tea.Cmd{}
	// If nothing selected, use focused item
	if len(selected) == 0 {
		idx := c.list.Index()
		if idx >= 0 && idx < len(c.list.Items()) {
			if item, ok := c.list.Items()[idx].(contextList); ok {
				msgs := msgs.ContextsSelectedMsg{
					ContextName:      item.Name,
					DefaultNamespace: item.DefaultNamespace,
				}
				selected = append(selected, msgs)
			}
		}
	}

	if len(selected) == 0 {
		return nil
	}
	for _, s := range selected {
		cmd := func() tea.Msg {
			return s
		}
		cmds = append(cmds, cmd)

	}

	return cmds
}

func (c *ContextsInfo) clearSelections() tea.Cmd {
	items := c.list.Items()
	changed := false

	for i, item := range items {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			ctx.Selected = false
			items[i] = ctx
			changed = true
		}
	}

	if changed {
		c.list.SetItems(items)
	}

	return nil
}

func (c *ContextsInfo) getSelectedContexts() []msgs.ContextsSelectedMsg {
	var selected []msgs.ContextsSelectedMsg
	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			msg := msgs.ContextsSelectedMsg{
				ContextName:      ctx.Name,
				DefaultNamespace: ctx.DefaultNamespace,
			}
			selected = append(selected, msg)
		}
	}
	return selected
}

func (c *ContextsInfo) View() string {
	c.list.Styles = styles.CatppuccinMochaListStylesFocused(c.Focused)
	c.list.SetShowStatusBar(false)
	c.list.SetShowTitle(false)
	c.list.SetShowHelp(false)
	return c.list.View()
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

	delegate := list.NewDefaultDelegate()
	c.list = list.New(itemList, delegate, c.Skel.GetTerminalWidth(), c.Skel.GetTerminalHeight())
	c.list.Title = ""
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
	return cl.DefaultNamespace
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
