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

// StatefulSetInfo contains StatefulSet metadata.
type StatefulSetInfo struct {
	Name            string
	Namespace       string
	Age             string
	ReadyReplicas   int32
	DesiredReplicas int32
	Selector        string
}

// StatefulSetToStatefulSetInfo converts a StatefulSet object to StatefulSetInfo.
func StatefulSetToStatefulSetInfo(sts *appsv1.StatefulSet) StatefulSetInfo {
	desiredReplicas := int32(1)
	if sts.Spec.Replicas != nil {
		desiredReplicas = *sts.Spec.Replicas
	}

	return StatefulSetInfo{
		Name:            sts.Name,
		Namespace:       sts.Namespace,
		Age:             formatDuration(time.Since(sts.CreationTimestamp.Time)),
		ReadyReplicas:   sts.Status.ReadyReplicas,
		DesiredReplicas: desiredReplicas,
		Selector:        v1.FormatLabelSelector(sts.Spec.Selector),
	}
}

// statefulSetsLW binds a StatefulSetInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func statefulSetsLW(namespace string) func(kubernetes.Interface) listerWatcher[*appsv1.StatefulSetList] {
	return func(cs kubernetes.Interface) listerWatcher[*appsv1.StatefulSetList] {
		return cs.AppsV1().StatefulSets(namespace)
	}
}

// WatchStatefulSets opens a watch on StatefulSets in the given namespace.
// See WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchStatefulSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "statefulsets in namespace "+namespace, statefulSetsLW(namespace))
}

// ListStatefulSets fetches every StatefulSet in the given namespace in one
// call. See ListPods for why this exists alongside the watch.
func (c *Client) ListStatefulSets(ctx context.Context, kubeContext, namespace string) ([]*appsv1.StatefulSet, error) {
	return listResource(ctx, c, kubeContext, "statefulsets in namespace "+namespace, statefulSetsLW(namespace),
		func(l *appsv1.StatefulSetList) []*appsv1.StatefulSet { return pointers(l.Items) })
}

// GetStatefulSetDetail fetches a single StatefulSet's status, rendered
// YAML, and recent events.
func (c *Client) GetStatefulSetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "StatefulSet"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get statefulset %s in namespace %s (context %s): %w",
			name, namespace, kubeContextName, err)
	}

	info := StatefulSetToStatefulSetInfo(sts)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Ready Replicas: %d/%d", info.ReadyReplicas, info.DesiredReplicas)
	for _, cond := range sts.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(cond.Type), string(cond.Status), cond.Reason, cond.Message))
	}
	d.YAML = renderDetailYAML(sts, "apps/v1", "StatefulSet")

	if events, err := c.getEvents(kubeContextName, namespace, "StatefulSet", name); err == nil {
		d.Events = events
	}

	return d, nil
}
