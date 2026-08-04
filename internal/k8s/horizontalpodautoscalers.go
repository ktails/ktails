package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// HorizontalPodAutoscalerInfo contains HorizontalPodAutoscaler metadata.
type HorizontalPodAutoscalerInfo struct {
	Name            string
	Namespace       string
	Age             string
	Reference       string // "kind/name" of the scale target
	MinReplicas     int32
	MaxReplicas     int32
	CurrentReplicas int32
	Targets         string // best-effort "name: N%" summary of CurrentMetrics
}

// currentMetricsSummary renders a best-effort, comma-joined summary of an
// HPA's currently observed resource-utilization metrics (e.g. "cpu: 45%,
// memory: 60%") — full metric detail (targets, object/pods/external
// metrics) is left to the YAML pane rather than reproduced here.
func currentMetricsSummary(metrics []autoscalingv2.MetricStatus) string {
	var parts []string
	for _, m := range metrics {
		if m.Resource == nil || m.Resource.Current.AverageUtilization == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %d%%", m.Resource.Name, *m.Resource.Current.AverageUtilization))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// HorizontalPodAutoscalerToHorizontalPodAutoscalerInfo converts an HPA
// object to HorizontalPodAutoscalerInfo.
func HorizontalPodAutoscalerToHorizontalPodAutoscalerInfo(hpa *autoscalingv2.HorizontalPodAutoscaler) HorizontalPodAutoscalerInfo {
	minReplicas := int32(1)
	if hpa.Spec.MinReplicas != nil {
		minReplicas = *hpa.Spec.MinReplicas
	}

	return HorizontalPodAutoscalerInfo{
		Name:            hpa.Name,
		Namespace:       hpa.Namespace,
		Age:             formatDuration(time.Since(hpa.CreationTimestamp.Time)),
		Reference:       hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name,
		MinReplicas:     minReplicas,
		MaxReplicas:     hpa.Spec.MaxReplicas,
		CurrentReplicas: hpa.Status.CurrentReplicas,
		Targets:         currentMetricsSummary(hpa.Status.CurrentMetrics),
	}
}

// horizontalPodAutoscalersLW binds a HorizontalPodAutoscalerInterface's
// List/Watch to namespace, for watchResource/listResource — see generic.go.
func horizontalPodAutoscalersLW(namespace string) func(kubernetes.Interface) listerWatcher[*autoscalingv2.HorizontalPodAutoscalerList] {
	return func(cs kubernetes.Interface) listerWatcher[*autoscalingv2.HorizontalPodAutoscalerList] {
		return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace)
	}
}

// WatchHorizontalPodAutoscalers opens a watch on HorizontalPodAutoscalers in
// the given namespace. See WatchPods for the implicit list-then-watch
// behavior.
func (c *Client) WatchHorizontalPodAutoscalers(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "horizontalpodautoscalers in namespace "+namespace, horizontalPodAutoscalersLW(namespace))
}

// ListHorizontalPodAutoscalers fetches every HorizontalPodAutoscaler in the
// given namespace in one call. See ListPods for why this exists alongside
// the watch.
func (c *Client) ListHorizontalPodAutoscalers(ctx context.Context, kubeContext, namespace string) ([]*autoscalingv2.HorizontalPodAutoscaler, error) {
	return listResource(ctx, c, kubeContext, "horizontalpodautoscalers in namespace "+namespace, horizontalPodAutoscalersLW(namespace),
		func(l *autoscalingv2.HorizontalPodAutoscalerList) []*autoscalingv2.HorizontalPodAutoscaler {
			return pointers(l.Items)
		})
}

// GetHorizontalPodAutoscalerDetail fetches a single HorizontalPodAutoscaler's
// status, rendered YAML, and recent events.
func (c *Client) GetHorizontalPodAutoscalerDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "HorizontalPodAutoscaler"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	hpa, err := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get horizontalpodautoscaler %s in namespace %s (context %s): %w",
			name, namespace, kubeContextName, err)
	}

	info := HorizontalPodAutoscalerToHorizontalPodAutoscalerInfo(hpa)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Replicas: %d (min=%s max=%s)  Reference: %s",
		info.CurrentReplicas, strconv.Itoa(int(info.MinReplicas)), strconv.Itoa(int(info.MaxReplicas)), info.Reference)
	for _, cond := range hpa.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(cond.Type), string(cond.Status), cond.Reason, cond.Message))
	}
	d.YAML = renderDetailYAML(hpa, "autoscaling/v2", "HorizontalPodAutoscaler")

	if events, err := c.getEvents(kubeContextName, namespace, "HorizontalPodAutoscaler", name); err == nil {
		d.Events = events
	}

	return d, nil
}
