package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
)

type Pods struct {
	// Window size
	Width  int
	Height int
	// Context name for this pod list; "All Namespaces" means all contexts
	ContextName string
	Focused     bool
	Client      *k8s.Client
	table       table.Model
}

func NewPodsModel(client *k8s.Client) *Pods {
	return &Pods{
		ContextName: "All Namespaces",
		Client:      client,
		table:       table.New(),
	}
}

// initContextPane

func (p *Pods) initPodListPane() {
	// leave any default panes created by the layout; ensure at least one table exists
	p.table = table.New(
		table.WithColumns(PodTableColumns()),
		table.WithRows([]table.Row{}),
	)
	// Provide sane defaults so it renders before first WindowSizeMsg
	p.table.SetWidth(60)
	p.table.SetHeight(10)
}

func (p *Pods) Init() tea.Cmd {
	// initialize panes
	p.initPodListPane()
	return nil
}

func (p *Pods) Update(tea.Msg) tea.Cmd {

	return nil
}

func (p *Pods) View() string {
	return ""
}

func (p *Pods) SetFocused(f bool) {
	p.Focused = f
}
