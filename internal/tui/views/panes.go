// Package views contains master layout of KTail, Left Pane(k8s context), Right Top Pane(pod list view), Right Bottom Pane(Pod Details views)
package views

import (
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/models"
)

type MasterLayout struct {
	ContextPane *models.ContextsInfo
	PodListPane map[string]*models.Pods
}

func NewLayout(client *k8s.Client) MasterLayout {
	ctxPane := models.NewContextInfo(client)
	return MasterLayout{
		ContextPane: ctxPane,
		PodListPane: map[string]*models.Pods{"All Namespaces": models.NewPodsModel(client)},
	}
}
