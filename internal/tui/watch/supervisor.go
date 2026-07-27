package watch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// Cluster is the seam to the Kubernetes client: everything the Supervisor
// needs from a cluster connection. *k8s.Client is the production adapter; a
// fake serving pre-canned watch.Interface streams is the test adapter.
type Cluster interface {
	WatchPods(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchDeployments(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchServices(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchConfigMaps(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchSecrets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchCronJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchStatefulSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchDaemonSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	WatchIngresses(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	// WatchNodes ignores namespace — Nodes are cluster-scoped.
	WatchNodes(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
}

// maxReconnectFailures is how many consecutive reconnect failures a
// (kind, context) watch tolerates before giving up and surfacing an error,
// rather than backing off forever against a genuinely broken context (bad
// creds, deleted context, unreachable apiserver).
const maxReconnectFailures = 6

// maxBackoff caps the exponential reconnect delay (1s, 2s, 4s, 8s, 16s, ...)
// so a flaky-but-recovering connection doesn't end up waiting minutes
// between attempts.
const maxBackoff = 32 * time.Second

// backoffDelay returns the reconnect delay for the Nth consecutive failure
// (1-indexed): 1s, 2s, 4s, 8s, 16s, capped at maxBackoff.
func backoffDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := time.Second << uint(failures-1)
	if delay <= 0 || delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

// watchState is the live plumbing state for one (kind, context) Watch()
// stream: the current generation (bumped on every restart, guarding against
// stale in-flight messages), the open watcher itself (nil between a restart
// and its WatchOpenedMsg landing), the local cache it's keeping in sync, the
// namespace it watches, and a consecutive-failure counter driving reconnect
// backoff (reset to 0 the moment any event is applied).
type watchState struct {
	generation int
	watcher    watch.Interface
	cache      rowCache
	namespace  string
	failures   int
}

type stateKey struct {
	kind    msgs.ResourceKind
	context string
}

// Update reports what a handled watch message means for the UI: either a
// fresh row set for one (kind, context), or a permanently failed watch.
type Update struct {
	Kind    msgs.ResourceKind
	Context string
	// RowsChanged fires both when a watch first opens (Rows(Kind) may be
	// empty — that's a valid loaded state) and on every applied event
	// afterward; see the WatchOpenedMsg case in Handle for why it can't wait
	// for an event that might never come.
	RowsChanged bool
	// GaveUp is set when reconnect attempts are exhausted; Err carries the
	// last failure.
	GaveUp bool
	Err    error
}

// Supervisor owns every watch stream, cache, and reconnect decision. Its
// caller (MainPage) only starts/stops contexts, forwards the three watch
// messages to Handle, and reads rows back out — it never touches a
// watch.Interface, a generation counter, or a backoff delay.
//
// All methods must be called from the bubbletea update loop; only the caches
// (which take their own locks) are touched from command goroutines.
type Supervisor struct {
	cluster Cluster
	states  map[stateKey]*watchState

	// Lazily-fetched svc Endpoint IPs, overlaid onto service rows by Rows:
	// endpoints holds the fetched IPs per context (service name -> IPs);
	// endpointsFetchedNS records which namespace they were fetched for, so
	// NeedsEndpoints can tell a fetch is still pending/valid for the
	// context's *current* namespace without refetching on every toggle.
	endpoints          map[string]map[string][]string
	endpointsFetchedNS map[string]string
}

func NewSupervisor(cluster Cluster) *Supervisor {
	return &Supervisor{
		cluster:            cluster,
		states:             make(map[stateKey]*watchState),
		endpoints:          make(map[string]map[string][]string),
		endpointsFetchedNS: make(map[string]string),
	}
}

// StartContext opens watches for every resource kind against one context,
// returning the commands that open them. Restarts cleanly if the context is
// already being watched.
func (s *Supervisor) StartContext(kubeContext, namespace string) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(msgs.Kinds()))
	for _, kind := range msgs.Kinds() {
		cmds = append(cmds, s.start(kind, kubeContext, namespace))
	}
	return cmds
}

// start begins (or supersedes) the watch for one (kind, context): any open
// watcher is stopped, the generation is bumped so stale in-flight messages
// are dropped, and a fresh open command is returned. The cache is created on
// first start and reused across restarts — a fresh watch's Added replay is
// idempotent against the upsert-based cache apply.
func (s *Supervisor) start(kind msgs.ResourceKind, kubeContext, namespace string) tea.Cmd {
	key := stateKey{kind: kind, context: kubeContext}
	st, ok := s.states[key]
	if !ok {
		st = &watchState{cache: newCacheFor(kind)}
		s.states[key] = st
	}
	if st.watcher != nil {
		st.watcher.Stop()
		st.watcher = nil
	}
	st.generation++
	st.failures = 0
	st.namespace = namespace
	return s.openCmd(kind, kubeContext, namespace, st.generation, 0)
}

// Restart force-restarts one kind's watch across the given contexts — the
// "r" key restarts only the active tab's kind, to avoid tripling API load on
// tabs the user isn't even looking at. Contexts not currently watched are
// skipped.
func (s *Supervisor) Restart(kind msgs.ResourceKind, contexts []string) []tea.Cmd {
	var cmds []tea.Cmd
	for _, kubeContext := range contexts {
		st, ok := s.states[stateKey{kind: kind, context: kubeContext}]
		if !ok {
			continue
		}
		cmds = append(cmds, s.start(kind, kubeContext, st.namespace))
	}
	return cmds
}

// StopContext stops and forgets every kind's watch for one context — called
// on context deselect. watch.Interface.Stop() is guaranteed to close
// ResultChan(), so any goroutine blocked in a wait command's receive
// unblocks cleanly rather than leaking; bumping the generation first means
// that goroutine's eventual message is dropped as stale even if it was
// already past the blocking receive.
func (s *Supervisor) StopContext(kubeContext string) {
	for _, kind := range msgs.Kinds() {
		key := stateKey{kind: kind, context: kubeContext}
		st, ok := s.states[key]
		if !ok {
			continue
		}
		st.generation++
		if st.watcher != nil {
			st.watcher.Stop()
		}
		delete(s.states, key)
	}
	delete(s.endpoints, kubeContext)
	delete(s.endpointsFetchedNS, kubeContext)
}

// Watching reports whether any context is currently being watched — i.e.
// whether restarting a kind's watches could do anything at all.
func (s *Supervisor) Watching() bool {
	return len(s.states) > 0
}

// Rows rebuilds one kind's rows across every watched context, sorted by
// context name for a stable cross-context order. Every row map is freshly
// allocated by the cache, so callers own the result outright — no defensive
// cloning needed anywhere downstream.
func (s *Supervisor) Rows(kind msgs.ResourceKind) []msgs.RowData {
	contexts := make([]string, 0, len(s.states))
	for key := range s.states {
		if key.kind == kind {
			contexts = append(contexts, key.context)
		}
	}
	sort.Strings(contexts)

	var all []msgs.RowData
	for _, kubeContext := range contexts {
		rows := s.states[stateKey{kind: kind, context: kubeContext}].cache.rows(kubeContext)
		if kind == msgs.KindServices {
			s.overlayEndpoints(kubeContext, rows)
		}
		all = append(all, rows...)
	}
	return all
}

// Handle processes one of the three watch messages, returning the resulting
// UI update (nil if the message was stale or needs no UI change), the next
// command to keep the stream going, and whether the message was a watch
// message at all.
func (s *Supervisor) Handle(msg tea.Msg) (*Update, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case msgs.WatchOpenedMsg:
		st, ok := s.current(msg.Kind, msg.Context, msg.Generation)
		if !ok {
			msg.Watcher.Stop()
			return nil, nil, true
		}
		st.watcher = msg.Watcher
		// Report loaded as soon as the watch is live, not on the first
		// applied event: a namespace with zero objects of this kind gets no
		// synthetic replay at all (Kubernetes only replays what exists), so
		// waiting for an event here would wait forever. Rows(kind) already
		// returns an empty slice correctly when the cache is empty — an
		// empty tab is a valid loaded state, not a still-loading one.
		upd := &Update{Kind: msg.Kind, Context: msg.Context, RowsChanged: true}
		return upd, s.waitCmd(msg.Kind, msg.Context, msg.Generation, msg.Watcher, st.cache), true

	case msgs.WatchEventMsg:
		st, ok := s.current(msg.Kind, msg.Context, msg.Generation)
		if !ok {
			return nil, nil, true
		}
		st.failures = 0
		upd := &Update{Kind: msg.Kind, Context: msg.Context, RowsChanged: true}
		return upd, s.waitCmd(msg.Kind, msg.Context, msg.Generation, st.watcher, st.cache), true

	case msgs.WatchClosedMsg:
		st, ok := s.current(msg.Kind, msg.Context, msg.Generation)
		if !ok {
			return nil, nil, true
		}
		st.watcher = nil
		// RBAC denials don't heal with a retry — the ServiceAccount either has
		// the list/watch verb on this resource or it doesn't — so give up
		// immediately instead of burning through backoff attempts first.
		if apierrors.IsForbidden(msg.Err) {
			err := fmt.Errorf("RBAC: not permitted to view %s in context '%s'",
				strings.ToLower(msg.Kind.Title()), msg.Context)
			return &Update{Kind: msg.Kind, Context: msg.Context, GaveUp: true, Err: err}, nil, true
		}
		st.failures++
		if st.failures > maxReconnectFailures {
			err := fmt.Errorf("failed to watch %s for context '%s' after %d attempts: %v",
				strings.ToLower(msg.Kind.Title()), msg.Context, st.failures, msg.Err)
			return &Update{Kind: msg.Kind, Context: msg.Context, GaveUp: true, Err: err}, nil, true
		}
		return nil, s.openCmd(msg.Kind, msg.Context, st.namespace, msg.Generation, backoffDelay(st.failures)), true
	}
	return nil, nil, false
}

// current returns the live state for (kind, context) only if generation is
// still the current one — the single staleness check every handler shares.
func (s *Supervisor) current(kind msgs.ResourceKind, kubeContext string, generation int) (*watchState, bool) {
	st, ok := s.states[stateKey{kind: kind, context: kubeContext}]
	if !ok || st.generation != generation {
		return nil, false
	}
	return st, true
}

// watchFn returns the Cluster method that opens a watch for kind.
func (s *Supervisor) watchFn(kind msgs.ResourceKind) func(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	switch kind {
	case msgs.KindPods:
		return s.cluster.WatchPods
	case msgs.KindDeployments:
		return s.cluster.WatchDeployments
	case msgs.KindServices:
		return s.cluster.WatchServices
	case msgs.KindConfigMaps:
		return s.cluster.WatchConfigMaps
	case msgs.KindSecrets:
		return s.cluster.WatchSecrets
	case msgs.KindJobs:
		return s.cluster.WatchJobs
	case msgs.KindCronJobs:
		return s.cluster.WatchCronJobs
	case msgs.KindStatefulSets:
		return s.cluster.WatchStatefulSets
	case msgs.KindDaemonSets:
		return s.cluster.WatchDaemonSets
	case msgs.KindIngresses:
		return s.cluster.WatchIngresses
	case msgs.KindNodes:
		return s.cluster.WatchNodes
	}
	return nil
}

// openCmd opens (after an optional backoff delay) a watch for one
// (kind, context), reporting the outcome as a WatchOpenedMsg or
// WatchClosedMsg carrying the generation it was issued under.
func (s *Supervisor) openCmd(kind msgs.ResourceKind, kubeContext, namespace string, generation int, delay time.Duration) tea.Cmd {
	open := s.watchFn(kind)
	return func() tea.Msg {
		if delay > 0 {
			time.Sleep(delay)
		}
		w, err := open(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.WatchClosedMsg{Kind: kind, Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.WatchOpenedMsg{Kind: kind, Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// waitCmd blocks for the next event on watcher, applies it to cache, then
// non-blockingly drains any additional already-buffered events (collapsing
// bursts — e.g. the initial full-list replay, or a mass rollout — into fewer
// UI updates instead of one message per object) before returning a freshly
// rebuilt row set. Handle re-issues this command after each WatchEventMsg to
// keep the read loop going.
func (s *Supervisor) waitCmd(kind msgs.ResourceKind, kubeContext string, generation int, watcher watch.Interface, cache rowCache) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-watcher.ResultChan()
		if !ok {
			return msgs.WatchClosedMsg{Kind: kind, Context: kubeContext, Generation: generation}
		}
		if err := cache.apply(ev); err != nil {
			return msgs.WatchClosedMsg{Kind: kind, Context: kubeContext, Generation: generation, Err: err}
		}

		for {
			select {
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return msgs.WatchEventMsg{Kind: kind, Context: kubeContext, Generation: generation, Rows: cache.rows(kubeContext)}
				}
				if err := cache.apply(ev); err != nil {
					return msgs.WatchClosedMsg{Kind: kind, Context: kubeContext, Generation: generation, Err: err}
				}
			default:
				return msgs.WatchEventMsg{Kind: kind, Context: kubeContext, Generation: generation, Rows: cache.rows(kubeContext)}
			}
		}
	}
}

// NeedsEndpoints reports whether svc Endpoint IPs still need fetching for
// this context's given namespace — false once fetched (or requested) for
// that exact namespace, true again if the namespace has since changed.
func (s *Supervisor) NeedsEndpoints(kubeContext, namespace string) bool {
	return s.endpointsFetchedNS[kubeContext] != namespace
}

// MarkEndpointsRequested records that a fetch for this context+namespace is
// in flight (or done), so a second wide-mode toggle before the first fetch
// resolves doesn't dispatch a duplicate request.
func (s *Supervisor) MarkEndpointsRequested(kubeContext, namespace string) {
	s.endpointsFetchedNS[kubeContext] = namespace
}

// ClearEndpointsRequested un-marks a context's fetch, letting the next
// wide-mode toggle retry — used when a fetch comes back with an error.
func (s *Supervisor) ClearEndpointsRequested(kubeContext string) {
	delete(s.endpointsFetchedNS, kubeContext)
}

// SetEndpoints stores a resolved Endpoint IPs fetch for a context+namespace;
// Rows overlays it onto that context's service rows from then on.
func (s *Supervisor) SetEndpoints(kubeContext, namespace string, endpoints map[string][]string) {
	s.endpoints[kubeContext] = endpoints
	s.endpointsFetchedNS[kubeContext] = namespace
}

// overlayEndpoints replaces the Endpoint IPs placeholder on freshly built
// service rows with the context's fetched IPs, if any.
func (s *Supervisor) overlayEndpoints(kubeContext string, rows []msgs.RowData) {
	endpoints, ok := s.endpoints[kubeContext]
	if !ok {
		return
	}
	for _, row := range rows {
		name, _ := row[msgs.SvcKeyName].(string)
		row[msgs.SvcKeyEndpointIPs] = formatEndpointIPs(endpoints[name])
	}
}

// formatEndpointIPs renders a service's endpoint IPs as a sorted,
// deterministic comma-separated list, "-" when the service currently has no
// endpoints (e.g. no matching/ready pods).
func formatEndpointIPs(ips []string) string {
	if len(ips) == 0 {
		return "-"
	}
	sorted := make([]string, len(ips))
	copy(sorted, ips)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}
