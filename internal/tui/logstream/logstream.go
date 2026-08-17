// Package logstream owns the full pod-log-stream lifecycle for the merged
// Log pane: opening a follow stream per targeted pod/container, reading it
// line by line, and tearing it down — mirroring watch.Supervisor's shape
// for the same reason: the caller (MainPage) shouldn't touch a stream, a
// scanner, or a generation counter directly. The Supervisor is the
// package's interface; everything else is implementation.
package logstream

import (
	"bufio"
	"context"
	"io"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// logTailLines is the number of pre-existing lines backfilled when a log
// stream opens, matching `kubectl logs -f --tail=N`.
const logTailLines = 200

// maxLogLineBytes bounds bufio.Scanner's per-line buffer so a single
// abnormally long log line (e.g. a huge JSON blob) can't abort the scan
// with bufio.ErrTooLong.
const maxLogLineBytes = 1024 * 1024

// Target identifies one pod/container source to be tailed.
type Target struct {
	Key                                string // context/namespace/pod/container
	Context, Namespace, Pod, Container string
}

// Streamer is the consumer-side seam to k8s.Client, satisfied by its
// StreamLogs method — a fake in tests, *k8s.Client in production.
type Streamer interface {
	StreamLogs(ctx context.Context, kubeContext, namespace, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error)
}

// sourceState is the live stream-plumbing state for one open source.
// generation is bumped (implicitly, by being replaced with a new
// sourceState) whenever a source is closed and reopened under the same
// key, guarding against a stale in-flight message from the superseded
// stream being adopted.
type sourceState struct {
	stream     io.ReadCloser
	scanner    *bufio.Scanner
	generation int
	cancel     context.CancelFunc
}

// Update reports what a handled log-stream message means for the UI: a new
// line for one source, or that source's stream ending.
type Update struct {
	SourceKey string
	Line      string
	// Closed is set when the source's stream ended, either cleanly (Err ==
	// nil) or because opening/reading it failed (Err != nil).
	Closed bool
	Err    error
}

// Supervisor owns every open log source's stream: opening (via Reconcile),
// reading (via Handle), and tearing down (via Reconcile or CloseAll). Its
// caller (MainPage) only decides which targets should be tailed and
// forwards the three log-stream messages to Handle.
type Supervisor struct {
	client  Streamer
	sources map[string]*sourceState
}

// New builds a Supervisor backed by client.
func New(client Streamer) *Supervisor {
	return &Supervisor{client: client, sources: make(map[string]*sourceState)}
}

// Reconcile opens sources newly present in targets, closes sources no
// longer targeted, and leaves unchanged sources running untouched. opened
// is the subset of targets newly opened by this call and closed is the set
// of source keys removed — the caller uses them to keep its own render
// model (e.g. models.LogPage's AddSource/RemoveSource) in sync before the
// returned cmds' results start arriving. An empty targets closes every
// source.
func (s *Supervisor) Reconcile(targets []Target) (opened []Target, closed []string, cmds []tea.Cmd) {
	targetSet := make(map[string]Target, len(targets))
	for _, t := range targets {
		targetSet[t.Key] = t
	}

	for key := range s.sources {
		if _, wanted := targetSet[key]; !wanted {
			s.closeSource(key)
			closed = append(closed, key)
		}
	}

	for key, t := range targetSet {
		if _, exists := s.sources[key]; exists {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.sources[key] = &sourceState{generation: 1, cancel: cancel}
		opened = append(opened, t)
		cmds = append(cmds, s.openCmd(ctx, t, 1))
	}
	return opened, closed, cmds
}

// CloseAll closes every currently open source — used when the Log pane
// itself closes. Safe to call when nothing is streaming.
func (s *Supervisor) CloseAll() {
	for key := range s.sources {
		s.closeSource(key)
	}
}

// closeSource cancels the source's context (a no-op if its stream already
// opened, and the fix for a stream that's still hung mid-open) and closes
// its stream (if any), then forgets it.
func (s *Supervisor) closeSource(key string) {
	st, ok := s.sources[key]
	if !ok {
		return
	}
	st.cancel()
	if st.stream != nil {
		st.stream.Close()
	}
	delete(s.sources, key)
}

// openCmd opens a following log stream for one target, backfilled with the
// last logTailLines lines, reporting the outcome as a LogStreamOpenedMsg or
// LogStreamClosedMsg carrying the generation it was issued under.
func (s *Supervisor) openCmd(ctx context.Context, t Target, generation int) tea.Cmd {
	return func() tea.Msg {
		opts := &corev1.PodLogOptions{
			Follow:    true,
			TailLines: ptr.To(int64(logTailLines)),
			Container: t.Container,
		}
		stream, err := s.client.StreamLogs(ctx, t.Context, t.Namespace, t.Pod, opts)
		if err != nil {
			return msgs.LogStreamClosedMsg{SourceKey: t.Key, Generation: generation, Err: err}
		}
		return msgs.LogStreamOpenedMsg{SourceKey: t.Key, Generation: generation, Stream: stream}
	}
}

// waitForLogLineCmd reads the next line from scanner and returns it as a
// LogLineMsg, or a LogStreamClosedMsg once the stream ends (scanner.Err()
// is nil on a clean EOF). Handle re-issues this command after each
// LogLineMsg to keep that source's read loop going.
func waitForLogLineCmd(sourceKey string, generation int, scanner *bufio.Scanner) tea.Cmd {
	return func() tea.Msg {
		if scanner.Scan() {
			return msgs.LogLineMsg{SourceKey: sourceKey, Generation: generation, Line: scanner.Text()}
		}
		return msgs.LogStreamClosedMsg{SourceKey: sourceKey, Generation: generation, Err: scanner.Err()}
	}
}

// newLogScanner wraps an opened log stream in a bufio.Scanner sized to
// tolerate abnormally long individual log lines.
func newLogScanner(stream io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)
	return scanner
}

// Handle processes one of the three log-stream messages, returning the
// resulting UI update (nil if the message was stale or needs no UI
// change), the next command to keep the stream going, and whether the
// message was a log-stream message at all.
func (s *Supervisor) Handle(msg tea.Msg) (*Update, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case msgs.LogStreamOpenedMsg:
		// Stale — this source has since been restarted or closed. Close the
		// stream rather than adopting it; other open sources are unaffected.
		st, ok := s.sources[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			msg.Stream.Close()
			return nil, nil, true
		}
		st.stream = msg.Stream
		st.scanner = newLogScanner(msg.Stream)
		return nil, waitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner), true

	case msgs.LogLineMsg:
		st, ok := s.sources[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return nil, nil, true
		}
		return &Update{SourceKey: msg.SourceKey, Line: msg.Line},
			waitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner), true

	case msgs.LogStreamClosedMsg:
		st, ok := s.sources[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return nil, nil, true
		}
		if st.stream != nil {
			st.stream.Close()
		}
		delete(s.sources, msg.SourceKey)
		return &Update{SourceKey: msg.SourceKey, Closed: true, Err: msg.Err}, nil, true
	}
	return nil, nil, false
}
