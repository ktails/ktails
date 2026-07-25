// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
)

// ResourceKind identifies one of the watched resource types. It is the
// single tab/watch/cache identity used everywhere a "Deployments" | "Pods" |
// "svc" string switch used to live.
type ResourceKind int

const (
	KindDeployments ResourceKind = iota
	KindPods
	KindServices
	KindConfigMaps
	KindSecrets
	// KindNodes is cluster-scoped, not namespaced — its rows carry no
	// KeyNamespace, and its watch ignores the namespace argument.
	KindNodes
)

// Kinds returns every ResourceKind in tab order.
func Kinds() []ResourceKind {
	return []ResourceKind{KindDeployments, KindPods, KindServices, KindConfigMaps, KindSecrets, KindNodes}
}

// Title is the tab label shown in the Tab Area header.
func (k ResourceKind) Title() string {
	switch k {
	case KindDeployments:
		return "Deployments"
	case KindPods:
		return "Pods"
	case KindServices:
		return "svc"
	case KindConfigMaps:
		return "ConfigMaps"
	case KindSecrets:
		return "Secrets"
	case KindNodes:
		return "Nodes"
	}
	return ""
}

// Kind is the Kubernetes kind name, as shown in the Detail Pane.
func (k ResourceKind) Kind() string {
	switch k {
	case KindDeployments:
		return "Deployment"
	case KindPods:
		return "Pod"
	case KindServices:
		return "Service"
	case KindConfigMaps:
		return "ConfigMap"
	case KindSecrets:
		return "Secret"
	case KindNodes:
		return "Node"
	}
	return ""
}

func (k ResourceKind) String() string {
	return k.Title()
}

// RowData is a keyed row of field values for the Pods/Deployments/svc
// tables. Keys matching a table.Column's key are displayed; others (e.g.
// KeyContext/PodKeyContainers) ride along as hidden metadata for the
// Detail pane / Log pane to read without a visible column of their own.
type RowData = map[string]any

// Shared row keys — deliberately identical across all three resource kinds,
// so kind-agnostic callers (the Detail Pane's row→identity extraction) can
// read a selected row without switching on its kind.
const (
	KeyName      = "name"
	KeyNamespace = "namespace"
	KeyContext   = "context"
)

// Column keys for Pods rows (see watch.Supervisor / models.ResourceTable).
const (
	PodKeyCheck      = "check"
	PodKeyName       = KeyName
	PodKeyNamespace  = KeyNamespace
	PodKeyStatus     = "status"
	PodKeyRestarts   = "restarts"
	PodKeyAge        = "age"
	PodKeyContext    = KeyContext   // hidden, used by the detail tab
	PodKeyContainers = "containers" // hidden, comma-separated, used by the log pane
	PodKeyNode       = "node"       // wide mode only
	PodKeyNodeIP     = "nodeIP"     // wide mode only
	PodKeyPodIP      = "podIP"      // wide mode only
	PodKeyReady      = "ready"      // wide mode only, "ready/total" containers
)

// Column keys for Deployments rows.
const (
	DeployKeyName      = KeyName
	DeployKeyAge       = "age"
	DeployKeyReplicas  = "replicas"
	DeployKeyContext   = KeyContext
	DeployKeyNamespace = KeyNamespace // hidden, used by the detail panel
	DeployKeyStrategy  = "strategy"   // wide mode only
	DeployKeyAvailable = "available"  // wide mode only
	DeployKeyUpdated   = "updated"    // wide mode only
	DeployKeySelector  = "selector"   // wide mode only
)

// Column keys for svc rows.
const (
	SvcKeyName        = KeyName
	SvcKeyNamespace   = KeyNamespace
	SvcKeyType        = "type"
	SvcKeyClusterIP   = "clusterIP"
	SvcKeyPorts       = "ports"
	SvcKeyAge         = "age"
	SvcKeyContext     = KeyContext    // hidden, used by the detail tab
	SvcKeySelector    = "selector"    // wide mode only
	SvcKeyExternalIP  = "externalIP"  // wide mode only
	SvcKeyEndpointIPs = "endpointIPs" // wide mode only, "…" until lazily fetched
)

// Column keys for ConfigMaps rows.
const (
	ConfigMapKeyName      = KeyName
	ConfigMapKeyNamespace = KeyNamespace
	ConfigMapKeyKeys      = "keys" // count of data keys, e.g. "3"
	ConfigMapKeyAge       = "age"
	ConfigMapKeyContext   = KeyContext // hidden, used by the detail tab
	ConfigMapKeyKeyNames  = "keyNames" // wide mode only, comma-joined data keys
)

// Column keys for Secrets rows. Values themselves are never surfaced in the
// table or the Detail Pane's YAML — see watch's secretRow and
// k8s.GetSecretDetail's redaction.
const (
	SecretKeyName      = KeyName
	SecretKeyNamespace = KeyNamespace
	SecretKeyType      = "type"
	SecretKeyKeys      = "keys" // count of data keys, e.g. "2"
	SecretKeyAge       = "age"
	SecretKeyContext   = KeyContext // hidden, used by the detail tab
)

// Column keys for Nodes rows. Nodes are cluster-scoped: there is no
// KeyNamespace here, and Node rows are not deduplicated per namespace.
const (
	NodeKeyName       = KeyName
	NodeKeyStatus     = "status"
	NodeKeyRoles      = "roles"
	NodeKeyAge        = "age"
	NodeKeyVersion    = "version"
	NodeKeyContext    = KeyContext   // hidden, used by the detail tab
	NodeKeyInternalIP = "internalIP" // wide mode only
	NodeKeyOS         = "os"         // wide mode only
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

// WatchOpenedMsg carries a freshly opened watch for one resource kind in one
// context+namespace. Generation must match that (kind, context)'s current
// generation in the watch Supervisor before the watch is adopted — otherwise
// it's been superseded (a manual "r" restart or context deselect) and should
// just be stopped.
type WatchOpenedMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
	Watcher    watch.Interface
}

// WatchEventMsg carries a freshly rebuilt row set for one (kind, context)
// watch cache, after applying one or more buffered watch events.
type WatchEventMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
	Rows       []RowData
}

// WatchClosedMsg reports that one (kind, context) watch ended, either
// cleanly (Err == nil) or because opening/reading it failed (Err != nil).
type WatchClosedMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
	Err        error
}
