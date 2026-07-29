// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	KindJobs
	KindCronJobs
	KindStatefulSets
	KindDaemonSets
	KindIngresses
	KindPodDisruptionBudgets
	KindHorizontalPodAutoscalers
	// KindNodes is cluster-scoped, not namespaced — its rows carry no
	// KeyNamespace, and its watch ignores the namespace argument.
	KindNodes
)

// Kinds returns every ResourceKind in tab order.
func Kinds() []ResourceKind {
	return []ResourceKind{
		KindDeployments, KindPods, KindServices, KindConfigMaps, KindSecrets,
		KindJobs, KindCronJobs, KindStatefulSets, KindDaemonSets, KindIngresses,
		KindPodDisruptionBudgets, KindHorizontalPodAutoscalers,
		KindNodes,
	}
}

// Title is the tab label shown in the Tab Area header.
func (k ResourceKind) Title() string {
	switch k {
	case KindDeployments:
		return "Deployments"
	case KindPods:
		return "Pods"
	case KindServices:
		return "Services"
	case KindConfigMaps:
		return "ConfigMaps"
	case KindSecrets:
		return "Secrets"
	case KindJobs:
		return "Jobs"
	case KindCronJobs:
		return "CronJobs"
	case KindStatefulSets:
		return "StatefulSets"
	case KindDaemonSets:
		return "DaemonSets"
	case KindIngresses:
		return "Ingresses"
	case KindPodDisruptionBudgets:
		return "PDBs"
	case KindHorizontalPodAutoscalers:
		return "HPAs"
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
	case KindJobs:
		return "Job"
	case KindCronJobs:
		return "CronJob"
	case KindStatefulSets:
		return "StatefulSet"
	case KindDaemonSets:
		return "DaemonSet"
	case KindIngresses:
		return "Ingress"
	case KindPodDisruptionBudgets:
		return "PodDisruptionBudget"
	case KindHorizontalPodAutoscalers:
		return "HorizontalPodAutoscaler"
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
	// KeyCreatedAt carries each row's raw creation timestamp (time.Time),
	// set once at row-build time by watch.cache's row converters. It backs
	// nothing visible — Age's displayed string comes from the per-kind
	// KeyAge field — it exists purely so Age can be sorted chronologically
	// rather than as text (see MainPage.sortRows).
	KeyCreatedAt = "createdAt"
)

// Column keys for Pods rows (see watch.Supervisor / models.ResourceTable).
const (
	PodKeyCheck      = "check"
	PodKeyName       = KeyName
	PodKeyNamespace  = KeyNamespace
	PodKeyStatus     = "status"
	PodKeyRestarts   = "restarts"
	PodKeyAge        = "age"
	PodKeyContext    = KeyContext   // Context column; also used by the detail tab
	PodKeyContainers = "containers" // hidden, comma-separated, used by the log pane
	PodKeyNode       = "node"       // wide mode only
	PodKeyNodeIP     = "nodeIP"     // wide mode only
	PodKeyPodIP      = "podIP"      // wide mode only
	PodKeyReady      = "ready"      // wide mode only, "ready/total" containers
	PodKeyCPU        = "cpu"        // "-" until lazily fetched from metrics-server
	PodKeyMemory     = "memory"     // "-" until lazily fetched from metrics-server
)

// Column keys for Deployments rows.
const (
	DeployKeyName      = KeyName
	DeployKeyAge       = "age"
	DeployKeyReplicas  = "replicas"
	DeployKeyContext   = KeyContext
	DeployKeyNamespace = KeyNamespace
	DeployKeyStrategy  = "strategy"  // wide mode only
	DeployKeyAvailable = "available" // wide mode only
	DeployKeyUpdated   = "updated"   // wide mode only
	DeployKeySelector  = "selector"  // wide mode only
)

// Column keys for svc rows.
const (
	SvcKeyName        = KeyName
	SvcKeyNamespace   = KeyNamespace
	SvcKeyType        = "type"
	SvcKeyClusterIP   = "clusterIP"
	SvcKeyPorts       = "ports"
	SvcKeyAge         = "age"
	SvcKeyContext     = KeyContext    // Context column; also used by the detail tab
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
	ConfigMapKeyContext   = KeyContext // Context column; also used by the detail tab
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
	SecretKeyContext   = KeyContext // Context column; also used by the detail tab
)

// Column keys for Jobs rows.
const (
	JobKeyName        = KeyName
	JobKeyNamespace   = KeyNamespace
	JobKeyCompletions = "completions"
	JobKeyDuration    = "duration"
	JobKeyAge         = "age"
	JobKeyContext     = KeyContext // Context column; also used by the detail tab
	JobKeyStatus      = "status"   // wide mode only
)

// Column keys for CronJobs rows.
const (
	CronJobKeyName          = KeyName
	CronJobKeyNamespace     = KeyNamespace
	CronJobKeySchedule      = "schedule"
	CronJobKeySuspend       = "suspend"
	CronJobKeyAge           = "age"
	CronJobKeyContext       = KeyContext      // Context column; also used by the detail tab
	CronJobKeyLastScheduled = "lastScheduled" // wide mode only
)

// Column keys for StatefulSets rows.
const (
	StatefulSetKeyName      = KeyName
	StatefulSetKeyNamespace = KeyNamespace
	StatefulSetKeyReady     = "ready" // "ready/desired" replicas
	StatefulSetKeyAge       = "age"
	StatefulSetKeyContext   = KeyContext // Context column; also used by the detail tab
	StatefulSetKeySelector  = "selector" // wide mode only
)

// Column keys for DaemonSets rows.
const (
	DaemonSetKeyName      = KeyName
	DaemonSetKeyNamespace = KeyNamespace
	DaemonSetKeyReady     = "ready" // "ready/desired" node count
	DaemonSetKeyAge       = "age"
	DaemonSetKeyContext   = KeyContext // Context column; also used by the detail tab
	DaemonSetKeySelector  = "selector" // wide mode only
)

// Column keys for Ingresses rows.
const (
	IngressKeyName      = KeyName
	IngressKeyNamespace = KeyNamespace
	IngressKeyHosts     = "hosts"
	IngressKeyClass     = "class"
	IngressKeyAge       = "age"
	IngressKeyContext   = KeyContext // Context column; also used by the detail tab
	IngressKeyBackends  = "backends" // wide mode only, service:port list
)

// Column keys for PodDisruptionBudgets rows.
const (
	PDBKeyName               = KeyName
	PDBKeyNamespace          = KeyNamespace
	PDBKeyMinMaxAvailable    = "minMaxAvailable" // e.g. "minAvailable=2" or "maxUnavailable=25%"
	PDBKeyAllowedDisruptions = "allowedDisruptions"
	PDBKeyAge                = "age"
	PDBKeyContext            = KeyContext       // Context column; also used by the detail tab
	PDBKeyCurrentHealthy     = "currentHealthy" // wide mode only
	PDBKeyDesiredHealthy     = "desiredHealthy" // wide mode only
)

// Column keys for HorizontalPodAutoscalers rows.
const (
	HPAKeyName      = KeyName
	HPAKeyNamespace = KeyNamespace
	HPAKeyReference = "reference" // "kind/name" the HPA scales
	HPAKeyMinMax    = "minMax"    // "min-max" replica bounds
	HPAKeyReplicas  = "replicas"  // current replica count
	HPAKeyTargets   = "targets"   // best-effort "current%/target%" metric summary
	HPAKeyAge       = "age"
	HPAKeyContext   = KeyContext // Context column; also used by the detail tab
)

// Column keys for Nodes rows. Nodes are cluster-scoped: there is no
// KeyNamespace here, and Node rows are not deduplicated per namespace.
const (
	NodeKeyName       = KeyName
	NodeKeyStatus     = "status"
	NodeKeyRoles      = "roles"
	NodeKeyAge        = "age"
	NodeKeyVersion    = "version"
	NodeKeyContext    = KeyContext   // Context column; also used by the detail tab
	NodeKeyInternalIP = "internalIP" // wide mode only
	NodeKeyOS         = "os"         // wide mode only
	NodeKeyCPU        = "cpu"        // "-" until lazily fetched from metrics-server
	NodeKeyMemory     = "memory"     // "-" until lazily fetched from metrics-server
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

// ResourceUsage is one resource's current CPU/Memory usage, pre-formatted
// (e.g. "120m", "256Mi") by the metrics.k8s.io client — see
// k8s.PodMetricsInfo/NodeMetricsInfo.
type ResourceUsage struct {
	CPU    string
	Memory string
}

// PodMetricsMsg carries lazily, periodically-fetched CPU/Memory usage
// (namespace/name -> usage) for every pod in one context, or an error —
// see cmds.LoadPodMetricsCmd. Fetched only while the Pods tab is active
// (see MainPage.fetchMetricsIfNeeded), throttled well below the refresh
// tick since metrics-server itself only refreshes internally every ~60s.
type PodMetricsMsg struct {
	Context string
	Usage   map[string]ResourceUsage
	Err     error
}

// NodeMetricsMsg is PodMetricsMsg's Nodes counterpart — keyed by node name
// (Nodes are cluster-scoped, so no namespace qualifier is needed).
type NodeMetricsMsg struct {
	Context string
	Usage   map[string]ResourceUsage
	Err     error
}

// ContextsStateMsg reports how the context selection changed since the last
// time it was confirmed — Added and Deselected are already diffed against
// the previous confirm, so a consumer never needs to re-diff against its
// own prior snapshot to find out what's genuinely new.
type ContextsStateMsg struct {
	Added      []ContextsSelectedMsg
	Deselected []string // context names to remove
}

// NodesAccessMsg reports whether one context can watch Nodes, from a
// pre-flight SelfSubjectAccessReview check (see cmds.CheckNodesAccessCmd) —
// dispatched once per newly-selected context, before its Nodes watch would
// otherwise be opened.
type NodesAccessMsg struct {
	Context string
	Allowed bool
	Err     error
}

// NamespacesMsg carries every namespace available in one context, or an
// error (e.g. an RBAC denial), for the Namespaces pane. Fetched once per
// newly-selected context.
type NamespacesMsg struct {
	Context    string
	Namespaces []string
	Err        error
}

// NamespacesStateMsg reports how one context's checked namespaces changed
// since the last Namespaces-pane confirm — Added and Removed are already
// diffed, mirroring ContextsStateMsg's Added/Deselected shape. This is a
// display-only filter change now (see watch.Supervisor's stateKey doc
// comment) — it never starts or stops a watch. AllNamespaces is non-nil
// only when the context's all-namespaces mode changed (entered or left);
// nil means Added/Removed alone describe the change.
type NamespacesStateMsg struct {
	Context       string
	Added         []string
	Removed       []string
	AllNamespaces *bool
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

// ListLoadedMsg carries the result of a one-shot, cluster-wide List() call
// for one resource kind in one context, issued before that watch opens so
// the table can paint every row at once instead of waiting for the watch's
// synthetic Added replay to trickle in — see watch.Supervisor.start.
// Generation must match that (kind, context)'s current generation before the
// result is adopted; Err is non-nil on failure, in which case the watch is
// opened anyway and reports/handles the failure through its own path (see
// Supervisor.Handle). Objects is kind-erased (metav1.Object) since this
// message is kind-agnostic; the Supervisor's per-kind cache asserts it back
// to its concrete type when seeding.
type ListLoadedMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
	Objects    []metav1.Object
	Err        error
}

// WatchOpenedMsg carries a freshly opened, cluster-wide watch for one
// resource kind in one context. Generation must match that (kind,
// context)'s current generation in the watch Supervisor before the watch is
// adopted — otherwise it's been superseded (a manual "r" restart or context
// deselect) and should just be stopped.
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
