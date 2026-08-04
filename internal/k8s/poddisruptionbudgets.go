package k8s

import (
	"context"
	"fmt"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
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

// podDisruptionBudgetsLW binds a PodDisruptionBudgetInterface's List/Watch
// to namespace, for watchResource/listResource — see generic.go.
func podDisruptionBudgetsLW(namespace string) func(kubernetes.Interface) listerWatcher[*policyv1.PodDisruptionBudgetList] {
	return func(cs kubernetes.Interface) listerWatcher[*policyv1.PodDisruptionBudgetList] {
		return cs.PolicyV1().PodDisruptionBudgets(namespace)
	}
}

// WatchPodDisruptionBudgets opens a watch on PodDisruptionBudgets in the
// given namespace. See WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "poddisruptionbudgets in namespace "+namespace, podDisruptionBudgetsLW(namespace))
}

// ListPodDisruptionBudgets fetches every PodDisruptionBudget in the given
// namespace in one call. See ListPods for why this exists alongside the
// watch.
func (c *Client) ListPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) ([]*policyv1.PodDisruptionBudget, error) {
	return listResource(ctx, c, kubeContext, "poddisruptionbudgets in namespace "+namespace, podDisruptionBudgetsLW(namespace),
		func(l *policyv1.PodDisruptionBudgetList) []*policyv1.PodDisruptionBudget { return pointers(l.Items) })
}

// GetPodDisruptionBudgetDetail fetches a single PodDisruptionBudget's
// status, rendered YAML, and recent events.
func (c *Client) GetPodDisruptionBudgetDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "PodDisruptionBudget"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	pdb, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, v1.GetOptions{})
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
	d.YAML = renderDetailYAML(pdb, "policy/v1", "PodDisruptionBudget")

	if events, err := c.getEvents(kubeContextName, namespace, "PodDisruptionBudget", name); err == nil {
		d.Events = events
	}

	return d, nil
}
