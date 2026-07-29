package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodMetricsInfo is one pod's current CPU/Memory usage, summed across its
// containers.
type PodMetricsInfo struct {
	Namespace string
	Name      string
	CPU       string
	Memory    string
}

// NodeMetricsInfo is one node's current CPU/Memory usage.
type NodeMetricsInfo struct {
	Name   string
	CPU    string
	Memory string
}

// ListPodMetrics fetches current CPU/Memory usage for every pod in the given
// namespace from the Metrics Server (metrics.k8s.io). A cluster without
// metrics-server installed returns an error here (a NotFound/discovery
// error from the aggregated API) — callers treat that the same way a
// permission denial is treated elsewhere in this codebase: a quiet,
// recognizable failure, not a crash.
func (c *Client) ListPodMetrics(kubeContext, namespace string) ([]PodMetricsInfo, error) {
	metricsClient, err := c.GetMetricsClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics client for context %s: %w", kubeContext, err)
	}

	list, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pod metrics in namespace %s (context %s): %w", namespace, kubeContext, err)
	}

	out := make([]PodMetricsInfo, 0, len(list.Items))
	for _, pm := range list.Items {
		var cpu, mem resource.Quantity
		for _, container := range pm.Containers {
			if q, ok := container.Usage[corev1.ResourceCPU]; ok {
				cpu.Add(q)
			}
			if q, ok := container.Usage[corev1.ResourceMemory]; ok {
				mem.Add(q)
			}
		}
		out = append(out, PodMetricsInfo{
			Namespace: pm.Namespace,
			Name:      pm.Name,
			CPU:       cpu.String(),
			Memory:    mem.String(),
		})
	}
	return out, nil
}

// ListNodeMetrics fetches current CPU/Memory usage for every node from the
// Metrics Server. Cluster-scoped, like Nodes themselves — namespace doesn't
// apply.
func (c *Client) ListNodeMetrics(kubeContext string) ([]NodeMetricsInfo, error) {
	metricsClient, err := c.GetMetricsClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics client for context %s: %w", kubeContext, err)
	}

	list, err := metricsClient.MetricsV1beta1().NodeMetricses().List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list node metrics for context %s: %w", kubeContext, err)
	}

	out := make([]NodeMetricsInfo, 0, len(list.Items))
	for _, nm := range list.Items {
		cpu := nm.Usage[corev1.ResourceCPU]
		mem := nm.Usage[corev1.ResourceMemory]
		out = append(out, NodeMetricsInfo{
			Name:   nm.Name,
			CPU:    cpu.String(),
			Memory: mem.String(),
		})
	}
	return out, nil
}
