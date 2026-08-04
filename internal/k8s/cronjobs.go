package k8s

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
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

// cronJobsLW binds a CronJobInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func cronJobsLW(namespace string) func(kubernetes.Interface) listerWatcher[*batchv1.CronJobList] {
	return func(cs kubernetes.Interface) listerWatcher[*batchv1.CronJobList] {
		return cs.BatchV1().CronJobs(namespace)
	}
}

// WatchCronJobs opens a watch on CronJobs in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchCronJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "cronjobs in namespace "+namespace, cronJobsLW(namespace))
}

// ListCronJobs fetches every CronJob in the given namespace in one call. See
// ListPods for why this exists alongside the watch.
func (c *Client) ListCronJobs(ctx context.Context, kubeContext, namespace string) ([]*batchv1.CronJob, error) {
	return listResource(ctx, c, kubeContext, "cronjobs in namespace "+namespace, cronJobsLW(namespace),
		func(l *batchv1.CronJobList) []*batchv1.CronJob { return pointers(l.Items) })
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
	d.YAML = renderDetailYAML(cj, "batch/v1", "CronJob")

	if events, err := c.getEvents(kubeContextName, namespace, "CronJob", name); err == nil {
		d.Events = events
	}

	return d, nil
}
