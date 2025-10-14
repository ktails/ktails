// Package views contains master layout of KTail, Left Pane(k8s context), Right Top Pane(pod list view), Right Bottom Pane(Pod Details views)
package views

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
)

type MasterLayout struct {
	ContextPane *models.ContextsInfo
	PodListPane []table.Model
}

func NewLayout(client *k8s.Client) MasterLayout {
	// Initialize table so it has non-nil internals and a header
	t := table.New(
		table.WithColumns(PodTableColumns()),
		table.WithRows([]table.Row{}),
	)
	// Provide sane defaults so it renders before first WindowSizeMsg
	t.SetWidth(60)
	t.SetHeight(10)
	ctxPane := models.NewContextInfo(client)
	return MasterLayout{
		ContextPane: ctxPane,
		PodListPane: []table.Model{t},
	}
}
