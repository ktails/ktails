// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"

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

// ServiceTableMsg carries service data or errors from async operations
type ServiceTableMsg struct {
	Context string
	Rows    []table.Row
	Err     error // Error during service fetching
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

// LogStreamOpenedMsg carries a freshly opened pod log stream for one source
// in the merged Log pane. SourceKey identifies which pod/container/context
// this belongs to; Generation must match that source's current generation
// in MainPage before the stream is adopted — otherwise this source has
// since been restarted or closed and the stream should just be closed.
type LogStreamOpenedMsg struct {
	SourceKey  string
	Generation int
	Stream     io.ReadCloser
}

// LogLineMsg carries a single line read from one source's open log stream.
type LogLineMsg struct {
	SourceKey  string
	Generation int
	Line       string
}

// LogStreamClosedMsg reports that one source's log stream ended, either
// because the server closed it (Err == nil, e.g. a non-following read
// finished) or because opening/reading it failed (Err != nil).
type LogStreamClosedMsg struct {
	SourceKey  string
	Generation int
	Err        error
}

// RefreshTickMsg fires on the auto-refresh interval, self-rescheduled by
// whoever handles it. It carries nothing — its only job is to trigger the
// same per-tab refresh path manual refresh uses.
type RefreshTickMsg struct{}
