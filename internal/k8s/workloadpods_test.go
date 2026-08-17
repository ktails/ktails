package k8s

import (
	"context"
	"sort"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ktails/ktails/internal/kinds"
)

func sortedNames(names []string) []string {
	sort.Strings(names)
	return names
}

// TestResolveWorkloadPods_Deployment guards the Spec.Selector resolution
// path shared by Deployment/StatefulSet/DaemonSet: only Pods whose labels
// match the Deployment's own selector come back, not every Pod in the
// namespace.
func TestResolveWorkloadPods_Deployment(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	matching := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "default", Labels: map[string]string{"app": "web"}},
	}
	other := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "other-1", Namespace: "default", Labels: map[string]string{"app": "other"}},
	}
	c, _ := newTestClient("ctx1", dep, matching, other)

	names, err := c.ResolveWorkloadPods(context.Background(), "ctx1", "default", kinds.KindDeployments, "web")
	if err != nil {
		t.Fatalf("ResolveWorkloadPods returned error: %v", err)
	}
	if got := sortedNames(names); len(got) != 1 || got[0] != "web-1" {
		t.Fatalf("expected [web-1], got %v", got)
	}
}

// TestResolveWorkloadPods_Job guards the job-name label path (not
// Spec.Selector, which can be nil on older Jobs).
func TestResolveWorkloadPods_Job(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "backfill", Namespace: "default"}}
	matching := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "backfill-abcde", Namespace: "default",
			Labels: map[string]string{jobNameLabel: "backfill"},
		},
	}
	unrelated := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "default"},
	}
	c, _ := newTestClient("ctx1", job, matching, unrelated)

	names, err := c.ResolveWorkloadPods(context.Background(), "ctx1", "default", kinds.KindJobs, "backfill")
	if err != nil {
		t.Fatalf("ResolveWorkloadPods returned error: %v", err)
	}
	if got := sortedNames(names); len(got) != 1 || got[0] != "backfill-abcde" {
		t.Fatalf("expected [backfill-abcde], got %v", got)
	}
}

// TestResolveWorkloadPods_CronJob guards the two-step CronJob resolution:
// find its Jobs by OwnerReference, then each Job's Pods by job-name label,
// merged across every retained Job and deduplicated.
func TestResolveWorkloadPods_CronJob(t *testing.T) {
	owner := metav1.OwnerReference{Kind: "CronJob", Name: "nightly"}
	job1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly-111", Namespace: "default", OwnerReferences: []metav1.OwnerReference{owner}},
	}
	job2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly-222", Namespace: "default", OwnerReferences: []metav1.OwnerReference{owner}},
	}
	unrelatedJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "other-333", Namespace: "default"}}

	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "nightly-111-x", Namespace: "default", Labels: map[string]string{jobNameLabel: "nightly-111"},
	}}
	pod2 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "nightly-222-x", Namespace: "default", Labels: map[string]string{jobNameLabel: "nightly-222"},
	}}
	unrelatedPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "other-333-x", Namespace: "default", Labels: map[string]string{jobNameLabel: "other-333"},
	}}

	c, _ := newTestClient("ctx1", job1, job2, unrelatedJob, pod1, pod2, unrelatedPod)

	names, err := c.ResolveWorkloadPods(context.Background(), "ctx1", "default", kinds.KindCronJobs, "nightly")
	if err != nil {
		t.Fatalf("ResolveWorkloadPods returned error: %v", err)
	}
	if got := sortedNames(names); len(got) != 2 || got[0] != "nightly-111-x" || got[1] != "nightly-222-x" {
		t.Fatalf("expected [nightly-111-x nightly-222-x], got %v", got)
	}
}

// TestResolveWorkloadPods_UnsupportedKind guards the fallback for a kind
// that doesn't own Pods (HasPods() false) reaching this function anyway.
func TestResolveWorkloadPods_UnsupportedKind(t *testing.T) {
	c, _ := newTestClient("ctx1")
	if _, err := c.ResolveWorkloadPods(context.Background(), "ctx1", "default", kinds.KindConfigMaps, "foo"); err == nil {
		t.Fatal("expected an error for a kind that doesn't own pods")
	}
}
