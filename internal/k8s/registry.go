package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/kinds"
)

// kindBinding adapts one resource kind's concretely-typed
// Watch<Kind>/List<Kind> methods into the kind-erased shape Client.Watch/
// Client.List need — the only place this package's 13 kinds' worth of
// per-kind methods collapse into a single dispatch table. Adding a 14th
// kind means adding one entry here (plus its own Watch<Kind>/List<Kind> and
// row conversion) rather than widening a public interface every consumer
// has to implement in full.
type kindBinding struct {
	watch func(ctx context.Context, c *Client, kubeContext, namespace, resourceVersion string) (watch.Interface, error)
	list  func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error)
}

var kindRegistry = map[kinds.ResourceKind]kindBinding{
	kinds.KindPods: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchPods(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListPods(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindDeployments: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchDeployments(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListDeployments(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindServices: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchServices(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListServices(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindConfigMaps: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchConfigMaps(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListConfigMaps(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindSecrets: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchSecrets(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListSecrets(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindJobs: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchJobs(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListJobs(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindCronJobs: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchCronJobs(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListCronJobs(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindStatefulSets: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchStatefulSets(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListStatefulSets(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindDaemonSets: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchDaemonSets(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListDaemonSets(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindIngresses: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchIngresses(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListIngresses(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindPodDisruptionBudgets: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchPodDisruptionBudgets(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListPodDisruptionBudgets(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindHorizontalPodAutoscalers: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchHorizontalPodAutoscalers(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListHorizontalPodAutoscalers(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
	kinds.KindNodes: {
		watch: func(ctx context.Context, c *Client, kubeContext, namespace, rv string) (watch.Interface, error) {
			return c.WatchNodes(ctx, kubeContext, namespace, rv)
		},
		list: func(ctx context.Context, c *Client, kubeContext, namespace string) ([]metav1.Object, string, error) {
			items, rv, err := c.ListNodes(ctx, kubeContext, namespace)
			return toObjects(items), rv, err
		},
	},
}

// Watch opens a watch for kind, scoped to namespace ("" for cluster-wide)
// and resourceVersion ("" for a bare open — see WatchPods' doc comment) —
// the narrow consumer-side seam watch.Supervisor depends on (as
// watch.Cluster) instead of one Watch<Kind> method per kind.
func (c *Client) Watch(ctx context.Context, kind kinds.ResourceKind, kubeContext, namespace, resourceVersion string) (watch.Interface, error) {
	b, ok := kindRegistry[kind]
	if !ok {
		return nil, fmt.Errorf("k8s: unknown resource kind %v", kind)
	}
	return b.watch(ctx, c, kubeContext, namespace, resourceVersion)
}

// List fetches every object of kind in namespace ("" for cluster-wide) in
// one call, kind-erased to []metav1.Object, along with the list's
// resourceVersion for a subsequent Watch call — List's own Cluster-seam
// counterpart to Watch.
func (c *Client) List(ctx context.Context, kind kinds.ResourceKind, kubeContext, namespace string) ([]metav1.Object, string, error) {
	b, ok := kindRegistry[kind]
	if !ok {
		return nil, "", fmt.Errorf("k8s: unknown resource kind %v", kind)
	}
	return b.list(ctx, c, kubeContext, namespace)
}
