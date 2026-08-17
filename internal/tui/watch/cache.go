// Package watch owns the full watch lifecycle for every resource kind:
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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// EndpointIPsPlaceholder is shown in the svc Endpoint IPs wide-mode column
// until the lazy endpoint fetch resolves for that context+namespace.
const EndpointIPsPlaceholder = "…"

// MetricsPlaceholder is shown in the Pods/Nodes CPU/Memory wide-mode columns
// until the periodic metrics-server fetch resolves for that context — or
// permanently, if that context's cluster has no metrics-server installed
// (see Supervisor.PodMetricsUnavailable/NodeMetricsUnavailable).
const MetricsPlaceholder = "-"

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
	rows(kubeContext string) []msgs.Row
	seed(objs []metav1.Object)
}

// resourceCache is a local, per-context mirror of one resource kind's state
// kept in sync by a Watch() stream, avoiding a full re-List() on every
// refresh. Rows are rebuilt fresh from the stored raw objects on every rows()
// call — that's what keeps the Age column accurate without a second field to
// keep in sync.
type resourceCache[T metav1.Object] struct {
	mu    sync.Mutex
	byKey map[string]cacheEntry[T]
	toRow func(obj T, kubeContext string) msgs.Row
	trim  func(T) T // applied to every object before storing; nil = store as-is
}

type cacheEntry[T metav1.Object] struct {
	obj             T
	resourceVersion string
}

// trimManagedFields drops managedFields — often ~half of an object's
// encoded size, and never read from the cache (only from the separate
// detail-view fetch, which strips it independently) — from every cached
// kind, mirroring client-go informers' default TransformFunc use.
func trimManagedFields[T metav1.Object](obj T) T {
	obj.SetManagedFields(nil)
	return obj
}

// newResourceCache builds a cache with only the default managedFields trim.
func newResourceCache[T metav1.Object](toRow func(T, string) msgs.Row) *resourceCache[T] {
	return newTrimmedCache(toRow, trimManagedFields[T])
}

// newTrimmedCache builds a cache whose trim composes the default
// managedFields strip with a kind-specific reduction (e.g. trimSecret).
func newTrimmedCache[T metav1.Object](toRow func(T, string) msgs.Row, trim func(T) T) *resourceCache[T] {
	return &resourceCache[T]{
		byKey: make(map[string]cacheEntry[T]),
		toRow: toRow,
		trim:  trim,
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
		if status, ok := event.Object.(*metav1.Status); ok {
			// Preserve the typed API status so apierrors.IsForbidden /
			// IsResourceExpired checks upstream (Supervisor.Handle) still work.
			return &apierrors.StatusError{ErrStatus: *status}
		}
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
		rv := obj.GetResourceVersion()
		if c.trim != nil {
			obj = c.trim(obj)
		}
		c.byKey[key] = cacheEntry[T]{obj: obj, resourceVersion: rv}
	case watch.Deleted:
		obj, ok := event.Object.(T)
		if !ok {
			return nil
		}
		delete(c.byKey, obj.GetNamespace()+"/"+obj.GetName())
	}
	return nil
}

// seed replaces the cache's entire contents with the result of a List()
// call — a full reset rather than an upsert-merge, since (unlike a watch
// event) a List result is a complete, authoritative snapshot: merging would
// leave behind entries for objects deleted since the cache was last
// populated. Objects that don't assert to T are skipped (kind-erased
// ListLoadedMsg carries metav1.Object; every element is expected to be T in
// practice, since a cache is only ever seeded from its own kind's List
// result).
func (c *resourceCache[T]) seed(objs []metav1.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()

	byKey := make(map[string]cacheEntry[T], len(objs))
	for _, o := range objs {
		obj, ok := o.(T)
		if !ok {
			continue
		}
		key := obj.GetNamespace() + "/" + obj.GetName()
		rv := obj.GetResourceVersion()
		if c.trim != nil {
			obj = c.trim(obj)
		}
		byKey[key] = cacheEntry[T]{obj: obj, resourceVersion: rv}
	}
	c.byKey = byKey
}

// rows rebuilds every row fresh from the stored raw objects, sorted by
// namespace/name for a stable table order.
func (c *resourceCache[T]) rows(kubeContext string) []msgs.Row {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]msgs.Row, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, c.toRow(c.byKey[key].obj, kubeContext))
	}
	return rows
}

// newCacheFor builds the cache for one resource kind, wiring in that kind's
// object→row conversion, via kindSpecs (kindspec.go) — see its doc comment.
func newCacheFor(kind msgs.ResourceKind) rowCache {
	spec, ok := kindSpecs[kind]
	if !ok {
		return nil
	}
	return spec.newCache()
}

func podRow(pod *corev1.Pod, kubeContext string) msgs.Row {
	info := k8s.PodToPodInfo(pod, kubeContext)
	return msgs.Row{
		Name:       info.Name,
		Namespace:  info.Namespace,
		Context:    info.Context,
		CreatedAt:  pod.GetCreationTimestamp().Time,
		Containers: info.Containers,
		Cells: map[string]string{
			msgs.PodKeyStatus:   info.Status,
			msgs.PodKeyRestarts: strconv.FormatInt(int64(info.Restarts), 10),
			msgs.PodKeyAge:      info.Age,
			msgs.PodKeyNode:     info.Node,
			msgs.PodKeyNodeIP:   info.NodeIP,
			msgs.PodKeyPodIP:    info.PodIP,
			msgs.PodKeyReady:    info.ReadyContainers,
			msgs.PodKeyCPU:      MetricsPlaceholder,
			msgs.PodKeyMemory:   MetricsPlaceholder,
		},
	}
}

func deploymentRow(dep *appsv1.Deployment, kubeContext string) msgs.Row {
	info := k8s.DeploymentToDeploymentInfo(dep)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: dep.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.DeployKeyAge:       info.Age,
			msgs.DeployKeyReplicas:  strconv.Itoa(int(info.ReadyReplicas)) + "/" + strconv.Itoa(int(info.DesiredReplicas)),
			msgs.DeployKeyStrategy:  info.Strategy,
			msgs.DeployKeyAvailable: strconv.FormatInt(int64(info.AvailableReplicas), 10),
			msgs.DeployKeyUpdated:   strconv.FormatInt(int64(info.UpdatedReplicas), 10),
			msgs.DeployKeySelector:  info.Selector,
		},
	}
}

func serviceRow(svc *corev1.Service, kubeContext string) msgs.Row {
	info := k8s.ServiceToServiceInfo(svc)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: svc.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.SvcKeyType:        info.Type,
			msgs.SvcKeyClusterIP:   info.ClusterIP,
			msgs.SvcKeyPorts:       info.Ports,
			msgs.SvcKeyAge:         info.Age,
			msgs.SvcKeySelector:    info.Selector,
			msgs.SvcKeyExternalIP:  info.ExternalIP,
			msgs.SvcKeyEndpointIPs: EndpointIPsPlaceholder,
		},
	}
}

// trimConfigMap composes with trimManagedFields to drop cached ConfigMap
// values while keeping key names, since configMapRow / ConfigMapToConfigMapInfo
// only ever read key names and counts.
func trimConfigMap(cm *corev1.ConfigMap) *corev1.ConfigMap {
	cm = trimManagedFields(cm)
	for k := range cm.Data {
		cm.Data[k] = ""
	}
	cm.BinaryData = nil
	return cm
}

func configMapRow(cm *corev1.ConfigMap, kubeContext string) msgs.Row {
	info := k8s.ConfigMapToConfigMapInfo(cm)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: cm.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.ConfigMapKeyKeys:     strconv.Itoa(len(info.Keys)),
			msgs.ConfigMapKeyAge:      info.Age,
			msgs.ConfigMapKeyKeyNames: strings.Join(info.Keys, ","),
		},
	}
}

// trimSecret composes with trimManagedFields to drop cached Secret values —
// entire clusters' worth of secret material otherwise sits in memory for
// every watched context — while keeping key names, since secretRow /
// k8s.SecretToSecretInfo only ever read key names and counts.
func trimSecret(s *corev1.Secret) *corev1.Secret {
	s = trimManagedFields(s)
	for k := range s.Data {
		s.Data[k] = nil
	}
	s.StringData = nil
	return s
}

// secretRow never carries values — only key names and the count, matching
// k8s.SecretToSecretInfo (see redactedValue).
func secretRow(secret *corev1.Secret, kubeContext string) msgs.Row {
	info := k8s.SecretToSecretInfo(secret)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: secret.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.SecretKeyType: info.Type,
			msgs.SecretKeyKeys: strconv.Itoa(len(info.Keys)),
			msgs.SecretKeyAge:  info.Age,
		},
	}
}

func jobRow(job *batchv1.Job, kubeContext string) msgs.Row {
	info := k8s.JobToJobInfo(job)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: job.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.JobKeyCompletions: info.Completions,
			msgs.JobKeyDuration:    info.Duration,
			msgs.JobKeyAge:         info.Age,
			msgs.JobKeyStatus:      info.Status,
		},
	}
}

func cronJobRow(cj *batchv1.CronJob, kubeContext string) msgs.Row {
	info := k8s.CronJobToCronJobInfo(cj)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: cj.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.CronJobKeySchedule:      info.Schedule,
			msgs.CronJobKeySuspend:       strconv.FormatBool(info.Suspend),
			msgs.CronJobKeyAge:           info.Age,
			msgs.CronJobKeyLastScheduled: info.LastScheduled,
		},
	}
}

func statefulSetRow(sts *appsv1.StatefulSet, kubeContext string) msgs.Row {
	info := k8s.StatefulSetToStatefulSetInfo(sts)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: sts.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.StatefulSetKeyReady:    strconv.Itoa(int(info.ReadyReplicas)) + "/" + strconv.Itoa(int(info.DesiredReplicas)),
			msgs.StatefulSetKeyAge:      info.Age,
			msgs.StatefulSetKeySelector: info.Selector,
		},
	}
}

func daemonSetRow(ds *appsv1.DaemonSet, kubeContext string) msgs.Row {
	info := k8s.DaemonSetToDaemonSetInfo(ds)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: ds.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.DaemonSetKeyReady:    strconv.Itoa(int(info.ReadyNodes)) + "/" + strconv.Itoa(int(info.DesiredNodes)),
			msgs.DaemonSetKeyAge:      info.Age,
			msgs.DaemonSetKeySelector: info.Selector,
		},
	}
}

func ingressRow(ing *networkingv1.Ingress, kubeContext string) msgs.Row {
	info := k8s.IngressToIngressInfo(ing)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: ing.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.IngressKeyHosts:    strings.Join(info.Hosts, ","),
			msgs.IngressKeyClass:    info.Class,
			msgs.IngressKeyAge:      info.Age,
			msgs.IngressKeyBackends: strings.Join(info.Backends, ","),
		},
	}
}

func pdbRow(pdb *policyv1.PodDisruptionBudget, kubeContext string) msgs.Row {
	info := k8s.PodDisruptionBudgetToPodDisruptionBudgetInfo(pdb)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: pdb.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.PDBKeyMinMaxAvailable:    info.MinMaxAvailable,
			msgs.PDBKeyAllowedDisruptions: strconv.FormatInt(int64(info.AllowedDisruptions), 10),
			msgs.PDBKeyAge:                info.Age,
			msgs.PDBKeyCurrentHealthy:     strconv.FormatInt(int64(info.CurrentHealthy), 10),
			msgs.PDBKeyDesiredHealthy:     strconv.FormatInt(int64(info.DesiredHealthy), 10),
		},
	}
}

func hpaRow(hpa *autoscalingv2.HorizontalPodAutoscaler, kubeContext string) msgs.Row {
	info := k8s.HorizontalPodAutoscalerToHorizontalPodAutoscalerInfo(hpa)
	return msgs.Row{
		Name:      info.Name,
		Namespace: info.Namespace,
		Context:   kubeContext,
		CreatedAt: hpa.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.HPAKeyReference: info.Reference,
			msgs.HPAKeyMinMax:    strconv.Itoa(int(info.MinReplicas)) + "-" + strconv.Itoa(int(info.MaxReplicas)),
			msgs.HPAKeyReplicas:  strconv.Itoa(int(info.CurrentReplicas)),
			msgs.HPAKeyTargets:   info.Targets,
			msgs.HPAKeyAge:       info.Age,
		},
	}
}

// nodeRow is keyed only by kubeContext — Nodes are cluster-scoped, so there
// is no per-namespace row, and kubeContext also stands in for the (unused)
// Namespace column.
func nodeRow(node *corev1.Node, kubeContext string) msgs.Row {
	info := k8s.NodeToNodeInfo(node)
	return msgs.Row{
		Name:      info.Name,
		Context:   kubeContext,
		CreatedAt: node.GetCreationTimestamp().Time,
		Cells: map[string]string{
			msgs.NodeKeyStatus:     info.Status,
			msgs.NodeKeyRoles:      info.Roles,
			msgs.NodeKeyAge:        info.Age,
			msgs.NodeKeyVersion:    info.Version,
			msgs.NodeKeyInternalIP: info.InternalIP,
			msgs.NodeKeyOS:         info.OS,
			msgs.NodeKeyCPU:        MetricsPlaceholder,
			msgs.NodeKeyMemory:     MetricsPlaceholder,
		},
	}
}
