package models

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/termkit/skeleton"
)

type DeploymentPage struct {
	Skel   *skeleton.Skeleton
	Client *k8s.Client
	table  table.Model
}

func NewDeploymentPage(s *skeleton.Skeleton, client *k8s.Client) *DeploymentPage {
	return &DeploymentPage{
		Skel:   s,
		Client: client,
		table:  table.New(table.WithColumns(DeploymentTableColumns())),
	}
}
