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

// WatchStatefulSets opens a watch on StatefulSets in the given namespace.
// See WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchStatefulSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.AppsV1().StatefulSets(namespace).Watch(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch statefulsets in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// GetStatefulSetDetail fetches a single StatefulSet's status, rendered
// YAML, and recent events.
func (c *Client) GetStatefulSetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "StatefulSet"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), name, v1.GetOptions{})
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

	sts.ManagedFields = nil
	sts.TypeMeta = v1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"}
	if yamlBytes, yamlErr := yaml.Marshal(sts); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "StatefulSet", name); err == nil {
		d.Events = events
	}

	return d, nil
}
