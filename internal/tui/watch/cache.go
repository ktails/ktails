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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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
	case msgs.KindConfigMaps:
		return newResourceCache(configMapRow)
	case msgs.KindSecrets:
		return newResourceCache(secretRow)
	case msgs.KindJobs:
		return newResourceCache(jobRow)
	case msgs.KindCronJobs:
		return newResourceCache(cronJobRow)
	case msgs.KindStatefulSets:
		return newResourceCache(statefulSetRow)
	case msgs.KindDaemonSets:
		return newResourceCache(daemonSetRow)
	case msgs.KindIngresses:
		return newResourceCache(ingressRow)
	case msgs.KindNodes:
		return newResourceCache(nodeRow)
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

func configMapRow(cm *corev1.ConfigMap, kubeContext string) msgs.RowData {
	info := k8s.ConfigMapToConfigMapInfo(cm)
	return msgs.RowData{
		msgs.ConfigMapKeyName:      info.Name,
		msgs.ConfigMapKeyNamespace: info.Namespace,
		msgs.ConfigMapKeyKeys:      strconv.Itoa(len(info.Keys)),
		msgs.ConfigMapKeyAge:       info.Age,
		msgs.ConfigMapKeyContext:   kubeContext,
		msgs.ConfigMapKeyKeyNames:  strings.Join(info.Keys, ","),
	}
}

// secretRow never carries values — only key names and the count, matching
// k8s.SecretToSecretInfo (see redactedValue).
func secretRow(secret *corev1.Secret, kubeContext string) msgs.RowData {
	info := k8s.SecretToSecretInfo(secret)
	return msgs.RowData{
		msgs.SecretKeyName:      info.Name,
		msgs.SecretKeyNamespace: info.Namespace,
		msgs.SecretKeyType:      info.Type,
		msgs.SecretKeyKeys:      strconv.Itoa(len(info.Keys)),
		msgs.SecretKeyAge:       info.Age,
		msgs.SecretKeyContext:   kubeContext,
	}
}

func jobRow(job *batchv1.Job, kubeContext string) msgs.RowData {
	info := k8s.JobToJobInfo(job)
	return msgs.RowData{
		msgs.JobKeyName:        info.Name,
		msgs.JobKeyNamespace:   info.Namespace,
		msgs.JobKeyCompletions: info.Completions,
		msgs.JobKeyDuration:    info.Duration,
		msgs.JobKeyAge:         info.Age,
		msgs.JobKeyContext:     kubeContext,
		msgs.JobKeyStatus:      info.Status,
	}
}

func cronJobRow(cj *batchv1.CronJob, kubeContext string) msgs.RowData {
	info := k8s.CronJobToCronJobInfo(cj)
	return msgs.RowData{
		msgs.CronJobKeyName:          info.Name,
		msgs.CronJobKeyNamespace:     info.Namespace,
		msgs.CronJobKeySchedule:      info.Schedule,
		msgs.CronJobKeySuspend:       strconv.FormatBool(info.Suspend),
		msgs.CronJobKeyAge:           info.Age,
		msgs.CronJobKeyContext:       kubeContext,
		msgs.CronJobKeyLastScheduled: info.LastScheduled,
	}
}

func statefulSetRow(sts *appsv1.StatefulSet, kubeContext string) msgs.RowData {
	info := k8s.StatefulSetToStatefulSetInfo(sts)
	return msgs.RowData{
		msgs.StatefulSetKeyName:      info.Name,
		msgs.StatefulSetKeyNamespace: info.Namespace,
		msgs.StatefulSetKeyReady:     strconv.Itoa(int(info.ReadyReplicas)) + "/" + strconv.Itoa(int(info.DesiredReplicas)),
		msgs.StatefulSetKeyAge:       info.Age,
		msgs.StatefulSetKeyContext:   kubeContext,
		msgs.StatefulSetKeySelector:  info.Selector,
	}
}

func daemonSetRow(ds *appsv1.DaemonSet, kubeContext string) msgs.RowData {
	info := k8s.DaemonSetToDaemonSetInfo(ds)
	return msgs.RowData{
		msgs.DaemonSetKeyName:      info.Name,
		msgs.DaemonSetKeyNamespace: info.Namespace,
		msgs.DaemonSetKeyReady:     strconv.Itoa(int(info.ReadyNodes)) + "/" + strconv.Itoa(int(info.DesiredNodes)),
		msgs.DaemonSetKeyAge:       info.Age,
		msgs.DaemonSetKeyContext:   kubeContext,
		msgs.DaemonSetKeySelector:  info.Selector,
	}
}

func ingressRow(ing *networkingv1.Ingress, kubeContext string) msgs.RowData {
	info := k8s.IngressToIngressInfo(ing)
	return msgs.RowData{
		msgs.IngressKeyName:      info.Name,
		msgs.IngressKeyNamespace: info.Namespace,
		msgs.IngressKeyHosts:     strings.Join(info.Hosts, ","),
		msgs.IngressKeyClass:     info.Class,
		msgs.IngressKeyAge:       info.Age,
		msgs.IngressKeyContext:   kubeContext,
		msgs.IngressKeyBackends:  strings.Join(info.Backends, ","),
	}
}

// nodeRow is keyed only by kubeContext — Nodes are cluster-scoped, so there
// is no per-namespace row, and kubeContext also stands in for the (unused)
// Namespace column.
func nodeRow(node *corev1.Node, kubeContext string) msgs.RowData {
	info := k8s.NodeToNodeInfo(node)
	return msgs.RowData{
		msgs.NodeKeyName:       info.Name,
		msgs.NodeKeyStatus:     info.Status,
		msgs.NodeKeyRoles:      info.Roles,
		msgs.NodeKeyAge:        info.Age,
		msgs.NodeKeyVersion:    info.Version,
		msgs.NodeKeyContext:    kubeContext,
		msgs.NodeKeyInternalIP: info.InternalIP,
		msgs.NodeKeyOS:         info.OS,
	}
}
