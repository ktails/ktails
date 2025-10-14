// Package views contains master layout of KTail, Left Pane(k8s context), Right Top Pane(pod list view), Right Bottom Pane(Pod Details views)
package views

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
)

type MasterLayout struct {
	ContextPane list.Model
	PodListPane table.Model
}

func NewLayout() MasterLayout {
	// Initialize list with a default delegate to avoid nil pointers inside the model
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 0, 0)
	// Initialize table so it has non-nil internals and a header
	t := table.New(
		table.WithColumns(PodTableColumns()),
		table.WithRows([]table.Row{}),
	)
	// Provide sane defaults so it renders before first WindowSizeMsg
	t.SetWidth(60)
	t.SetHeight(10)
	return MasterLayout{
		ContextPane: l,
		PodListPane: t,
	}
}
