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

// JobInfo contains Job metadata.
type JobInfo struct {
	Name        string
	Namespace   string
	Age         string
	Completions string
	Duration    string
	Status      string
}

// JobToJobInfo converts a Job object to JobInfo.
func JobToJobInfo(job *batchv1.Job) JobInfo {
	desired := int32(1)
	if job.Spec.Completions != nil {
		desired = *job.Spec.Completions
	}

	duration := ""
	switch {
	case job.Status.StartTime != nil && job.Status.CompletionTime != nil:
		duration = formatDuration(job.Status.CompletionTime.Sub(job.Status.StartTime.Time))
	case job.Status.StartTime != nil:
		duration = formatDuration(time.Since(job.Status.StartTime.Time))
	}

	status := "Running"
	for _, cond := range job.Status.Conditions {
		if cond.Status != "True" {
			continue
		}
		switch cond.Type {
		case batchv1.JobComplete:
			status = "Complete"
		case batchv1.JobFailed:
			status = "Failed"
		}
	}

	return JobInfo{
		Name:        job.Name,
		Namespace:   job.Namespace,
		Age:         formatDuration(time.Since(job.CreationTimestamp.Time)),
		Completions: fmt.Sprintf("%d/%d", job.Status.Succeeded, desired),
		Duration:    duration,
		Status:      status,
	}
}

// WatchJobs opens a watch on Jobs in the given namespace. See WatchPods for
// the implicit list-then-watch behavior.
func (c *Client) WatchJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch jobs in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// GetJobDetail fetches a single Job's status, rendered YAML, and recent events.
func (c *Client) GetJobDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Job"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	job, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get job %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := JobToJobInfo(job)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Completions: %s, Status: %s", info.Completions, info.Status)
	for _, cond := range job.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(cond.Type), string(cond.Status), cond.Reason, cond.Message))
	}

	job.ManagedFields = nil
	job.TypeMeta = metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"}
	if yamlBytes, yamlErr := yaml.Marshal(job); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "Job", name); err == nil {
		d.Events = events
	}

	return d, nil
}
