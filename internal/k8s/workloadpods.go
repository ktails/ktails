package k8s

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/ktails/ktails/internal/kinds"
)

// jobNameLabel is the label every Pod a Job creates carries — kubectl's own
// convention (`kubectl logs job/<name>` resolves the same way), used instead
// of a Job's Spec.Selector, which can be nil on older Jobs.
const jobNameLabel = "batch.kubernetes.io/job-name"

// ResolveWorkloadPods returns the names of every Pod currently owned by the
// given workload — the aggregate-log counterpart to a single Pod row: kind
// must satisfy kinds.ResourceKind.HasPods(). Namespace is the caller's own
// (already-known) responsibility; only pod names are returned, not full Pod
// objects, since the caller (MainPage.openWorkloadLogs) looks each one up in
// the already-loaded Pods-tab cache for its containers.
func (c *Client) ResolveWorkloadPods(ctx context.Context, kubeContext, namespace string, kind kinds.ResourceKind, name string) ([]string, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	switch kind {
	case kinds.KindDeployments:
		dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get deployment %s in namespace %s: %w", name, namespace, err)
		}
		return podsBySelector(ctx, clientset, namespace, dep.Spec.Selector)

	case kinds.KindStatefulSets:
		sts, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get statefulset %s in namespace %s: %w", name, namespace, err)
		}
		return podsBySelector(ctx, clientset, namespace, sts.Spec.Selector)

	case kinds.KindDaemonSets:
		ds, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get daemonset %s in namespace %s: %w", name, namespace, err)
		}
		return podsBySelector(ctx, clientset, namespace, ds.Spec.Selector)

	case kinds.KindJobs:
		return podsByJobName(ctx, clientset, namespace, name)

	case kinds.KindCronJobs:
		return podsForCronJob(ctx, clientset, namespace, name)
	}

	return nil, fmt.Errorf("kind %s does not own pods", kind)
}

// podsBySelector lists Pod names in namespace matching a workload's
// `.spec.selector`, shared by Deployment/StatefulSet/DaemonSet — all three
// select their Pods the same way, only the object fetched to get there
// differs.
func podsBySelector(ctx context.Context, clientset kubernetes.Interface, namespace string, selector *metav1.LabelSelector) ([]string, error) {
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("invalid pod selector: %w", err)
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}
	return podNames(pods.Items), nil
}

// podsByJobName lists Pod names owned by one Job, via kubectl's own
// job-name label convention rather than the Job's (possibly nil)
// Spec.Selector.
func podsByJobName(ctx context.Context, clientset kubernetes.Interface, namespace, jobName string) ([]string, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", jobNameLabel, jobName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for job %s in namespace %s: %w", jobName, namespace, err)
	}
	return podNames(pods.Items), nil
}

// podsForCronJob resolves every Pod across every Job a CronJob currently
// retains — CronJobs don't label their own Pods directly, so this is a
// two-step resolution: find this CronJob's Jobs by OwnerReference, then
// each Job's Pods by the same job-name label podsByJobName uses.
func podsForCronJob(ctx context.Context, clientset kubernetes.Interface, namespace, cronJobName string) ([]string, error) {
	jobs, err := clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs in namespace %s: %w", namespace, err)
	}

	var names []string
	seen := make(map[string]bool)
	for _, job := range jobs.Items {
		if !ownedByCronJob(job, cronJobName) {
			continue
		}
		jobPods, err := podsByJobName(ctx, clientset, namespace, job.Name)
		if err != nil {
			return nil, err
		}
		for _, n := range jobPods {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	return names, nil
}

func ownedByCronJob(job batchv1.Job, cronJobName string) bool {
	for _, ref := range job.OwnerReferences {
		if ref.Kind == "CronJob" && ref.Name == cronJobName {
			return true
		}
	}
	return false
}

func podNames(pods []corev1.Pod) []string {
	names := make([]string, len(pods))
	for i, p := range pods {
		names[i] = p.Name
	}
	return names
}
