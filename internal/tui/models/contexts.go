package models

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
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
	list      list.Model
	// Dimensions
	dimensions Dimensions
}

func (c *ContextsInfo) SetDimensions(d Dimensions) {
	c.dimensions = d
	frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
	inner := d.GetInnerDimensions(frameW, frameH, true)
	c.list.SetSize(inner.Width, inner.Height)
}

func (c *ContextsInfo) GetDimensions() Dimensions {
	return c.dimensions
}

func NewContextInfo(client *k8s.Client) *ContextsInfo {
	return &ContextsInfo{
		Client:     client,
		PaneTitle:  "Kubernetes Contexts",
		dimensions: Dimensions{Width: 30, Height: 10}, // Default dimensions
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	c.initContextPane()
	return nil
}

func (c *ContextsInfo) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		// if c.Width == 0 {
		// 	c.Width = m.Width / 3
		// }
		// if c.Height == 0 {
		// 	c.Height = m.Height
		// }
		// frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
		// innerW := c.Width - frameW
		// if innerW < 10 {
		// 	innerW = 10
		// }
		// innerH := c.Height - (frameH + 1)
		// if innerH < 5 {
		// 	innerH = 5
		// }
		// c.list.SetSize(innerW, innerH)
		return nil

	case tea.KeyMsg:
		switch m.String() {
		case " ":
			return c.toggleSelection()
		case "enter":
			return c.confirmSelection()
		case "esc":
			return c.clearSelections()
		default:
			var cmd tea.Cmd
			c.list, cmd = c.list.Update(msg)
			return cmd
		}
	default:
		var cmd tea.Cmd
		c.list, cmd = c.list.Update(msg)
		return cmd
	}
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

func (c *ContextsInfo) confirmSelection() tea.Cmd {
	selected := c.getSelectedContexts()

	// If nothing selected, use focused item
	if len(selected) == 0 {
		idx := c.list.Index()
		if idx >= 0 && idx < len(c.list.Items()) {
			if item, ok := c.list.Items()[idx].(contextList); ok {
				selected = []string{item.Name}
			}
		}
	}

	if len(selected) == 0 {
		return nil
	}

	return func() tea.Msg {
		return msgs.ContextsSelectedMsg{
			Contexts: selected,
		}
	}
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

func (c *ContextsInfo) getSelectedContexts() []string {
	var selected []string
	for _, item := range c.list.Items() {
		if ctx, ok := item.(contextList); ok && ctx.Selected {
			selected = append(selected, ctx.Name)
		}
	}
	return selected
}

func (c *ContextsInfo) View() string {
	c.list.Styles = styles.CatppuccinMochaListStylesFocused(c.Focused)
	c.list.SetShowStatusBar(false)
	c.list.SetShowTitle(false)
	return styles.RenderTitledPane(c.PaneTitle, c.dimensions.Width, c.dimensions.Height, c.list.View(), c.Focused)
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
	c.list = list.New(itemList, delegate, 0, 0)
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
	return fmt.Sprintf("Namespace: %s\nCluster: %s", cl.DefaultNamespace, cl.Cluster)
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
