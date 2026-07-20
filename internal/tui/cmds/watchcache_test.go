package cmds

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func TestPodWatchCache_AddedModifiedDeleted(t *testing.T) {
	c := NewPodWatchCache()

	podA := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "1"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	if err := c.apply(watch.Event{Type: watch.Added, Object: podA}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.Rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.PodKeyStatus] != "Pending" {
		t.Fatalf("expected Pending, got %v", rows[0][msgs.PodKeyStatus])
	}

	podAModified := podA.DeepCopy()
	podAModified.ResourceVersion = "2"
	podAModified.Status.Phase = corev1.PodRunning
	if err := c.apply(watch.Event{Type: watch.Modified, Object: podAModified}); err != nil {
		t.Fatalf("apply Modified: %v", err)
	}

	rows = c.Rows("ctx1")
	if len(rows) != 1 || rows[0][msgs.PodKeyStatus] != "Running" {
		t.Fatalf("expected 1 Running row after modify, got %+v", rows)
	}

	if err := c.apply(watch.Event{Type: watch.Deleted, Object: podA}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.Rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestPodWatchCache_StaleModifiedIgnored(t *testing.T) {
	c := NewPodWatchCache()

	podA := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "5"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if err := c.apply(watch.Event{Type: watch.Added, Object: podA}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	stale := podA.DeepCopy()
	stale.ResourceVersion = "3"
	stale.Status.Phase = corev1.PodFailed
	if err := c.apply(watch.Event{Type: watch.Modified, Object: stale}); err != nil {
		t.Fatalf("apply stale Modified: %v", err)
	}

	rows := c.Rows("ctx1")
	if len(rows) != 1 || rows[0][msgs.PodKeyStatus] != "Running" {
		t.Fatalf("expected stale redelivery to be ignored, got %+v", rows)
	}
}

func TestPodWatchCache_ErrorEvent(t *testing.T) {
	c := NewPodWatchCache()
	status := &metav1.Status{Message: "boom"}
	err := c.apply(watch.Event{Type: watch.Error, Object: status})
	if err == nil {
		t.Fatal("expected error from watch.Error event")
	}
}

func TestPodWatchCache_SortedByNamespaceThenName(t *testing.T) {
	c := NewPodWatchCache()
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "ns2", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns1", ResourceVersion: "1"}},
	}
	for _, p := range pods {
		if err := c.apply(watch.Event{Type: watch.Added, Object: p}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}

	rows := c.Rows("ctx1")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	wantOrder := []string{"a", "b", "z"}
	for i, want := range wantOrder {
		if rows[i][msgs.PodKeyName] != want {
			t.Fatalf("row %d: expected name %s, got %v", i, want, rows[i][msgs.PodKeyName])
		}
	}
}

func TestDeploymentWatchCache_AddedDeleted(t *testing.T) {
	c := NewDeploymentWatchCache()
	replicas := int32(3)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "default", ResourceVersion: "1"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	if err := c.apply(watch.Event{Type: watch.Added, Object: dep}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.Rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.DeployKeyReplicas] != "2/3" {
		t.Fatalf("expected replicas 2/3, got %v", rows[0][msgs.DeployKeyReplicas])
	}

	if err := c.apply(watch.Event{Type: watch.Deleted, Object: dep}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.Rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestServiceWatchCache_RowsIncludeEndpointPlaceholder(t *testing.T) {
	c := NewServiceWatchCache()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default", ResourceVersion: "1"},
	}
	if err := c.apply(watch.Event{Type: watch.Added, Object: svc}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.Rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.SvcKeyEndpointIPs] != endpointIPsPlaceholder {
		t.Fatalf("expected placeholder %q, got %v", endpointIPsPlaceholder, rows[0][msgs.SvcKeyEndpointIPs])
	}
}
