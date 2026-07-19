// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"

	"github.com/ktails/ktails/internal/k8s"
)

// RowData is a keyed row of field values for the Pods/Deployments/svc
// tables. Keys matching a table.Column's key are displayed; others (e.g.
// PodKeyContext/PodKeyContainers) ride along as hidden metadata for the
// Detail pane / Log pane to read without a visible column of their own.
type RowData = map[string]any

// Column keys for Pods rows (see cmds.LoadPodInfoCmd / models.PodPage).
const (
	PodKeyCheck      = "check"
	PodKeyName       = "name"
	PodKeyNamespace  = "namespace"
	PodKeyStatus     = "status"
	PodKeyRestarts   = "restarts"
	PodKeyAge        = "age"
	PodKeyContext    = "context"    // hidden, used by the detail tab
	PodKeyContainers = "containers" // hidden, comma-separated, used by the log pane
	PodKeyNode       = "node"       // wide mode only
	PodKeyNodeIP     = "nodeIP"     // wide mode only
	PodKeyPodIP      = "podIP"      // wide mode only
	PodKeyReady      = "ready"      // wide mode only, "ready/total" containers
)

// Column keys for Deployments rows (see cmds.LoadDeploymentInfoCmd).
const (
	DeployKeyName      = "name"
	DeployKeyAge       = "age"
	DeployKeyReplicas  = "replicas"
	DeployKeyContext   = "context"
	DeployKeyNamespace = "namespace" // hidden, used by the detail panel
	DeployKeyStrategy  = "strategy"  // wide mode only
	DeployKeyAvailable = "available" // wide mode only
	DeployKeyUpdated   = "updated"   // wide mode only
	DeployKeySelector  = "selector"  // wide mode only
)

// Column keys for svc rows (see cmds.LoadServiceInfoCmd).
const (
	SvcKeyName        = "name"
	SvcKeyNamespace   = "namespace"
	SvcKeyType        = "type"
	SvcKeyClusterIP   = "clusterIP"
	SvcKeyPorts       = "ports"
	SvcKeyAge         = "age"
	SvcKeyContext     = "context"     // hidden, used by the detail tab
	SvcKeySelector    = "selector"    // wide mode only
	SvcKeyExternalIP  = "externalIP"  // wide mode only
	SvcKeyEndpointIPs = "endpointIPs" // wide mode only, "…" until lazily fetched
)

// PodTableMsg carries pod data or errors from async operations
type PodTableMsg struct {
	Context string
	Rows    []RowData
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
	Rows    []RowData
	Err     error // Error during deployment fetching
}

// ServiceTableMsg carries service data or errors from async operations
type ServiceTableMsg struct {
	Context string
	Rows    []RowData
	Err     error // Error during service fetching
}

// ServiceEndpointsMsg carries lazily-fetched Endpoint IPs (service name ->
// IP list) for every service in one context+namespace, or an error. Fetched
// once per context+namespace the first time svc wide mode turns on — see
// cmds.LoadServiceEndpointsCmd.
type ServiceEndpointsMsg struct {
	Context   string
	Namespace string
	Endpoints map[string][]string
	Err       error
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
