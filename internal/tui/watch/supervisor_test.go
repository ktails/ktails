package watch

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kwatch "k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// fakeCluster is the test adapter at the Cluster seam: it serves pre-made
// FakeWatchers (or an error) instead of dialing a real apiserver.
type fakeCluster struct {
	watchers map[msgs.ResourceKind]*kwatch.FakeWatcher
	err      error
}

func newFakeCluster() *fakeCluster {
	watchers := make(map[msgs.ResourceKind]*kwatch.FakeWatcher, len(msgs.Kinds()))
	for _, kind := range msgs.Kinds() {
		watchers[kind] = kwatch.NewFakeWithChanSize(16, false)
	}
	return &fakeCluster{watchers: watchers}
}

func (f *fakeCluster) watch(kind msgs.ResourceKind) (kwatch.Interface, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.watchers[kind], nil
}

func (f *fakeCluster) WatchPods(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindPods)
}

func (f *fakeCluster) WatchDeployments(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindDeployments)
}

func (f *fakeCluster) WatchServices(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindServices)
}

func (f *fakeCluster) WatchConfigMaps(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindConfigMaps)
}

func (f *fakeCluster) WatchSecrets(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindSecrets)
}

func (f *fakeCluster) WatchNodes(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindNodes)
}

func (f *fakeCluster) WatchJobs(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindJobs)
}

func (f *fakeCluster) WatchCronJobs(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindCronJobs)
}

func (f *fakeCluster) WatchStatefulSets(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindStatefulSets)
}

func (f *fakeCluster) WatchDaemonSets(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindDaemonSets)
}

func (f *fakeCluster) WatchIngresses(context.Context, string, string) (kwatch.Interface, error) {
	return f.watch(msgs.KindIngresses)
}

// List<Kind> fakes: every kind lists as empty (no error) by default —
// nothing in this test suite exercises the List-first paint's row content,
// only its ordering ahead of the watch, so an empty list is sufficient.
func (f *fakeCluster) ListPods(context.Context, string, string) ([]*corev1.Pod, error) {
	return nil, nil
}

func (f *fakeCluster) ListDeployments(context.Context, string, string) ([]*appsv1.Deployment, error) {
	return nil, nil
}

func (f *fakeCluster) ListServices(context.Context, string, string) ([]*corev1.Service, error) {
	return nil, nil
}

func (f *fakeCluster) ListConfigMaps(context.Context, string, string) ([]*corev1.ConfigMap, error) {
	return nil, nil
}

func (f *fakeCluster) ListSecrets(context.Context, string, string) ([]*corev1.Secret, error) {
	return nil, nil
}

func (f *fakeCluster) ListJobs(context.Context, string, string) ([]*batchv1.Job, error) {
	return nil, nil
}

func (f *fakeCluster) ListCronJobs(context.Context, string, string) ([]*batchv1.CronJob, error) {
	return nil, nil
}

func (f *fakeCluster) ListStatefulSets(context.Context, string, string) ([]*appsv1.StatefulSet, error) {
	return nil, nil
}

func (f *fakeCluster) ListDaemonSets(context.Context, string, string) ([]*appsv1.DaemonSet, error) {
	return nil, nil
}

func (f *fakeCluster) ListIngresses(context.Context, string, string) ([]*networkingv1.Ingress, error) {
	return nil, nil
}

func (f *fakeCluster) ListNodes(context.Context, string, string) ([]*corev1.Node, error) {
	return nil, nil
}

// runCmd executes a tea.Cmd synchronously and returns its message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	return cmd()
}

// driveToOpened runs a start/AddNamespace command through the List-first
// sequence (see Supervisor.start): it executes cmd, expects a
// ListLoadedMsg, feeds it to Handle to seed the cache and obtain the
// follow-up open command, executes that, and returns the resulting
// WatchOpenedMsg — the point every pre-List-first test used to start from.
func driveToOpened(t *testing.T, s *Supervisor, cmd tea.Cmd) msgs.WatchOpenedMsg {
	t.Helper()
	msg := runCmd(t, cmd)
	listMsg, ok := msg.(msgs.ListLoadedMsg)
	if !ok {
		t.Fatalf("expected ListLoadedMsg, got %T", msg)
	}
	_, openCmd, handled := s.Handle(listMsg)
	if !handled || openCmd == nil {
		t.Fatalf("expected the list result to hand back an open command, handled=%v openCmd=%v", handled, openCmd)
	}
	opened, ok := runCmd(t, openCmd).(msgs.WatchOpenedMsg)
	if !ok {
		t.Fatalf("expected WatchOpenedMsg, got %T", opened)
	}
	return opened
}

// startPodsWatch drives one context's Pods watch through list+open+adopt: it
// runs StartContext, drives each kind's List->Watch sequence, and hands the
// Pods WatchOpenedMsg to Handle, returning the wait command that reads
// events.
func startPodsWatch(t *testing.T, s *Supervisor) tea.Cmd {
	t.Helper()
	var waitCmd tea.Cmd
	for _, cmd := range s.StartContext("ctx1") {
		opened := driveToOpened(t, s, cmd)
		upd, next, handled := s.Handle(opened)
		if !handled || upd == nil || !upd.RowsChanged || next == nil {
			t.Fatalf("expected opened msg to report loaded with a wait command, got upd=%v next=%v handled=%v", upd, next, handled)
		}
		if opened.Kind == msgs.KindPods {
			waitCmd = next
		}
	}
	return waitCmd
}

// TestSupervisor_OpenWithNoEventsStillMarksLoaded is the regression test for
// the "stuck on Secrets list" bug: a namespace with zero existing objects of
// a kind gets no synthetic replay events at all (Kubernetes only replays
// what exists), so a watch opened against it may never deliver a single
// WatchEventMsg. Handle must report the (kind, context) as loaded the moment
// the watch opens, not wait for an event that may never arrive — otherwise
// the loading spinner never clears and an empty tab looks permanently stuck.
func TestSupervisor_OpenWithNoEventsStillMarksLoaded(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)

	var secretsOpened msgs.WatchOpenedMsg
	for _, cmd := range s.StartContext("ctx1") {
		opened := driveToOpened(t, s, cmd)
		if opened.Kind == msgs.KindSecrets {
			secretsOpened = opened
		}
	}

	// Nothing is ever added to cluster.watchers[msgs.KindSecrets] — this
	// namespace has zero Secrets, exactly the scenario that hung before.
	upd, next, handled := s.Handle(secretsOpened)
	if !handled || upd == nil || !upd.RowsChanged {
		t.Fatalf("expected the open itself to report loaded, got upd=%+v handled=%v", upd, handled)
	}
	if upd.Kind != msgs.KindSecrets || upd.Context != "ctx1" {
		t.Fatalf("expected update for (Secrets, ctx1), got %+v", upd)
	}
	if next == nil {
		t.Fatal("expected a wait command to keep listening for future events")
	}
	if rows := s.Rows(msgs.KindSecrets); len(rows) != 0 {
		t.Fatalf("expected zero rows for an empty namespace, got %+v", rows)
	}
}

// TestSupervisor_ListSeedsCacheBeforeWatchOpens guards the List-first hybrid
// load: start() must issue a List() call before the watch opens, and the
// List result must seed the cache (so Rows() returns every listed object at
// once) rather than waiting for the watch's synthetic Added replay to
// trickle rows in one at a time.
func TestSupervisor_ListSeedsCacheBeforeWatchOpens(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)

	var podsCmd tea.Cmd
	for _, cmd := range s.StartContext("ctx1") {
		if opened := runCmd(t, cmd); opened != nil {
			if lm, ok := opened.(msgs.ListLoadedMsg); ok && lm.Kind == msgs.KindPods {
				podsCmd = cmd
			}
		}
	}
	if podsCmd == nil {
		t.Fatal("expected a List command for Pods")
	}

	listMsg := runCmd(t, podsCmd).(msgs.ListLoadedMsg)
	upd, openCmd, handled := s.Handle(listMsg)
	if !handled || upd == nil || !upd.RowsChanged {
		t.Fatalf("expected the List result to report loaded rows immediately, got upd=%+v handled=%v", upd, handled)
	}
	if openCmd == nil {
		t.Fatal("expected the List result to hand back a command to open the watch")
	}

	// The cache was seeded from the List result before the watch ever
	// opened — Rows() must already reflect it.
	rows := s.Rows(msgs.KindPods)
	if len(rows) != 0 {
		t.Fatalf("expected the fake's empty List result reflected in Rows(), got %+v", rows)
	}

	opened, ok := runCmd(t, openCmd).(msgs.WatchOpenedMsg)
	if !ok {
		t.Fatalf("expected the open command to yield WatchOpenedMsg, got %T", opened)
	}
	if opened.Kind != msgs.KindPods || opened.Context != "ctx1" {
		t.Fatalf("expected WatchOpenedMsg for (Pods, ctx1), got %+v", opened)
	}
}

func TestSupervisor_EventFlowProducesRows(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)
	waitCmd := startPodsWatch(t, s)

	cluster.watchers[msgs.KindPods].Add(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", ResourceVersion: "1"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})

	msg := runCmd(t, waitCmd)
	event, ok := msg.(msgs.WatchEventMsg)
	if !ok {
		t.Fatalf("expected WatchEventMsg, got %T", msg)
	}

	upd, next, handled := s.Handle(event)
	if !handled || next == nil {
		t.Fatalf("expected event handled with a follow-up wait command")
	}
	if upd == nil || !upd.RowsChanged || upd.Kind != msgs.KindPods || upd.Context != "ctx1" {
		t.Fatalf("expected a RowsChanged update for (Pods, ctx1), got %+v", upd)
	}

	rows := s.Rows(msgs.KindPods)
	if len(rows) != 1 || rows[0][msgs.PodKeyName] != "pod-a" {
		t.Fatalf("expected pod-a in supervisor rows, got %+v", rows)
	}
}

func TestSupervisor_StaleGenerationDropped(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)
	startPodsWatch(t, s)

	// A message from a superseded watch (old generation) must be dropped and
	// its watcher stopped, not adopted.
	staleWatcher := kwatch.NewFakeWithChanSize(1, false)
	upd, next, handled := s.Handle(msgs.WatchOpenedMsg{
		Kind: msgs.KindPods, Context: "ctx1", Generation: 0, Watcher: staleWatcher,
	})
	if !handled || upd != nil || next != nil {
		t.Fatalf("expected stale opened msg swallowed, got upd=%v next=%v", upd, next)
	}
	if !staleWatcher.IsStopped() {
		t.Fatal("expected the stale watcher to be stopped")
	}
}

func TestSupervisor_StopContextStopsWatcherAndDropsRows(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)
	waitCmd := startPodsWatch(t, s)

	cluster.watchers[msgs.KindPods].Add(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", ResourceVersion: "1"},
	})
	if upd, _, _ := s.Handle(runCmd(t, waitCmd)); upd == nil || !upd.RowsChanged {
		t.Fatal("expected rows before StopContext")
	}

	s.StopContext("ctx1")
	if !cluster.watchers[msgs.KindPods].IsStopped() {
		t.Fatal("expected StopContext to stop the pods watcher")
	}
	if rows := s.Rows(msgs.KindPods); len(rows) != 0 {
		t.Fatalf("expected no rows after StopContext, got %d", len(rows))
	}
	if s.Watching() {
		t.Fatal("expected supervisor to be idle after StopContext")
	}
}

func TestSupervisor_ClosedReconnectsThenGivesUp(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)
	startPodsWatch(t, s)

	closed := msgs.WatchClosedMsg{
		Kind: msgs.KindPods, Context: "ctx1", Generation: 1, Err: errors.New("stream broke"),
	}

	// The first maxReconnectFailures closes each get a reconnect command.
	for i := 0; i < maxReconnectFailures; i++ {
		upd, next, handled := s.Handle(closed)
		if !handled || upd != nil {
			t.Fatalf("close %d: expected a silent reconnect, got update %+v", i+1, upd)
		}
		if next == nil {
			t.Fatalf("close %d: expected a reconnect command", i+1)
		}
	}

	// The next close exhausts the budget: no reconnect, a GaveUp update.
	upd, next, handled := s.Handle(closed)
	if !handled || next != nil {
		t.Fatal("expected no reconnect command after exhausting failures")
	}
	if upd == nil || !upd.GaveUp || upd.Err == nil {
		t.Fatalf("expected a GaveUp update with an error, got %+v", upd)
	}
	if upd.Forbidden {
		t.Fatal("expected an exhausted-reconnect failure to not be marked Forbidden")
	}
}

// TestSupervisor_ForbiddenGivesUpImmediatelyAndMarksForbidden guards the
// quiet-RBAC-failure path: an RBAC denial must give up on the first close
// (no backoff/reconnect attempts wasted on a permission that isn't coming
// back) and set Update.Forbidden so the caller (MainPage) can fail quietly
// instead of surfacing a loud error banner — the same treatment a denied
// Nodes watch already gets via its pre-flight check.
func TestSupervisor_ForbiddenGivesUpImmediatelyAndMarksForbidden(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)
	startPodsWatch(t, s)

	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("denied"))
	closed := msgs.WatchClosedMsg{
		Kind: msgs.KindPods, Context: "ctx1", Generation: 1, Err: forbidden,
	}

	upd, next, handled := s.Handle(closed)
	if !handled || next != nil {
		t.Fatal("expected no reconnect command for an RBAC denial")
	}
	if upd == nil || !upd.GaveUp || !upd.Forbidden {
		t.Fatalf("expected a GaveUp+Forbidden update, got %+v", upd)
	}
}

func TestSupervisor_EndpointsOverlayAndDedup(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)

	var svcWait tea.Cmd
	for _, cmd := range s.StartContext("ctx1") {
		opened := driveToOpened(t, s, cmd)
		_, next, _ := s.Handle(opened)
		if opened.Kind == msgs.KindServices {
			svcWait = next
		}
	}

	cluster.watchers[msgs.KindServices].Add(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default", ResourceVersion: "1"},
	})
	s.Handle(runCmd(t, svcWait))

	if !s.NeedsEndpoints("ctx1") {
		t.Fatal("expected endpoints to be needed before any fetch")
	}
	s.MarkEndpointsRequested("ctx1")
	if s.NeedsEndpoints("ctx1") {
		t.Fatal("expected no duplicate fetch while one is in flight")
	}

	s.SetEndpoints("ctx1", map[string][]string{"default/svc-a": {"10.0.0.2", "10.0.0.1"}})
	rows := s.Rows(msgs.KindServices)
	if len(rows) != 1 || rows[0][msgs.SvcKeyEndpointIPs] != "10.0.0.1,10.0.0.2" {
		t.Fatalf("expected sorted endpoint IPs overlaid on svc rows, got %+v", rows)
	}

	// A context change (StopContext) makes the fetch needed again.
	s.StopContext("ctx1")
	if !s.NeedsEndpoints("ctx1") {
		t.Fatal("expected endpoints needed again after the context was stopped")
	}
}

// TestSupervisor_StartContextIsOneWatchPerKindClusterWide guards the
// collapsed (kind, context) keying: StartContext must open exactly one
// watch per namespaced kind for a context — cluster-wide (namespace="")
// rather than one per checked namespace — since which namespaces are
// checked in the Namespaces pane is now a display-only filter applied
// downstream (MainPage), not something the Supervisor's watches key on.
func TestSupervisor_StartContextIsOneWatchPerKindClusterWide(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)

	cmds := s.StartContext("ctx1")
	wantKinds := len(msgs.Kinds()) - 1 // every kind except KindNodes
	if len(cmds) != wantKinds {
		t.Fatalf("expected %d start commands (one per namespaced kind), got %d", wantKinds, len(cmds))
	}

	if _, ok := s.states[stateKey{kind: msgs.KindPods, context: "ctx1"}]; !ok {
		t.Fatal("expected a single cluster-wide Pods watch state for ctx1")
	}
	if _, ok := s.states[stateKey{kind: msgs.KindNodes, context: "ctx1"}]; ok {
		t.Fatal("expected StartContext to skip cluster-scoped KindNodes")
	}
}

// TestStartContext_ExcludesNodes_StartKindAddsIt guards the Nodes gating:
// StartContext must never open a Nodes watch on its own (MainPage gates it
// behind a CanWatchNodes pre-flight check first), but StartKind must still
// be able to start it explicitly once that check clears.
func TestStartContext_ExcludesNodes_StartKindAddsIt(t *testing.T) {
	cluster := newFakeCluster()
	s := NewSupervisor(cluster)

	for _, cmd := range s.StartContext("ctx1") {
		msg := runCmd(t, cmd)
		if listMsg, ok := msg.(msgs.ListLoadedMsg); ok && listMsg.Kind == msgs.KindNodes {
			t.Fatal("expected StartContext to never open a Nodes watch")
		}
	}
	if _, ok := s.states[stateKey{kind: msgs.KindNodes, context: "ctx1"}]; ok {
		t.Fatal("expected no Nodes watch state after StartContext alone")
	}

	runCmd(t, s.StartKind(msgs.KindNodes, "ctx1"))
	if _, ok := s.states[stateKey{kind: msgs.KindNodes, context: "ctx1"}]; !ok {
		t.Fatal("expected StartKind to open a Nodes watch state")
	}
}

func TestBackoffDelay(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{5, 16 * time.Second},
		{6, 32 * time.Second},
		{20, maxBackoff},
	}
	for _, tc := range cases {
		if got := backoffDelay(tc.failures); got != tc.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}
