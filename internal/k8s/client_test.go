package k8s

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
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

// newDialTestClient builds a Client whose dial is controlled entirely by
// build — no real kubeconfig or network involved — for exercising
// GetClientForContext's in-flight dedup (see getOrBuild).
func newDialTestClient(build func(contextName string) (kubernetes.Interface, error)) *Client {
	c := &Client{
		clientsByContext: make(map[string]kubernetes.Interface),
		building:         make(map[string]*buildResult[kubernetes.Interface]),
	}
	c.buildClient = build
	return c
}

// TestGetClientForContext_DifferentContextsDoNotBlock guards the regression
// this fix targets: GetClientForContext used to hold the write lock across
// the whole dial, so one slow/unreachable context's lookup froze every other
// context's lookup too. A dial for "fast" must complete without waiting for
// a concurrent, still-in-flight dial for "slow".
func TestGetClientForContext_DifferentContextsDoNotBlock(t *testing.T) {
	release := make(chan struct{})
	c := newDialTestClient(func(contextName string) (kubernetes.Interface, error) {
		if contextName == "slow" {
			<-release
		}
		return fake.NewClientset(), nil
	})

	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		if _, err := c.GetClientForContext("slow"); err != nil {
			t.Errorf("slow dial failed: %v", err)
		}
	}()

	// Give the "slow" goroutine a head start so it's genuinely in flight
	// before "fast" is requested — without the fix, "fast" would queue
	// behind it regardless of order.
	time.Sleep(20 * time.Millisecond)

	fastDone := make(chan struct{})
	go func() {
		defer close(fastDone)
		if _, err := c.GetClientForContext("fast"); err != nil {
			t.Errorf("fast dial failed: %v", err)
		}
	}()

	select {
	case <-fastDone:
	case <-time.After(2 * time.Second):
		t.Fatal("fast dial blocked behind the in-flight slow dial")
	case <-slowDone:
		t.Fatal("slow dial finished before fast was even released — release channel never closed yet")
	}

	close(release)
	<-slowDone
}

// TestGetClientForContext_SameContextDedupsInFlightDials guards the other
// half of getOrBuild: concurrent requests for the *same* context must share
// one dial rather than each triggering their own.
func TestGetClientForContext_SameContextDedupsInFlightDials(t *testing.T) {
	var buildCalls int64
	release := make(chan struct{})
	c := newDialTestClient(func(contextName string) (kubernetes.Interface, error) {
		atomic.AddInt64(&buildCalls, 1)
		<-release
		return fake.NewClientset(), nil
	})

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.GetClientForContext("shared")
		}(i)
	}

	// Let every goroutine reach the in-flight wait before releasing the
	// single dial, so this actually exercises the dedup path rather than
	// racing ahead of it.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	if got := atomic.LoadInt64(&buildCalls); got != 1 {
		t.Fatalf("expected exactly one build call for concurrent requests to the same context, got %d", got)
	}
}

// TestListPods_TimesOutAgainstHungServer guards the regression this fix
// targets: every one-shot call used context.Background() with
// rest.Config.Timeout == 0, so a hung-but-connected apiserver blocked a
// List/Get call forever with no error path. requestTimeout is temporarily
// shrunk (it's a package var for exactly this) so the test doesn't actually
// wait 15 real seconds — the server sleeps well past that shrunk timeout
// before responding, so the request is guaranteed to still be outstanding
// when the client-side deadline fires.
func TestListPods_TimesOutAgainstHungServer(t *testing.T) {
	origTimeout := requestTimeout
	requestTimeout = 30 * time.Millisecond
	defer func() { requestTimeout = origTimeout }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("failed to build clientset against test server: %v", err)
	}
	c := &Client{clientsByContext: map[string]kubernetes.Interface{"ctx1": clientset}}

	_, err = c.ListPods(context.Background(), "ctx1", "default")
	if err == nil {
		t.Fatal("expected an error from a hung server, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a context.DeadlineExceeded-wrapped error, got: %v", err)
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
