// Package models
package models

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
)

// Ensure ContextsInfo implements list.Item
type contextList struct {
	Name             string
	Cluster          string
	DefaultNamespace string
}

type ContextsInfo struct {
	Width     int
	Height    int
	Client    *k8s.Client
	Focused   bool
	PaneTitle string
	list      list.Model
}

func NewContextInfo(client *k8s.Client) *ContextsInfo {
	return &ContextsInfo{
		Client:    client,
		PaneTitle: "Kubernetes Contexts",
	}
}

func (c *ContextsInfo) Init() tea.Cmd {
	c.initContextPane()
	// Optionally show spinner/loading later; nothing to do for now
	return nil
}

func (c *ContextsInfo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		if c.Width == 0 {
			c.Width = m.Width / 3
		}
		if c.Height == 0 {
			c.Height = m.Height
		}
		// Frame for body (no top border), plus 1 line for title
		frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
		innerW := c.Width - frameW
		if innerW < 10 {
			innerW = 10
		}
		innerH := c.Height - (frameH + 1)
		if innerH < 5 {
			innerH = 5
		}
		c.list.SetSize(innerW, innerH)
		return c, nil
	default:
		var cmd tea.Cmd
		c.list, cmd = c.list.Update(msg)
		return c, cmd
	}
}

func (c *ContextsInfo) View() string {
	c.list.Styles = styles.CatppuccinMochaListStylesFocused(c.Focused)
	c.list.SetShowStatusBar(false)
	c.list.SetShowTitle(false)
	return styles.RenderTitledPane(c.PaneTitle, c.Width, c.Height, c.list.View(), c.Focused)
}

// initContextPane
func (c *ContextsInfo) initContextPane() {
	rawContextsList, err := c.Client.ListContexts()
	if err != nil {
		log.Printf("unable to fetch context from client: %v", err)
	}
	itemList := make([]list.Item, 0, len(rawContextsList))
	for _, ctxInfo := range rawContextsList {
		itemList = append(itemList, contextList{
			ctxInfo.Name,
			ctxInfo.Cluster,
			ctxInfo.DefaultNamespace,
		})
	}
	delegate := list.NewDefaultDelegate()
	c.list = list.New(itemList, delegate, 0, 0)
	// We render the title in the pane border, so hide the list's internal title
	c.list.Title = ""
}

func (cl contextList) Title() string {
	// Prefix with selection marker and current-context star
	return cl.Name
}

func (cl contextList) Description() string {
	// You can customize the description; keep minimal for now
	s := fmt.Sprintf("Namespace: %s\nCluster: %s", cl.DefaultNamespace, cl.Cluster)
	return s
}

func (cl contextList) FilterValue() string {
	return cl.Name
}

func (c *ContextsInfo) HelpView() string {
	return c.list.Help.FullHelpView(c.list.FullHelp())
}

func (c *ContextsInfo) SetStyle(styles list.Styles) {
	c.list.Styles = styles
}

// SetFocused toggles pane focus styling
func (c *ContextsInfo) SetFocused(f bool) {
	c.Focused = f
}
