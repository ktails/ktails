// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import "github.com/charmbracelet/bubbles/table"

type PodTableMsg struct {
	Context string
	Rows    []table.Row
}
type ContextsSelectedMsg struct {
	ContextName      string
	DefaultNamespace string
}
type DeploymentTableMsg struct {
	Context string
	Rows    []table.Row
	Err     error
}
type ContextsStateMsg struct {
	Selected   []ContextsSelectedMsg
	Deselected []string // context names to remove
}
