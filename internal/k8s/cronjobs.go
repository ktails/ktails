package k8s

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/yaml"
)

// CronJobInfo contains CronJob metadata.
type CronJobInfo struct {
	Name          string
	Namespace     string
	Age           string
	Schedule      string
	Suspend       bool
	LastScheduled string
}

// CronJobToCronJobInfo converts a CronJob object to CronJobInfo.
func CronJobToCronJobInfo(cj *batchv1.CronJob) CronJobInfo {
	lastScheduled := "never"
	if cj.Status.LastScheduleTime != nil {
		lastScheduled = formatDuration(time.Since(cj.Status.LastScheduleTime.Time)) + " ago"
	}

	suspend := false
	if cj.Spec.Suspend != nil {
		suspend = *cj.Spec.Suspend
	}

	return CronJobInfo{
		Name:          cj.Name,
		Namespace:     cj.Namespace,
		Age:           formatDuration(time.Since(cj.CreationTimestamp.Time)),
		Schedule:      cj.Spec.Schedule,
		Suspend:       suspend,
		LastScheduled: lastScheduled,
	}
}

// WatchCronJobs opens a watch on CronJobs in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchCronJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.BatchV1().CronJobs(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch cronjobs in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// ListCronJobs fetches every CronJob in the given namespace in one call. See
// ListPods for why this exists alongside the watch.
func (c *Client) ListCronJobs(ctx context.Context, kubeContext, namespace string) ([]*batchv1.CronJob, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	list, err := clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cronjobs in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	out := make([]*batchv1.CronJob, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

// GetCronJobDetail fetches a single CronJob's status, rendered YAML, and
// recent events.
func (c *Client) GetCronJobDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "CronJob"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	cj, err := clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get cronjob %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := CronJobToCronJobInfo(cj)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Schedule: %s, Last scheduled: %s", info.Schedule, info.LastScheduled)

	cj.ManagedFields = nil
	cj.TypeMeta = metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"}
	if yamlBytes, yamlErr := yaml.Marshal(cj); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "CronJob", name); err == nil {
		d.Events = events
	}

	return d, nil
}
