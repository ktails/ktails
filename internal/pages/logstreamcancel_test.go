package pages

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ktails/ktails/internal/tui/models"
)

// fakeReadCloser lets a test assert Close() was called without a live pod
// log stream.
type fakeReadCloser struct {
	closed bool
}

func (f *fakeReadCloser) Read(p []byte) (int, error) { return 0, io.EOF }
func (f *fakeReadCloser) Close() error {
	f.closed = true
	return nil
}

// newTestLogStreamState builds a logStreamState with a recorded cancel so
// tests can assert it was invoked.
func newTestLogStreamState(stream io.ReadCloser) (*logStreamState, *bool) {
	called := false
	st := &logStreamState{generation: 1, stream: stream}
	ctx, cancel := context.WithCancel(context.Background())
	st.ctx = ctx
	st.cancel = func() {
		called = true
		cancel()
	}
	return st, &called
}

// TestCloseLogSource_CancelsContextAndClosesStream covers Step 2's fix for
// StreamLogs' formerly uncancellable context.Background(): closing a pane
// must be able to reap a hung-open stream via cancel, not just Close() an
// already-opened one.
func TestCloseLogSource_CancelsContextAndClosesStream(t *testing.T) {
	stream := &fakeReadCloser{}
	st, cancelCalled := newTestLogStreamState(stream)

	m := &MainPage{
		logStreams: map[string]*logStreamState{"key": st},
		podLogs:    models.NewLogPage(),
	}
	m.closeLogSource("key")

	if !*cancelCalled {
		t.Fatal("expected closeLogSource to call cancel")
	}
	if !stream.closed {
		t.Fatal("expected closeLogSource to close the stream")
	}
	if _, ok := m.logStreams["key"]; ok {
		t.Fatal("expected closeLogSource to remove the source from logStreams")
	}
}

// TestCloseLogSource_CancelsHungOpen covers the case the fix specifically
// targets: a source whose stream never opened (StreamLogs' req.Stream(ctx)
// is still blocked). cancel must still be called even though there's no
// stream to Close().
func TestCloseLogSource_CancelsHungOpen(t *testing.T) {
	cancelCalled := false
	ctx, cancel := context.WithCancel(context.Background())
	st := &logStreamState{
		generation: 1,
		ctx:        ctx,
		cancel: func() {
			cancelCalled = true
			cancel()
		},
	}

	m := &MainPage{
		logStreams: map[string]*logStreamState{"key": st},
		podLogs:    models.NewLogPage(),
	}
	m.closeLogSource("key")

	if !cancelCalled {
		t.Fatal("expected closeLogSource to call cancel even with no stream opened yet")
	}
	if err := st.ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context to be canceled, got %v", err)
	}
}

// TestStopLogStream_CancelsAllSources mirrors closeLogSource's behavior for
// the closeLogs() (Log pane close) path.
func TestStopLogStream_CancelsAllSources(t *testing.T) {
	stream := &fakeReadCloser{}
	st, cancelCalled := newTestLogStreamState(stream)

	m := &MainPage{
		logStreams: map[string]*logStreamState{"key": st},
	}
	m.stopLogStream()

	if !*cancelCalled {
		t.Fatal("expected stopLogStream to call cancel")
	}
	if !stream.closed {
		t.Fatal("expected stopLogStream to close the stream")
	}
	if len(m.logStreams) != 0 {
		t.Fatalf("expected logStreams to be empty, got %d entries", len(m.logStreams))
	}
}
