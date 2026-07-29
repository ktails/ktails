package watch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// Cluster is the seam to the Kubernetes client: everything the Supervisor
// needs from a cluster connection. *k8s.Client is the production adapter; a
// fake serving pre-canned watch.Interface streams (and List results) is the
// test adapter. Each Watch<Kind> has a matching List<Kind>, used for the
// fast initial paint before the watch opens — see start. Every call is made
// with the context's kubeconfig default namespace, or "" (cluster-wide) for
// a context with none pinned — see stateKey and StartContext.
type Cluster interface {
	WatchPods(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListPods(ctx context.Context, kubeContext, namespace string) ([]*corev1.Pod, error)
	WatchDeployments(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListDeployments(ctx context.Context, kubeContext, namespace string) ([]*appsv1.Deployment, error)
	WatchServices(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListServices(ctx context.Context, kubeContext, namespace string) ([]*corev1.Service, error)
	WatchConfigMaps(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListConfigMaps(ctx context.Context, kubeContext, namespace string) ([]*corev1.ConfigMap, error)
	WatchSecrets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListSecrets(ctx context.Context, kubeContext, namespace string) ([]*corev1.Secret, error)
	WatchJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListJobs(ctx context.Context, kubeContext, namespace string) ([]*batchv1.Job, error)
	WatchCronJobs(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListCronJobs(ctx context.Context, kubeContext, namespace string) ([]*batchv1.CronJob, error)
	WatchStatefulSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListStatefulSets(ctx context.Context, kubeContext, namespace string) ([]*appsv1.StatefulSet, error)
	WatchDaemonSets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListDaemonSets(ctx context.Context, kubeContext, namespace string) ([]*appsv1.DaemonSet, error)
	WatchIngresses(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListIngresses(ctx context.Context, kubeContext, namespace string) ([]*networkingv1.Ingress, error)
	WatchPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListPodDisruptionBudgets(ctx context.Context, kubeContext, namespace string) ([]*policyv1.PodDisruptionBudget, error)
	WatchHorizontalPodAutoscalers(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListHorizontalPodAutoscalers(ctx context.Context, kubeContext, namespace string) ([]*autoscalingv2.HorizontalPodAutoscaler, error)
	// WatchNodes/ListNodes ignore namespace — Nodes are cluster-scoped.
	WatchNodes(ctx context.Context, kubeContext, namespace string) (watch.Interface, error)
	ListNodes(ctx context.Context, kubeContext, namespace string) ([]*corev1.Node, error)
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
// and its WatchOpenedMsg landing), the local cache it's keeping in sync, and
// a consecutive-failure counter driving reconnect backoff (reset to 0 the
// moment any event is applied).
type watchState struct {
	generation int
	watcher    watch.Interface
	cache      rowCache
	failures   int
}

// stateKey identifies one watch stream: one per (kind, context), scoped to
// the context's kubeconfig default namespace (or every namespace, for a
// context with none pinned) — see Cluster's doc comment and StartContext.
// Within that scope, which namespaces are actually shown is a display-only
// concern handled by a local filter downstream (MainPage), not by the
// Supervisor — the Namespaces pane's checked-namespace set never starts,
// stops, or otherwise touches a watch.
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
	// Forbidden is set alongside GaveUp when the failure was an RBAC denial
	// (apierrors.IsForbidden) rather than exhausted reconnect attempts —
	// callers use it to fail quietly (clear the loading indicator, nothing
	// more) the same way a denied Nodes watch already does via its
	// CanWatchNodes pre-flight check, rather than surfacing a loud
	// context-wide error banner for a resource kind that's just not
	// accessible in this cluster.
	Forbidden bool
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

	// namespaces holds the kubeconfig default namespace each context's
	// watches/lists are scoped to ("" means cluster-wide) — set by
	// StartContext, consulted by start so a restart (Restart) or a
	// StartKind call added after the fact (KindNodes) reuses the same
	// scope without needing to thread a namespace through every caller.
	namespaces map[string]string

	// Lazily-fetched svc Endpoint IPs, overlaid onto service rows by Rows:
	// endpoints holds the fetched IPs per context (namespace/service name ->
	// IPs, namespace-qualified since a cluster-wide fetch can see the same
	// service name reused across namespaces); endpointsFetched records which
	// contexts have already been fetched (or have a fetch in flight), so
	// NeedsEndpoints can tell a fetch is still pending/valid without
	// refetching on every toggle.
	endpoints        map[string]map[string][]string
	endpointsFetched map[string]bool

	// Lazily, periodically-fetched Pods/Nodes CPU/Memory usage from
	// metrics.k8s.io, overlaid onto rows by Rows the same way endpoints is.
	// Unlike endpoints (fetched once per context+namespace on a wide-mode
	// toggle and kept forever), usage changes continuously, so MainPage
	// refetches it on a throttled timer rather than once — see
	// MainPage.fetchMetricsIfNeeded. podMetricsUnavailable/
	// nodeMetricsUnavailable record a context whose metrics fetch has ever
	// failed (most commonly: metrics-server isn't installed), so
	// fetchMetricsIfNeeded stops retrying that context rather than
	// re-erroring every tick forever.
	podMetrics             map[string]map[string]msgs.ResourceUsage // context -> "namespace/name" -> usage
	podMetricsUnavailable  map[string]bool
	nodeMetrics            map[string]map[string]msgs.ResourceUsage // context -> node name -> usage
	nodeMetricsUnavailable map[string]bool
}

func NewSupervisor(cluster Cluster) *Supervisor {
	return &Supervisor{
		cluster:                cluster,
		states:                 make(map[stateKey]*watchState),
		namespaces:             make(map[string]string),
		endpoints:              make(map[string]map[string][]string),
		endpointsFetched:       make(map[string]bool),
		podMetrics:             make(map[string]map[string]msgs.ResourceUsage),
		podMetricsUnavailable:  make(map[string]bool),
		nodeMetrics:            make(map[string]map[string]msgs.ResourceUsage),
		nodeMetricsUnavailable: make(map[string]bool),
	}
}

// StartContext opens one watch per namespaced resource kind for a context,
// scoped to namespace (the context's kubeconfig default namespace, or "" for
// cluster-wide — see AppState.DefaultNamespace). Scoping to the pinned
// namespace, rather than always going cluster-wide, matters for RBAC: a
// ServiceAccount granted edit only within its own namespace has no
// cluster-scoped list/watch verb at all, so a cluster-wide request is
// outright Forbidden even though the namespaced one would succeed.
// KindNodes (cluster-scoped) is deliberately excluded here — it's gated
// behind a permission pre-check (see StartKind and
// cmds.CheckNodesAccessCmd) rather than started unconditionally, since it's
// commonly restricted and opening a watch that will just fail is wasted work
// the check avoids. Restarts cleanly if already watched.
func (s *Supervisor) StartContext(kubeContext, namespace string) []tea.Cmd {
	s.namespaces[kubeContext] = namespace
	var cmds []tea.Cmd
	for _, kind := range msgs.Kinds() {
		if kind == msgs.KindNodes {
			continue
		}
		cmds = append(cmds, s.start(kind, kubeContext))
	}
	return cmds
}

// StartKind opens the watch for a single (kind, context) — exported for
// KindNodes, whose watch MainPage only starts once CheckNodesAccessCmd
// confirms it's allowed, rather than unconditionally as part of
// StartContext.
func (s *Supervisor) StartKind(kind msgs.ResourceKind, kubeContext string) tea.Cmd {
	return s.start(kind, kubeContext)
}

// start begins (or supersedes) the watch for one (kind, context): any open
// watcher is stopped, the generation is bumped so stale in-flight messages
// are dropped, and a fresh List command is returned — its result (see
// Handle's ListLoadedMsg case) seeds the cache for an immediate full paint
// and then opens the watch, rather than opening the watch directly and
// waiting for its synthetic Added replay to trickle rows in one at a time.
// The cache is created on first start and reused across restarts — List's
// seed is a full reset (see resourceCache.seed), and a subsequent watch
// replay is idempotent against the upsert-based cache apply either way.
func (s *Supervisor) start(kind msgs.ResourceKind, kubeContext string) tea.Cmd {
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
	return s.listCmd(kind, kubeContext, s.namespaces[kubeContext], st.generation)
}

// Restart force-restarts one kind's watches across the given contexts — the
// "r" key restarts only the active tab's kind, to avoid tripling API load on
// tabs the user isn't even looking at. Contexts not currently watched are
// skipped.
func (s *Supervisor) Restart(kind msgs.ResourceKind, contexts []string) []tea.Cmd {
	contextSet := make(map[string]bool, len(contexts))
	for _, c := range contexts {
		contextSet[c] = true
	}
	var cmds []tea.Cmd
	for key := range s.states {
		if key.kind == kind && contextSet[key.context] {
			cmds = append(cmds, s.start(kind, key.context))
		}
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
	for key, st := range s.states {
		if key.context != kubeContext {
			continue
		}
		st.generation++
		if st.watcher != nil {
			st.watcher.Stop()
		}
		delete(s.states, key)
	}
	delete(s.namespaces, kubeContext)
	delete(s.endpoints, kubeContext)
	delete(s.endpointsFetched, kubeContext)
	delete(s.podMetrics, kubeContext)
	delete(s.podMetricsUnavailable, kubeContext)
	delete(s.nodeMetrics, kubeContext)
	delete(s.nodeMetricsUnavailable, kubeContext)
}

// Watching reports whether any context is currently being watched — i.e.
// whether restarting a kind's watches could do anything at all.
func (s *Supervisor) Watching() bool {
	return len(s.states) > 0
}

// Rows rebuilds one kind's rows across every watched context, sorted by
// context for a stable cross-context order. Every row map is freshly
// allocated by the cache, so callers own the result outright — no defensive
// cloning needed anywhere downstream. Rows are unfiltered by namespace here
// — every namespace this context can see comes back; narrowing to the
// namespaces checked in the Namespaces pane is a display-only concern
// handled by the caller (MainPage), not the Supervisor.
func (s *Supervisor) Rows(kind msgs.ResourceKind) []msgs.RowData {
	var contexts []string
	for key := range s.states {
		if key.kind == kind {
			contexts = append(contexts, key.context)
		}
	}
	sort.Strings(contexts)

	var all []msgs.RowData
	for _, ctx := range contexts {
		rows := s.states[stateKey{kind: kind, context: ctx}].cache.rows(ctx)
		switch kind {
		case msgs.KindServices:
			s.overlayEndpoints(ctx, rows)
		case msgs.KindPods:
			s.overlayPodMetrics(ctx, rows)
		case msgs.KindNodes:
			s.overlayNodeMetrics(ctx, rows)
		}
		all = append(all, rows...)
	}
	return all
}

// Handle processes one of the four watch messages, returning the resulting
// UI update (nil if the message was stale or needs no UI change), the next
// command to keep the stream going, and whether the message was a watch
// message at all.
func (s *Supervisor) Handle(msg tea.Msg) (*Update, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case msgs.ListLoadedMsg:
		st, ok := s.current(msg.Kind, msg.Context, msg.Generation)
		if !ok {
			return nil, nil, true
		}
		if msg.Err != nil {
			// The List failed (e.g. an RBAC denial) — don't surface a
			// separate error here. The watch is opened anyway and will hit
			// the same failure, reported through the existing
			// WatchClosedMsg path (immediate give-up on Forbidden, backoff
			// otherwise) rather than duplicating that logic here.
			return nil, s.openCmd(msg.Kind, msg.Context, s.namespaces[msg.Context], msg.Generation, 0), true
		}
		st.cache.seed(msg.Objects)
		upd := &Update{Kind: msg.Kind, Context: msg.Context, RowsChanged: true}
		return upd, s.openCmd(msg.Kind, msg.Context, s.namespaces[msg.Context], msg.Generation, 0), true

	case msgs.WatchOpenedMsg:
		st, ok := s.current(msg.Kind, msg.Context, msg.Generation)
		if !ok {
			msg.Watcher.Stop()
			return nil, nil, true
		}
		st.watcher = msg.Watcher
		// Report loaded as soon as the watch is live, not on the first
		// applied event: a context with zero objects of this kind gets no
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
			err := fmt.Errorf("RBAC: not permitted to view %s for context '%s'",
				strings.ToLower(msg.Kind.Title()), msg.Context)
			return &Update{Kind: msg.Kind, Context: msg.Context, GaveUp: true, Err: err, Forbidden: true}, nil, true
		}
		st.failures++
		if st.failures > maxReconnectFailures {
			err := fmt.Errorf("failed to watch %s for context '%s' after %d attempts: %v",
				strings.ToLower(msg.Kind.Title()), msg.Context, st.failures, msg.Err)
			return &Update{Kind: msg.Kind, Context: msg.Context, GaveUp: true, Err: err}, nil, true
		}
		return nil, s.openCmd(msg.Kind, msg.Context, s.namespaces[msg.Context], msg.Generation, backoffDelay(st.failures)), true
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
	case msgs.KindPodDisruptionBudgets:
		return s.cluster.WatchPodDisruptionBudgets
	case msgs.KindHorizontalPodAutoscalers:
		return s.cluster.WatchHorizontalPodAutoscalers
	case msgs.KindNodes:
		return s.cluster.WatchNodes
	}
	return nil
}

// toObjects converts a concrete List() result (e.g. []*corev1.Pod) to the
// kind-erased []metav1.Object carried by ListLoadedMsg.
func toObjects[T metav1.Object](items []T) []metav1.Object {
	out := make([]metav1.Object, len(items))
	for i, v := range items {
		out[i] = v
	}
	return out
}

// listFn returns the Cluster method that lists kind, already adapted to the
// kind-erased []metav1.Object shape listCmd needs.
func (s *Supervisor) listFn(kind msgs.ResourceKind) func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
	switch kind {
	case msgs.KindPods:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListPods(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindDeployments:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListDeployments(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindServices:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListServices(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindConfigMaps:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListConfigMaps(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindSecrets:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListSecrets(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindJobs:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListJobs(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindCronJobs:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListCronJobs(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindStatefulSets:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListStatefulSets(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindDaemonSets:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListDaemonSets(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindIngresses:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListIngresses(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindPodDisruptionBudgets:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListPodDisruptionBudgets(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindHorizontalPodAutoscalers:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListHorizontalPodAutoscalers(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	case msgs.KindNodes:
		return func(ctx context.Context, kubeContext, namespace string) ([]metav1.Object, error) {
			items, err := s.cluster.ListNodes(ctx, kubeContext, namespace)
			return toObjects(items), err
		}
	}
	return nil
}

// listCmd issues the one-shot List() call for one (kind, context), scoped to
// namespace ("" for cluster-wide), reporting the outcome as a ListLoadedMsg
// carrying the generation it was issued under.
func (s *Supervisor) listCmd(kind msgs.ResourceKind, kubeContext, namespace string, generation int) tea.Cmd {
	list := s.listFn(kind)
	return func() tea.Msg {
		objs, err := list(context.Background(), kubeContext, namespace)
		return msgs.ListLoadedMsg{Kind: kind, Context: kubeContext, Generation: generation, Objects: objs, Err: err}
	}
}

// openCmd opens (after an optional backoff delay) a watch for one (kind,
// context), scoped to namespace ("" for cluster-wide), reporting the outcome
// as a WatchOpenedMsg or WatchClosedMsg carrying the generation it was
// issued under.
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
// this context — false once fetched (or requested).
func (s *Supervisor) NeedsEndpoints(kubeContext string) bool {
	return !s.endpointsFetched[kubeContext]
}

// MarkEndpointsRequested records that a fetch for this context is in flight
// (or done), so a second wide-mode toggle before the first fetch resolves
// doesn't dispatch a duplicate request.
func (s *Supervisor) MarkEndpointsRequested(kubeContext string) {
	s.endpointsFetched[kubeContext] = true
}

// ClearEndpointsRequested un-marks a context's fetch, letting the next
// wide-mode toggle retry — used when a fetch comes back with an error.
func (s *Supervisor) ClearEndpointsRequested(kubeContext string) {
	delete(s.endpointsFetched, kubeContext)
}

// SetEndpoints stores a resolved Endpoint IPs fetch for a context, keyed by
// "namespace/service name" (namespace-qualified since the fetch is
// cluster-wide — the same service name can exist in more than one
// namespace); Rows overlays it onto that context's service rows from then
// on.
func (s *Supervisor) SetEndpoints(kubeContext string, endpoints map[string][]string) {
	s.endpoints[kubeContext] = endpoints
	s.endpointsFetched[kubeContext] = true
}

// overlayEndpoints replaces the Endpoint IPs placeholder on freshly built
// service rows with the context's fetched IPs, if any, matched by
// "namespace/service name".
func (s *Supervisor) overlayEndpoints(kubeContext string, rows []msgs.RowData) {
	endpoints, ok := s.endpoints[kubeContext]
	if !ok {
		return
	}
	for _, row := range rows {
		name, _ := row[msgs.SvcKeyName].(string)
		namespace, _ := row[msgs.SvcKeyNamespace].(string)
		row[msgs.SvcKeyEndpointIPs] = formatEndpointIPs(endpoints[namespace+"/"+name])
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

// PodMetricsUnavailable reports whether a Pods metrics fetch has ever failed
// for this context (most commonly: no metrics-server installed) —
// MainPage.fetchMetricsIfNeeded stops retrying once true, rather than
// re-erroring every tick forever.
func (s *Supervisor) PodMetricsUnavailable(kubeContext string) bool {
	return s.podMetricsUnavailable[kubeContext]
}

// MarkPodMetricsUnavailable records that this context's Pods metrics fetch
// failed.
func (s *Supervisor) MarkPodMetricsUnavailable(kubeContext string) {
	s.podMetricsUnavailable[kubeContext] = true
}

// SetPodMetrics stores a resolved CPU/Memory usage fetch for a context,
// keyed by "namespace/name"; Rows overlays it onto that context's Pods rows
// from then on, replacing whatever was there before (a successful refetch
// naturally clears any earlier MarkPodMetricsUnavailable, since a fresh
// fetch call only reaches here on success).
func (s *Supervisor) SetPodMetrics(kubeContext string, usage map[string]msgs.ResourceUsage) {
	s.podMetrics[kubeContext] = usage
	delete(s.podMetricsUnavailable, kubeContext)
}

// overlayPodMetrics replaces the CPU/Memory placeholder on freshly built pod
// rows with the context's fetched usage, if any, matched by
// "namespace/name". Pods with no matching entry (e.g. metrics-server hasn't
// reported for a just-started pod yet) keep the "-" placeholder set by
// podRow.
func (s *Supervisor) overlayPodMetrics(kubeContext string, rows []msgs.RowData) {
	usage, ok := s.podMetrics[kubeContext]
	if !ok {
		return
	}
	for _, row := range rows {
		name, _ := row[msgs.PodKeyName].(string)
		namespace, _ := row[msgs.PodKeyNamespace].(string)
		u, found := usage[namespace+"/"+name]
		if !found {
			continue
		}
		row[msgs.PodKeyCPU] = u.CPU
		row[msgs.PodKeyMemory] = u.Memory
	}
}

// NodeMetricsUnavailable/MarkNodeMetricsUnavailable/SetNodeMetrics/
// overlayNodeMetrics are PodMetricsUnavailable/MarkPodMetricsUnavailable/
// SetPodMetrics/overlayPodMetrics's Nodes counterparts, keyed by node name
// instead of "namespace/name" — Nodes are cluster-scoped.
func (s *Supervisor) NodeMetricsUnavailable(kubeContext string) bool {
	return s.nodeMetricsUnavailable[kubeContext]
}

func (s *Supervisor) MarkNodeMetricsUnavailable(kubeContext string) {
	s.nodeMetricsUnavailable[kubeContext] = true
}

func (s *Supervisor) SetNodeMetrics(kubeContext string, usage map[string]msgs.ResourceUsage) {
	s.nodeMetrics[kubeContext] = usage
	delete(s.nodeMetricsUnavailable, kubeContext)
}

func (s *Supervisor) overlayNodeMetrics(kubeContext string, rows []msgs.RowData) {
	usage, ok := s.nodeMetrics[kubeContext]
	if !ok {
		return
	}
	for _, row := range rows {
		name, _ := row[msgs.NodeKeyName].(string)
		u, found := usage[name]
		if !found {
			continue
		}
		row[msgs.NodeKeyCPU] = u.CPU
		row[msgs.NodeKeyMemory] = u.Memory
	}
}
