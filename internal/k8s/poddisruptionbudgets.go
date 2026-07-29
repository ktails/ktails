package k8s

import (
	"context"
	"fmt"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/yaml"
)

// PodDisruptionBudgetInfo contains PodDisruptionBudget metadata.
type PodDisruptionBudgetInfo struct {
	Name               string
	Namespace          string
	Age                string
	MinMaxAvailable    string // e.g. "minAvailable=2" or "maxUnavailable=25%"
	CurrentHealthy     int32
	DesiredHealthy     int32
	AllowedDisruptions int32
}

// PodDisruptionBudgetToPodDisruptionBudgetInfo converts a PodDisruptionBudget
// object to PodDisruptionBudgetInfo.
func PodDisruptionBudgetToPodDisruptionBudgetInfo(pdb *policyv1.PodDisruptionBudget) PodDisruptionBudgetInfo {
	minMax := "-"
	if pdb.Spec.MinAvailable != nil {
		minMax = "minAvailable=" + pdb.Spec.MinAvailable.String()
	} else if pdb.Spec.MaxUnavailable != nil {
		minMax = "maxUnavailable=" + pdb.Spec.MaxUnavailable.String()
	}

	return PodDisruptionBudgetInfo{
		Name:               pdb.Name,
		Namespace:          pdb.Namespace,
		Age:                formatDuration(time.Since(pdb.CreationTimestamp.Time)),
		MinMaxAvailable:    minMax,
		CurrentHealthy:     pdb.Status.CurrentHealthy,
		DesiredHealthy:     pdb.Status.DesiredHealthy,
		AllowedDisruptions: pdb.Status.DisruptionsAllowed,
	}
}

// WatchPodDisruptionBudgets opens a watch on PodDisruptionBudgets in the
// given namespace. See WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).Watch(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch poddisruptionbudgets in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// ListPodDisruptionBudgets fetches every PodDisruptionBudget in the given
// namespace in one call. See ListPods for why this exists alongside the
// watch.
func (c *Client) ListPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) ([]*policyv1.PodDisruptionBudget, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	list, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list poddisruptionbudgets in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	out := make([]*policyv1.PodDisruptionBudget, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

// GetPodDisruptionBudgetDetail fetches a single PodDisruptionBudget's
// status, rendered YAML, and recent events.
func (c *Client) GetPodDisruptionBudgetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "PodDisruptionBudget"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	pdb, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get poddisruptionbudget %s in namespace %s (context %s): %w",
			name, namespace, kubeContextName, err)
	}

	info := PodDisruptionBudgetToPodDisruptionBudgetInfo(pdb)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Healthy: %d/%d  Allowed Disruptions: %d", info.CurrentHealthy, info.DesiredHealthy, info.AllowedDisruptions)
	for _, cond := range pdb.Status.Conditions {
		d.Status = append(d.Status, formatCondition(cond.Type, string(cond.Status), cond.Reason, cond.Message))
	}

	pdb.ManagedFields = nil
	pdb.TypeMeta = v1.TypeMeta{APIVersion: "policy/v1", Kind: "PodDisruptionBudget"}
	if yamlBytes, yamlErr := yaml.Marshal(pdb); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "PodDisruptionBudget", name); err == nil {
		d.Events = events
	}

	return d, nil
}
