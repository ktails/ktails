// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
)

// RowData is a keyed row of field values for the Pods/Deployments/svc
// tables. Keys matching a table.Column's key are displayed; others (e.g.
// PodKeyContext/PodKeyContainers) ride along as hidden metadata for the
// Detail pane / Log pane to read without a visible column of their own.
type RowData = map[string]any

// Column keys for Pods rows (see cmds.PodWatchCache.Rows / models.PodPage).
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

// Column keys for Deployments rows (see cmds.DeploymentWatchCache.Rows).
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

// Column keys for svc rows (see cmds.ServiceWatchCache.Rows).
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
	SvcKeyEndpointIPs = "endpointIPs" // wide mode only, "..." until lazily fetched
)

// ContextsSelectedMsg represents a selected context with its namespace
type ContextsSelectedMsg struct {
	ContextName      string
	DefaultNamespace string
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
// whoever handles it. Watches keep table data current on their own; this
// tick now just re-renders Age text from the local watch caches (no API
// calls) — see MainPage's RefreshTickMsg handler.
type RefreshTickMsg struct{}

// PodWatchOpenedMsg carries a freshly opened Pods watch for one
// context+namespace. Generation must match that context's current
// generation in MainPage before the watch is adopted — otherwise it's been
// superseded (a manual "r" restart or context deselect) and should just be
// stopped.
type PodWatchOpenedMsg struct {
	Context    string
	Generation int
	Watcher    watch.Interface
}

// PodWatchEventMsg carries a freshly rebuilt row set for one context's Pods
// watch cache, after applying one or more buffered watch events.
type PodWatchEventMsg struct {
	Context    string
	Generation int
	Rows       []RowData
}

// PodWatchClosedMsg reports that one context's Pods watch ended, either
// cleanly (Err == nil) or because opening/reading it failed (Err != nil).
type PodWatchClosedMsg struct {
	Context    string
	Generation int
	Err        error
}

// DeploymentWatchOpenedMsg mirrors PodWatchOpenedMsg for Deployments.
type DeploymentWatchOpenedMsg struct {
	Context    string
	Generation int
	Watcher    watch.Interface
}

// DeploymentWatchEventMsg mirrors PodWatchEventMsg for Deployments.
type DeploymentWatchEventMsg struct {
	Context    string
	Generation int
	Rows       []RowData
}

// DeploymentWatchClosedMsg mirrors PodWatchClosedMsg for Deployments.
type DeploymentWatchClosedMsg struct {
	Context    string
	Generation int
	Err        error
}

// ServiceWatchOpenedMsg mirrors PodWatchOpenedMsg for Services.
type ServiceWatchOpenedMsg struct {
	Context    string
	Generation int
	Watcher    watch.Interface
}

// ServiceWatchEventMsg mirrors PodWatchEventMsg for Services.
type ServiceWatchEventMsg struct {
	Context    string
	Generation int
	Rows       []RowData
}

// ServiceWatchClosedMsg mirrors PodWatchClosedMsg for Services.
type ServiceWatchClosedMsg struct {
	Context    string
	Generation int
	Err        error
}
