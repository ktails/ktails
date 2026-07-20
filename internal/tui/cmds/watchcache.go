package cmds

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

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

// PodWatchCache is a local, per-context+namespace mirror of pod state kept
// in sync by a Watch() stream, avoiding a full re-List() on every refresh.
type PodWatchCache struct {
	mu    sync.Mutex
	byKey map[string]podCacheEntry
}

type podCacheEntry struct {
	pod             *corev1.Pod
	resourceVersion string
}

func NewPodWatchCache() *PodWatchCache {
	return &PodWatchCache{byKey: make(map[string]podCacheEntry)}
}

// apply updates the cache from one watch event. Returns a non-nil error only
// for a watch.Error event (the caller should treat that like a stream
// close/reconnect signal).
func (c *PodWatchCache) apply(event watch.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case watch.Error:
		return fmt.Errorf("watch error: %v", event.Object)
	case watch.Added, watch.Modified:
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			return nil
		}
		key := pod.Namespace + "/" + pod.Name
		if existing, ok := c.byKey[key]; ok && !resourceVersionLess(existing.resourceVersion, pod.ResourceVersion) {
			return nil
		}
		c.byKey[key] = podCacheEntry{pod: pod, resourceVersion: pod.ResourceVersion}
	case watch.Deleted:
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			return nil
		}
		delete(c.byKey, pod.Namespace+"/"+pod.Name)
	}
	return nil
}

// rows rebuilds every row fresh from the stored raw objects — this is what
// keeps the Age column accurate on every call without a second field to
// keep in sync.
func (c *PodWatchCache) Rows(kubeContext string) []msgs.RowData {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]msgs.RowData, 0, len(keys))
	for _, key := range keys {
		pod := k8s.PodToPodInfo(c.byKey[key].pod, kubeContext)
		rows = append(rows, msgs.RowData{
			msgs.PodKeyName:       pod.Name,
			msgs.PodKeyNamespace:  pod.Namespace,
			msgs.PodKeyStatus:     pod.Status,
			msgs.PodKeyRestarts:   strconv.FormatInt(int64(pod.Restarts), 10),
			msgs.PodKeyAge:        pod.Age,
			msgs.PodKeyContext:    pod.Context,
			msgs.PodKeyContainers: strings.Join(pod.Containers, ","),
			msgs.PodKeyNode:       pod.Node,
			msgs.PodKeyNodeIP:     pod.NodeIP,
			msgs.PodKeyPodIP:      pod.PodIP,
			msgs.PodKeyReady:      pod.ReadyContainers,
		})
	}
	return rows
}

// DeploymentWatchCache mirrors PodWatchCache for Deployments.
type DeploymentWatchCache struct {
	mu    sync.Mutex
	byKey map[string]deploymentCacheEntry
}

type deploymentCacheEntry struct {
	deployment      *appsv1.Deployment
	resourceVersion string
}

func NewDeploymentWatchCache() *DeploymentWatchCache {
	return &DeploymentWatchCache{byKey: make(map[string]deploymentCacheEntry)}
}

func (c *DeploymentWatchCache) apply(event watch.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case watch.Error:
		return fmt.Errorf("watch error: %v", event.Object)
	case watch.Added, watch.Modified:
		dep, ok := event.Object.(*appsv1.Deployment)
		if !ok {
			return nil
		}
		key := dep.Namespace + "/" + dep.Name
		if existing, ok := c.byKey[key]; ok && !resourceVersionLess(existing.resourceVersion, dep.ResourceVersion) {
			return nil
		}
		c.byKey[key] = deploymentCacheEntry{deployment: dep, resourceVersion: dep.ResourceVersion}
	case watch.Deleted:
		dep, ok := event.Object.(*appsv1.Deployment)
		if !ok {
			return nil
		}
		delete(c.byKey, dep.Namespace+"/"+dep.Name)
	}
	return nil
}

func (c *DeploymentWatchCache) Rows(kubeContext string) []msgs.RowData {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]msgs.RowData, 0, len(keys))
	for _, key := range keys {
		deployment := k8s.DeploymentToDeploymentInfo(c.byKey[key].deployment)
		rows = append(rows, msgs.RowData{
			msgs.DeployKeyName:      deployment.Name,
			msgs.DeployKeyAge:       deployment.Age,
			msgs.DeployKeyReplicas:  strconv.Itoa(int(deployment.ReadyReplicas)) + "/" + strconv.Itoa(int(deployment.DesiredReplicas)),
			msgs.DeployKeyContext:   kubeContext,
			msgs.DeployKeyNamespace: deployment.Namespace,
			msgs.DeployKeyStrategy:  deployment.Strategy,
			msgs.DeployKeyAvailable: strconv.FormatInt(int64(deployment.AvailableReplicas), 10),
			msgs.DeployKeyUpdated:   strconv.FormatInt(int64(deployment.UpdatedReplicas), 10),
			msgs.DeployKeySelector:  deployment.Selector,
		})
	}
	return rows
}

// ServiceWatchCache mirrors PodWatchCache for Services.
type ServiceWatchCache struct {
	mu    sync.Mutex
	byKey map[string]serviceCacheEntry
}

type serviceCacheEntry struct {
	service         *corev1.Service
	resourceVersion string
}

func NewServiceWatchCache() *ServiceWatchCache {
	return &ServiceWatchCache{byKey: make(map[string]serviceCacheEntry)}
}

func (c *ServiceWatchCache) apply(event watch.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case watch.Error:
		return fmt.Errorf("watch error: %v", event.Object)
	case watch.Added, watch.Modified:
		svc, ok := event.Object.(*corev1.Service)
		if !ok {
			return nil
		}
		key := svc.Namespace + "/" + svc.Name
		if existing, ok := c.byKey[key]; ok && !resourceVersionLess(existing.resourceVersion, svc.ResourceVersion) {
			return nil
		}
		c.byKey[key] = serviceCacheEntry{service: svc, resourceVersion: svc.ResourceVersion}
	case watch.Deleted:
		svc, ok := event.Object.(*corev1.Service)
		if !ok {
			return nil
		}
		delete(c.byKey, svc.Namespace+"/"+svc.Name)
	}
	return nil
}

func (c *ServiceWatchCache) Rows(kubeContext string) []msgs.RowData {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]msgs.RowData, 0, len(keys))
	for _, key := range keys {
		svc := k8s.ServiceToServiceInfo(c.byKey[key].service)
		rows = append(rows, msgs.RowData{
			msgs.SvcKeyName:        svc.Name,
			msgs.SvcKeyNamespace:   svc.Namespace,
			msgs.SvcKeyType:        svc.Type,
			msgs.SvcKeyClusterIP:   svc.ClusterIP,
			msgs.SvcKeyPorts:       svc.Ports,
			msgs.SvcKeyAge:         svc.Age,
			msgs.SvcKeyContext:     kubeContext,
			msgs.SvcKeySelector:    svc.Selector,
			msgs.SvcKeyExternalIP:  svc.ExternalIP,
			msgs.SvcKeyEndpointIPs: endpointIPsPlaceholder,
		})
	}
	return rows
}
