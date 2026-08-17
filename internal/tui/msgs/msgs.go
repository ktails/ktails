// Package msgs holds tea.Msg messages for routing k8s info to pages
package msgs

import (
	"io"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/kinds"
)

// ResourceKind identifies one of the watched resource types. It is the
// single tab/watch/cache identity used everywhere a "Deployments" | "Pods" |
// "svc" string switch used to live. It's a type alias (not a new type) to
// kinds.ResourceKind: k8s.Client's Watch/List methods (internal/k8s/
// registry.go) key their kind registry by kinds.ResourceKind, and msgs
// already imports k8s (for k8s.ResourceDetail in ResourceDetailMsg below),
// so k8s can't import msgs back without a cycle — kinds is the shared leaf
// package both depend on instead. The alias means every existing
// msgs.ResourceKind/msgs.KindXxx/msgs.Kinds() reference is unaffected.
type ResourceKind = kinds.ResourceKind

const (
	KindDeployments              = kinds.KindDeployments
	KindPods                     = kinds.KindPods
	KindServices                 = kinds.KindServices
	KindConfigMaps               = kinds.KindConfigMaps
	KindSecrets                  = kinds.KindSecrets
	KindJobs                     = kinds.KindJobs
	KindCronJobs                 = kinds.KindCronJobs
	KindStatefulSets             = kinds.KindStatefulSets
	KindDaemonSets               = kinds.KindDaemonSets
	KindIngresses                = kinds.KindIngresses
	KindPodDisruptionBudgets     = kinds.KindPodDisruptionBudgets
	KindHorizontalPodAutoscalers = kinds.KindHorizontalPodAutoscalers
	// KindNodes is cluster-scoped, not namespaced — its rows carry no
	// Namespace, and its watch ignores the namespace argument.
	KindNodes = kinds.KindNodes
)

// Kinds returns every ResourceKind in tab order.
func Kinds() []ResourceKind {
	return kinds.Kinds()
}

// Row is one table row's field values, for the Pods/Deployments/.../Nodes
// tables. Name/Namespace/Context/CreatedAt are promoted to typed fields
// since every kind carries them identically (kind-agnostic callers — the
// Detail Pane's row->identity extraction, sorting, namespace filtering —
// read a row without switching on its kind); Containers is Pods-only, read
// directly by the Log pane instead of splitting a comma-joined string.
// Cells holds every remaining, per-kind-specific display column, keyed by
// the same XxxKeyY constants a table.Column's key matches — a value here is
// displayed only if some column for that kind actually uses its key; others
// ride along as hidden metadata (e.g. wide-mode-only fields) for the Detail
// pane to read without a visible column of their own.
type Row struct {
	Name, Namespace, Context string
	CreatedAt                time.Time
	Containers               []string
	Cells                    map[string]string
}

// Shared row keys — deliberately identical across every resource kind, so
// kind-agnostic callers (the Detail Pane's row→identity extraction) can
// read a selected row without switching on its kind. Namespace/Name/Context
// back Row's typed fields directly rather than a Cells entry — see Row's
// doc comment — but stay defined here since every kind's own KeyX alias
// (e.g. PodKeyNamespace) is still a real btable.Column key, just no longer
// a RowData map key.
const (
	KeyName      = "name"
	KeyNamespace = "namespace"
	KeyContext   = "context"
)

// Column keys for Pods rows (see watch.Supervisor / models.ResourceTable).
// Containers has no column key — it's Row.Containers, a typed field (see
// Row's doc comment), read directly by the Log pane.
const (
	PodKeyCheck     = "check"
	PodKeyName      = KeyName
	PodKeyNamespace = KeyNamespace
	PodKeyStatus    = "status"
	PodKeyRestarts  = "restarts"
	PodKeyAge       = "age"
	PodKeyContext   = KeyContext // Context column; also used by the detail tab
	PodKeyNode      = "node"     // wide mode only
	PodKeyNodeIP    = "nodeIP"   // wide mode only
	PodKeyPodIP     = "podIP"    // wide mode only
	PodKeyReady     = "ready"    // wide mode only, "ready/total" containers
	PodKeyCPU       = "cpu"      // "-" until lazily fetched from metrics-server
	PodKeyMemory    = "memory"   // "-" until lazily fetched from metrics-server
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

// WorkloadPodsMsg carries the result of resolving one workload row's owned
// Pods (see k8s.Client.ResolveWorkloadPods), for MainPage's "l" key on a
// HasPods() kind — the aggregate-log counterpart to opening logs from the
// Pods tab directly. PodNames are looked up against the already-loaded
// Pods-tab cache (for their containers) rather than fetched as full Pod
// objects here.
type WorkloadPodsMsg struct {
	Context, Namespace, Name string
	Kind                     ResourceKind
	PodNames                 []string
	Err                      error
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
// to its concrete type when seeding. ResourceVersion is the list's own
// collection RV, threaded into the subsequent Watch() call so the server
// starts streaming from there instead of replaying every listed object as a
// synthetic Added event.
type ListLoadedMsg struct {
	Kind            ResourceKind
	Context         string
	Generation      int
	Objects         []metav1.Object
	ResourceVersion string
	Err             error
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

// WatchEventMsg reports that one (kind, context) watch cache changed after
// applying one or more buffered watch events. It deliberately carries no row
// data — callers rebuild rows via Supervisor.Rows(kind) on the resulting
// WatchFlushMsg, the single source of truth for row conversion (a second
// copy computed here was previously discarded unread by everything outside
// tests, paying for row conversion twice per event).
type WatchEventMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
}

// WatchFlushMsg fires after a short coalescing delay to convert a burst of
// WatchEventMsgs into a single row/UI rebuild — client-go's watch channel is
// unbuffered, so a rollout or mass change arrives as many individual events,
// each of which would otherwise trigger its own full Rows/filter/sort/render
// cycle.
type WatchFlushMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
}

// WatchClosedMsg reports that one (kind, context) watch ended, either
// cleanly (Err == nil) or because opening/reading it failed (Err != nil).
type WatchClosedMsg struct {
	Kind       ResourceKind
	Context    string
	Generation int
	Err        error
}
