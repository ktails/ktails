// Package watch owns the full watch lifecycle for the three resource kinds:
// opening Watch() streams per selected context, applying events to local
// caches, reconnecting with exponential backoff, and rebuilding table rows.
// The Supervisor is the module's interface; everything else is
// implementation.
package watch

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// EndpointIPsPlaceholder is shown in the svc Endpoint IPs wide-mode column
// until the lazy endpoint fetch resolves for that context+namespace.
const EndpointIPsPlaceholder = "…"

// resourceVersionLess reports whether a is an older resourceVersion than b.
// Kubernetes resourceVersions are opaque strings but are numeric in every
// real implementation (etcd's mod-revision) — parsed numerically when
// possible, falling back to a string compare (never wrong, just potentially
// non-monotonic) if either side isn't a plain integer.
func resourceVersionLess(a, b string) bool {
	an, aerr := strconv.ParseInt(a, 10, 64)
	bn, berr := strconv.ParseInt(b, 10, 64)
	if aerr == nil && berr == nil {
		return an < bn
	}
	return a < b
}

// rowCache is the kind-erased face of resourceCache, letting the Supervisor
// hold Pod/Deployment/Service caches in one map.
type rowCache interface {
	apply(event watch.Event) error
	rows(kubeContext string) []msgs.RowData
}

// resourceCache is a local, per-context mirror of one resource kind's state
// kept in sync by a Watch() stream, avoiding a full re-List() on every
// refresh. Rows are rebuilt fresh from the stored raw objects on every rows()
// call — that's what keeps the Age column accurate without a second field to
// keep in sync.
type resourceCache[T metav1.Object] struct {
	mu    sync.Mutex
	byKey map[string]cacheEntry[T]
	toRow func(obj T, kubeContext string) msgs.RowData
}

type cacheEntry[T metav1.Object] struct {
	obj             T
	resourceVersion string
}

func newResourceCache[T metav1.Object](toRow func(T, string) msgs.RowData) *resourceCache[T] {
	return &resourceCache[T]{
		byKey: make(map[string]cacheEntry[T]),
		toRow: toRow,
	}
}

// apply updates the cache from one watch event. Returns a non-nil error only
// for a watch.Error event (the caller should treat that like a stream
// close/reconnect signal). Add/Modify upserts are guarded by resourceVersion
// so a fresh watch's full replay is idempotent and stale redeliveries are
// ignored.
func (c *resourceCache[T]) apply(event watch.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case watch.Error:
		return fmt.Errorf("watch error: %v", event.Object)
	case watch.Added, watch.Modified:
		obj, ok := event.Object.(T)
		if !ok {
			return nil
		}
		key := obj.GetNamespace() + "/" + obj.GetName()
		if existing, ok := c.byKey[key]; ok && !resourceVersionLess(existing.resourceVersion, obj.GetResourceVersion()) {
			return nil
		}
		c.byKey[key] = cacheEntry[T]{obj: obj, resourceVersion: obj.GetResourceVersion()}
	case watch.Deleted:
		obj, ok := event.Object.(T)
		if !ok {
			return nil
		}
		delete(c.byKey, obj.GetNamespace()+"/"+obj.GetName())
	}
	return nil
}

// rows rebuilds every row fresh from the stored raw objects, sorted by
// namespace/name for a stable table order.
func (c *resourceCache[T]) rows(kubeContext string) []msgs.RowData {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]msgs.RowData, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, c.toRow(c.byKey[key].obj, kubeContext))
	}
	return rows
}

// newCacheFor builds the cache for one resource kind, wiring in that kind's
// object→row conversion.
func newCacheFor(kind msgs.ResourceKind) rowCache {
	switch kind {
	case msgs.KindPods:
		return newResourceCache(podRow)
	case msgs.KindDeployments:
		return newResourceCache(deploymentRow)
	case msgs.KindServices:
		return newResourceCache(serviceRow)
	}
	return nil
}

func podRow(pod *corev1.Pod, kubeContext string) msgs.RowData {
	info := k8s.PodToPodInfo(pod, kubeContext)
	return msgs.RowData{
		msgs.PodKeyName:       info.Name,
		msgs.PodKeyNamespace:  info.Namespace,
		msgs.PodKeyStatus:     info.Status,
		msgs.PodKeyRestarts:   strconv.FormatInt(int64(info.Restarts), 10),
		msgs.PodKeyAge:        info.Age,
		msgs.PodKeyContext:    info.Context,
		msgs.PodKeyContainers: strings.Join(info.Containers, ","),
		msgs.PodKeyNode:       info.Node,
		msgs.PodKeyNodeIP:     info.NodeIP,
		msgs.PodKeyPodIP:      info.PodIP,
		msgs.PodKeyReady:      info.ReadyContainers,
	}
}

func deploymentRow(dep *appsv1.Deployment, kubeContext string) msgs.RowData {
	info := k8s.DeploymentToDeploymentInfo(dep)
	return msgs.RowData{
		msgs.DeployKeyName:      info.Name,
		msgs.DeployKeyAge:       info.Age,
		msgs.DeployKeyReplicas:  strconv.Itoa(int(info.ReadyReplicas)) + "/" + strconv.Itoa(int(info.DesiredReplicas)),
		msgs.DeployKeyContext:   kubeContext,
		msgs.DeployKeyNamespace: info.Namespace,
		msgs.DeployKeyStrategy:  info.Strategy,
		msgs.DeployKeyAvailable: strconv.FormatInt(int64(info.AvailableReplicas), 10),
		msgs.DeployKeyUpdated:   strconv.FormatInt(int64(info.UpdatedReplicas), 10),
		msgs.DeployKeySelector:  info.Selector,
	}
}

func serviceRow(svc *corev1.Service, kubeContext string) msgs.RowData {
	info := k8s.ServiceToServiceInfo(svc)
	return msgs.RowData{
		msgs.SvcKeyName:        info.Name,
		msgs.SvcKeyNamespace:   info.Namespace,
		msgs.SvcKeyType:        info.Type,
		msgs.SvcKeyClusterIP:   info.ClusterIP,
		msgs.SvcKeyPorts:       info.Ports,
		msgs.SvcKeyAge:         info.Age,
		msgs.SvcKeyContext:     kubeContext,
		msgs.SvcKeySelector:    info.Selector,
		msgs.SvcKeyExternalIP:  info.ExternalIP,
		msgs.SvcKeyEndpointIPs: EndpointIPsPlaceholder,
	}
}
