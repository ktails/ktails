package watch

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// kindSpec bundles everything the Supervisor needs to drive one resource
// kind generically: its Watch/List Cluster methods (list already adapted
// to the kind-erased []metav1.Object shape listCmd needs, via toObjects)
// and its cache constructor. One kindSpecs entry per msgs.ResourceKind
// replaces three parallel 13-arm switches (watchFn, listFn, and cache.go's
// newCacheFor) that all keyed on the same enum — adding a 14th kind means
// adding one map entry here instead of touching three switches. msgs.
// Title()/msgs.Kind() stay switches on the enum itself (they belong there,
// not here), and MainPage's detail-cmd dispatch is a separate concern this
// doesn't touch.
type kindSpec struct {
	watch    func(c Cluster) func(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	list     func(c Cluster) func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error)
	newCache func() rowCache
}

var kindSpecs = map[msgs.ResourceKind]kindSpec{
	msgs.KindPods: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchPods },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListPods(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(podRow) },
	},
	msgs.KindDeployments: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchDeployments
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListDeployments(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(deploymentRow) },
	},
	msgs.KindServices: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchServices },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListServices(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(serviceRow) },
	},
	msgs.KindConfigMaps: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchConfigMaps
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListConfigMaps(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(configMapRow) },
	},
	msgs.KindSecrets: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchSecrets },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListSecrets(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(secretRow) },
	},
	msgs.KindJobs: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchJobs },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListJobs(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(jobRow) },
	},
	msgs.KindCronJobs: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchCronJobs },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListCronJobs(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(cronJobRow) },
	},
	msgs.KindStatefulSets: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchStatefulSets
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListStatefulSets(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(statefulSetRow) },
	},
	msgs.KindDaemonSets: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchDaemonSets
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListDaemonSets(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(daemonSetRow) },
	},
	msgs.KindIngresses: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchIngresses
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListIngresses(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(ingressRow) },
	},
	msgs.KindPodDisruptionBudgets: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchPodDisruptionBudgets
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListPodDisruptionBudgets(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(pdbRow) },
	},
	msgs.KindHorizontalPodAutoscalers: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) {
			return c.WatchHorizontalPodAutoscalers
		},
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListHorizontalPodAutoscalers(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(hpaRow) },
	},
	msgs.KindNodes: {
		watch: func(c Cluster) func(context.Context, string, string) (watch.Interface, error) { return c.WatchNodes },
		list: func(c Cluster) func(context.Context, string, string) ([]metav1.Object, error) {
			return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
				items, err := c.ListNodes(ctx, kubeContext, namespace)
				return toObjects(items), err
			}
		},
		newCache: func() rowCache { return newResourceCache(nodeRow) },
	},
}
