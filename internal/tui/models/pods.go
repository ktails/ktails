package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/styles"
)

type Pods struct {
	// Context name for this pod list
	ContextName string
	Namespace   string
	Focused     bool
	Client      *k8s.Client
	PaneTitle   string
	table       table.Model
	// Dimensions
	dimensions Dimensions
}

func (p *Pods) SetDimensions(d Dimensions) {
	p.dimensions = d
	frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
	inner := d.GetInnerDimensions(frameW, frameH)
	p.table.SetWidth(inner.Width)
	p.table.SetHeight(inner.Height)
}

func (p *Pods) GetDimensions() Dimensions {
	return p.dimensions
}
func NewPodsModel(client *k8s.Client, contextName, namespace string) *Pods {
	p := &Pods{
		ContextName: contextName,
		Namespace:   namespace,
		Client:      client,
		PaneTitle:   contextName + " - " + namespace,
		table:       table.New(),
		dimensions: Dimensions{Width: 60, Height: 10}, // Default dimensions
	}
	p.initPodListPane()
	return p
}

func (p *Pods) initPodListPane() {
	p.table = table.New(
		table.WithColumns(PodTableColumns()),
		table.WithRows([]table.Row{}),
		table.WithFocused(false),
	)
	// Provide sane defaults so it renders before first WindowSizeMsg
	p.table.SetWidth(60)
	p.table.SetHeight(10)
	p.table.SetStyles(styles.CatppuccinTableStyles())
}

func (p *Pods) Init() tea.Cmd {
	return nil
}

func (p *Pods) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// if p.Width == 0 {
		// 	p.Width = msg.Width
		// }
		// if p.Height == 0 {
		// 	p.Height = msg.Height
		// }
		// // Account for pane borders and padding
		// frameW, frameH := styles.PaneBodyStyle(false).GetFrameSize()
		// innerW := p.Width - frameW
		// if innerW < 10 {
		// 	innerW = 10
		// }
		// innerH := p.Height - (frameH + 1) // +1 for title bar
		// if innerH < 5 {
		// 	innerH = 5
		// }
		// p.table.SetWidth(innerW)
		// p.table.SetHeight(innerH)
		return nil

	case []table.Row:
		// Update rows when PodTableMsg is received
		p.table.SetRows(msg)
		return nil

	case tea.KeyMsg:
		// Forward key messages to the table when focused
		if p.Focused {
			var cmd tea.Cmd
			p.table, cmd = p.table.Update(msg)
			return cmd
		}
		return nil

	default:
		var cmd tea.Cmd
		p.table, cmd = p.table.Update(msg)
		return cmd
	}
}

func (p *Pods) View() string {
	// Update table styles based on focus state
	p.table.SetStyles(styles.TableStylesFocused(p.Focused))

	// Render the table with a titled pane
	return styles.RenderTitledPane(
		p.PaneTitle,
		p.dimensions.Width,
		p.dimensions.Height,
		p.table.View(),
		p.Focused,
	)
}

func (p *Pods) SetFocused(f bool) {
	p.Focused = f
	if f {
		p.table.Focus()
	} else {
		p.table.Blur()
	}
}

// GetSelectedPod returns the currently selected pod name, or empty string if none
func (p *Pods) GetSelectedPod() string {
	if p.table.Cursor() < 0 || p.table.Cursor() >= len(p.table.Rows()) {
		return ""
	}
	row := p.table.SelectedRow()
	if len(row) == 0 {
		return ""
	}
	// First column is pod name
	return row[0]
}

// UpdateRows updates the table rows
func (p *Pods) UpdateRows(rows []table.Row) {
	p.table.SetRows(rows)
}

// GetContext returns the context name for this pod pane
func (p *Pods) GetContext() string {
	return p.ContextName
}

// GetNamespace returns the namespace for this pod pane
func (p *Pods) GetNamespace() string {
	return p.Namespace
}
