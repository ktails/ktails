// Package views contains master layout of KTail, Left Pane(k8s context), Right Top Pane(pod list view), Right Bottom Pane(Pod Details views)
package views

import (
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
	"github.com/termkit/skeleton"
)

type MasterLayout struct {
	ContextPane *models.ContextsInfo
	PodPages    *skeleton.Skeleton
}

func NewLayout(client *k8s.Client) MasterLayout {
	ctxPane := models.NewContextInfo(client)
	s := skeleton.NewSkeleton()
	s.AddPage("No Pods", "No Pods to display. Select a context to view pods.", nil)
	return MasterLayout{
		ContextPane: ctxPane,
		PodPages:    s,
	}
}
