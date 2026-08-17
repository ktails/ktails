package logstream

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// fakeStreamer is the test adapter at the Streamer seam. If block is true,
// StreamLogs doesn't return until ctx is done, simulating a hung-but-
// connected apiserver (see TestReconcile_CancelsHungOpen). Otherwise it
// returns body immediately. Deliberately records no call history — tests
// here may invoke StreamLogs concurrently (see TestCloseAll_CancelsEverySource),
// and this fake has nothing to assert about the calls themselves beyond
// their outcome.
type fakeStreamer struct {
	block bool
	body  string
	err   error
}

func (f *fakeStreamer) StreamLogs(ctx context.Context, kubeContext, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.body)), nil
}

func targetA() Target {
	return Target{Key: "ctx1/default/pod-a/app", Context: "ctx1", Namespace: "default", Pod: "pod-a", Container: "app"}
}

// TestReconcile_OpensNewTarget guards the basic open path: a newly targeted
// source is returned in opened, and its cmd resolves to a
// LogStreamOpenedMsg once run.
func TestReconcile_OpensNewTarget(t *testing.T) {
	streamer := &fakeStreamer{body: "line-1\nline-2\n"}
	s := New(streamer)

	opened, closed, cmds := s.Reconcile([]Target{targetA()})
	if len(opened) != 1 || opened[0].Key != targetA().Key {
		t.Fatalf("expected targetA in opened, got %+v", opened)
	}
	if len(closed) != 0 {
		t.Fatalf("expected nothing closed, got %v", closed)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 open command, got %d", len(cmds))
	}

	msg := cmds[0]()
	opened_, ok := msg.(msgs.LogStreamOpenedMsg)
	if !ok {
		t.Fatalf("expected LogStreamOpenedMsg, got %T", msg)
	}
	if opened_.SourceKey != targetA().Key {
		t.Fatalf("expected SourceKey %q, got %q", targetA().Key, opened_.SourceKey)
	}
}

// TestReconcile_UnchangedSourceLeftRunning guards that reconciling with the
// same target set twice doesn't reopen an already-open source.
func TestReconcile_UnchangedSourceLeftRunning(t *testing.T) {
	streamer := &fakeStreamer{body: ""}
	s := New(streamer)

	s.Reconcile([]Target{targetA()})
	opened, closed, cmds := s.Reconcile([]Target{targetA()})
	if len(opened) != 0 || len(closed) != 0 || len(cmds) != 0 {
		t.Fatalf("expected a no-op reconcile for an unchanged target set, got opened=%v closed=%v cmds=%d", opened, closed, len(cmds))
	}
}

// TestReconcile_ClosesUntargetedSource guards the diff-close half: a source
// no longer present in targets is reported in closed.
func TestReconcile_ClosesUntargetedSource(t *testing.T) {
	streamer := &fakeStreamer{body: ""}
	s := New(streamer)

	s.Reconcile([]Target{targetA()})
	opened, closed, cmds := s.Reconcile(nil)
	if len(opened) != 0 || len(cmds) != 0 {
		t.Fatalf("expected nothing opened, got opened=%v cmds=%d", opened, len(cmds))
	}
	if len(closed) != 1 || closed[0] != targetA().Key {
		t.Fatalf("expected targetA closed, got %v", closed)
	}
}

// TestReconcile_CancelsHungOpen guards the fix Step 2 gave StreamLogs: the
// *open* itself (not just an already-opened stream) must be cancellable, so
// closing a pane can reap a hung-but-connected apiserver instead of leaking
// the goroutine forever.
func TestReconcile_CancelsHungOpen(t *testing.T) {
	streamer := &fakeStreamer{block: true}
	s := New(streamer)

	_, _, cmds := s.Reconcile([]Target{targetA()})
	if len(cmds) != 1 {
		t.Fatalf("expected 1 open command, got %d", len(cmds))
	}

	result := make(chan tea.Msg, 1)
	go func() { result <- cmds[0]() }()

	// Give the goroutine time to actually reach the blocking StreamLogs
	// call before removing the target — otherwise this wouldn't exercise
	// the hung-open case at all.
	time.Sleep(20 * time.Millisecond)

	_, closed, _ := s.Reconcile(nil)
	if len(closed) != 1 || closed[0] != targetA().Key {
		t.Fatalf("expected targetA closed, got %v", closed)
	}

	select {
	case msg := <-result:
		closedMsg, ok := msg.(msgs.LogStreamClosedMsg)
		if !ok {
			t.Fatalf("expected LogStreamClosedMsg, got %T", msg)
		}
		if !errors.Is(closedMsg.Err, context.Canceled) {
			t.Fatalf("expected a context.Canceled-wrapped error, got %v", closedMsg.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the hung open to be canceled and return, but it never did")
	}
}

// TestHandle_OpenedLineAndClosedFlow guards the read loop end to end: an
// opened stream's lines surface as Updates, and EOF closes it.
func TestHandle_OpenedLineAndClosedFlow(t *testing.T) {
	streamer := &fakeStreamer{body: "line-1\nline-2\n"}
	s := New(streamer)

	_, _, cmds := s.Reconcile([]Target{targetA()})
	openedMsg := cmds[0]()

	upd, waitCmd, handled := s.Handle(openedMsg)
	if !handled || upd != nil || waitCmd == nil {
		t.Fatalf("expected a silent adopt with a wait command, got upd=%v waitCmd=%v handled=%v", upd, waitCmd, handled)
	}

	lineMsg := waitCmd()
	upd, waitCmd, handled = s.Handle(lineMsg)
	if !handled || upd == nil || upd.Line != "line-1" || waitCmd == nil {
		t.Fatalf("expected an Update with line-1 and a next wait command, got upd=%+v handled=%v", upd, handled)
	}

	lineMsg = waitCmd()
	upd, waitCmd, handled = s.Handle(lineMsg)
	if !handled || upd == nil || upd.Line != "line-2" {
		t.Fatalf("expected an Update with line-2, got upd=%+v", upd)
	}

	closedMsg := waitCmd()
	upd, next, handled := s.Handle(closedMsg)
	if !handled || upd == nil || !upd.Closed || next != nil {
		t.Fatalf("expected a Closed update with no next command, got upd=%+v next=%v", upd, next)
	}
}

// TestHandle_StaleOpenedMsgIsClosedNotAdopted guards the same
// already-superseded-source protection the pre-refactor MainPage code had:
// a message for a source no longer tracked (closed, or never opened by
// this Supervisor) must close the incoming stream rather than adopt it.
func TestHandle_StaleOpenedMsgIsClosedNotAdopted(t *testing.T) {
	s := New(&fakeStreamer{})
	closer := &closeRecorder{}

	upd, cmd, handled := s.Handle(msgs.LogStreamOpenedMsg{SourceKey: "gone", Generation: 1, Stream: closer})
	if !handled || upd != nil || cmd != nil {
		t.Fatalf("expected a silent no-op, got upd=%v cmd=%v", upd, cmd)
	}
	if !closer.closed {
		t.Fatal("expected the stale stream to be closed")
	}
}

// TestCloseAll_CancelsEverySource guards the Log-pane-close path: every
// open source's context must be canceled, including ones whose stream is
// still hung mid-open.
func TestCloseAll_CancelsEverySource(t *testing.T) {
	streamer := &fakeStreamer{block: true}
	s := New(streamer)

	targetB := Target{Key: "ctx1/default/pod-b/app", Context: "ctx1", Namespace: "default", Pod: "pod-b", Container: "app"}
	_, _, cmds := s.Reconcile([]Target{targetA(), targetB})
	if len(cmds) != 2 {
		t.Fatalf("expected 2 open commands, got %d", len(cmds))
	}

	results := make(chan tea.Msg, 2)
	for _, cmd := range cmds {
		go func(c func() tea.Msg) { results <- c() }(cmd)
	}
	time.Sleep(20 * time.Millisecond)

	s.CloseAll()

	for i := 0; i < 2; i++ {
		select {
		case msg := <-results:
			closedMsg, ok := msg.(msgs.LogStreamClosedMsg)
			if !ok {
				t.Fatalf("expected LogStreamClosedMsg, got %T", msg)
			}
			if !errors.Is(closedMsg.Err, context.Canceled) {
				t.Fatalf("expected a context.Canceled-wrapped error, got %v", closedMsg.Err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected CloseAll to cancel every hung-open source, but at least one never returned")
		}
	}
}

// closeRecorder is a minimal io.ReadCloser that records whether Close was
// called, for TestHandle_StaleOpenedMsgIsClosedNotAdopted.
type closeRecorder struct {
	closed bool
}

func (c *closeRecorder) Read(p []byte) (int, error) { return 0, io.EOF }
func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}
