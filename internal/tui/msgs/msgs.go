// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/ktails/ktails/internal/k8s"
)

// PodTableMsg carries pod data or errors from async operations
type PodTableMsg struct {
	Context string
	Rows    []table.Row
	Err     error // Error during pod fetching
}

// ContextsSelectedMsg represents a selected context with its namespace
type ContextsSelectedMsg struct {
	ContextName      string
	DefaultNamespace string
}

// DeploymentTableMsg carries deployment data or errors from async operations
type DeploymentTableMsg struct {
	Context string
	Rows    []table.Row
	Err     error // Error during deployment fetching
}

// ContextsStateMsg represents the current state of context selections
type ContextsStateMsg struct {
	Selected   []ContextsSelectedMsg
	Deselected []string // context names to remove
}

// ResourceDetailMsg carries a single resource's (Deployment, Pod, ...) detail
// data or an error from an async fetch, for the Detail tab.
type ResourceDetailMsg struct {
	Context string
	Detail  k8s.ResourceDetail
	Err     error
}

// ErrorMsg is a general error message for displaying errors to users
type ErrorMsg struct {
	Context string // Which context caused the error (if applicable)
	Title   string // Short error title
	Err     error  // The actual error
}
