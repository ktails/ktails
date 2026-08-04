package k8s

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// DaemonSetInfo contains DaemonSet metadata.
type DaemonSetInfo struct {
	Name         string
	Namespace    string
	Age          string
	ReadyNodes   int32
	DesiredNodes int32
	Selector     string
}

// DaemonSetToDaemonSetInfo converts a DaemonSet object to DaemonSetInfo.
func DaemonSetToDaemonSetInfo(ds *appsv1.DaemonSet) DaemonSetInfo {
	return DaemonSetInfo{
		Name:         ds.Name,
		Namespace:    ds.Namespace,
		Age:          formatDuration(time.Since(ds.CreationTimestamp.Time)),
		ReadyNodes:   ds.Status.NumberReady,
		DesiredNodes: ds.Status.DesiredNumberScheduled,
		Selector:     v1.FormatLabelSelector(ds.Spec.Selector),
	}
}

// daemonSetsLW binds a DaemonSetInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func daemonSetsLW(namespace string) func(kubernetes.Interface) listerWatcher[*appsv1.DaemonSetList] {
	return func(cs kubernetes.Interface) listerWatcher[*appsv1.DaemonSetList] {
		return cs.AppsV1().DaemonSets(namespace)
	}
}

// WatchDaemonSets opens a watch on DaemonSets in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchDaemonSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "daemonsets in namespace "+namespace, daemonSetsLW(namespace))
}

// ListDaemonSets fetches every DaemonSet in the given namespace in one call.
// See ListPods for why this exists alongside the watch.
func (c *Client) ListDaemonSets(ctx context.Context, kubeContext, namespace string) ([]*appsv1.DaemonSet, error) {
	return listResource(ctx, c, kubeContext, "daemonsets in namespace "+namespace, daemonSetsLW(namespace),
		func(l *appsv1.DaemonSetList) []*appsv1.DaemonSet { return pointers(l.Items) })
}

// GetDaemonSetDetail fetches a single DaemonSet's status, rendered YAML,
// and recent events.
func (c *Client) GetDaemonSetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "DaemonSet"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get daemonset %s in namespace %s (context %s): %w",
			name, namespace, kubeContextName, err)
	}

	info := DaemonSetToDaemonSetInfo(ds)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Ready Nodes: %d/%d", info.ReadyNodes, info.DesiredNodes)
	for _, cond := range ds.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(cond.Type), string(cond.Status), cond.Reason, cond.Message))
	}
	d.YAML = renderDetailYAML(ds, "apps/v1", "DaemonSet")

	c.attachEvents(&d, kubeContextName, namespace, "DaemonSet", name)

	return d, nil
}
