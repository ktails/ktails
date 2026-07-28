package k8s

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/yaml"
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

// WatchDaemonSets opens a watch on DaemonSets in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchDaemonSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.AppsV1().DaemonSets(namespace).Watch(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch daemonsets in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// ListDaemonSets fetches every DaemonSet in the given namespace in one call.
// See ListPods for why this exists alongside the watch.
func (c *Client) ListDaemonSets(ctx context.Context, kubeContext, namespace string) ([]*appsv1.DaemonSet, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	list, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list daemonsets in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	out := make([]*appsv1.DaemonSet, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

// GetDaemonSetDetail fetches a single DaemonSet's status, rendered YAML,
// and recent events.
func (c *Client) GetDaemonSetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "DaemonSet"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), name, v1.GetOptions{})
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

	ds.ManagedFields = nil
	ds.TypeMeta = v1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSet"}
	if yamlBytes, yamlErr := yaml.Marshal(ds); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "DaemonSet", name); err == nil {
		d.Events = events
	}

	return d, nil
}
