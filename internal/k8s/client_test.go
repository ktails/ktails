package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd/api"
)

// newTestClient builds a Client backed by a fake clientset for the given
// context, bypassing the real kubeconfig-based connection path in
// GetClientForContext/createClientForContext.
func newTestClient(kubeContext string, objects ...runtime.Object) (*Client, *fake.Clientset) {
	clientset := fake.NewClientset(objects...)
	c := &Client{
		clientsByContext: map[string]kubernetes.Interface{
			kubeContext: clientset,
		},
	}
	return c, clientset
}

func TestWatchPods_ReplaysExistingThenLiveEvents(t *testing.T) {
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default"},
	}
	c, clientset := newTestClient("ctx1", existingPod)

	w, err := c.WatchPods(context.Background(), "ctx1", "default")
	if err != nil {
		t.Fatalf("WatchPods returned error: %v", err)
	}
	defer w.Stop()

	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Added {
			t.Fatalf("expected initial replay Added event, got %v", ev.Type)
		}
		pod, ok := ev.Object.(*corev1.Pod)
		if !ok || pod.Name != "pod-a" {
			t.Fatalf("expected replayed pod-a, got %+v", ev.Object)
		}
	default:
		t.Fatal("expected a buffered initial replay event, got none")
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "default"},
	}
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), newPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create pod-b: %v", err)
	}

	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Added {
			t.Fatalf("expected live Added event, got %v", ev.Type)
		}
		pod, ok := ev.Object.(*corev1.Pod)
		if !ok || pod.Name != "pod-b" {
			t.Fatalf("expected live pod-b, got %+v", ev.Object)
		}
	default:
		t.Fatal("expected a live create event, got none")
	}
}

func TestWatchDeployments_ReplaysExisting(t *testing.T) {
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "default"},
	}
	c, _ := newTestClient("ctx1", existing)

	w, err := c.WatchDeployments(context.Background(), "ctx1", "default")
	if err != nil {
		t.Fatalf("WatchDeployments returned error: %v", err)
	}
	defer w.Stop()

	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Added {
			t.Fatalf("expected initial replay Added event, got %v", ev.Type)
		}
		dep, ok := ev.Object.(*appsv1.Deployment)
		if !ok || dep.Name != "dep-a" {
			t.Fatalf("expected replayed dep-a, got %+v", ev.Object)
		}
	default:
		t.Fatal("expected a buffered initial replay event, got none")
	}
}

func TestWatchServices_ReplaysExisting(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default"},
	}
	c, _ := newTestClient("ctx1", existing)

	w, err := c.WatchServices(context.Background(), "ctx1", "default")
	if err != nil {
		t.Fatalf("WatchServices returned error: %v", err)
	}
	defer w.Stop()

	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Added {
			t.Fatalf("expected initial replay Added event, got %v", ev.Type)
		}
		svc, ok := ev.Object.(*corev1.Service)
		if !ok || svc.Name != "svc-a" {
			t.Fatalf("expected replayed svc-a, got %+v", ev.Object)
		}
	default:
		t.Fatal("expected a buffered initial replay event, got none")
	}
}

func TestWatchPods_UnknownContext(t *testing.T) {
	c := &Client{
		clientsByContext: map[string]kubernetes.Interface{},
		rawConfig:        &api.Config{Contexts: map[string]*api.Context{}},
	}
	if _, err := c.WatchPods(context.Background(), "missing", "default"); err == nil {
		t.Fatal("expected error for unknown context, got nil")
	}
}

// TestCanWatchNodes_AllowedAndDenied guards both SelfSubjectAccessReview
// outcomes. The fake clientset's default tracker can't handle this resource
// at all (it's a virtual, non-stored type, like TokenReview), so a reactor
// standing in for the apiserver's actual authorization decision is required
// for either outcome, not just "allowed".
func TestCanWatchNodes_AllowedAndDenied(t *testing.T) {
	c, clientset := newTestClient("ctx1")

	var wantAllowed bool
	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		review := action.(clienttesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
		review.Status.Allowed = wantAllowed
		return true, review, nil
	})

	allowed, err := c.CanWatchNodes("ctx1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected denied when the reactor reports Allowed=false")
	}

	wantAllowed = true
	allowed, err = c.CanWatchNodes("ctx1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed once the reactor grants it")
	}
}

func TestCanWatchNodes_UnknownContext(t *testing.T) {
	c := &Client{
		clientsByContext: map[string]kubernetes.Interface{},
		rawConfig:        &api.Config{Contexts: map[string]*api.Context{}},
	}
	if _, err := c.CanWatchNodes("missing"); err == nil {
		t.Fatal("expected error for unknown context, got nil")
	}
}
