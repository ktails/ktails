// Package state holds per-context UI status: which contexts are selected
// (and their namespaces), what's loading, what has loaded at least once, and
// any per-context error. Resource rows themselves live in the watch
// Supervisor's caches — this module deliberately does not store them.
package state

import (
	"image/color"
	"sync"

	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

type AppState struct {
	// Selected contexts and their checked namespaces — always at least one
	// entry per context (seeded from kubeconfig's default on AddContext),
	// but a context can have several checked at once via the Namespaces
	// pane. This is a display-only filter now (every context's watch is
	// cluster-wide regardless — see watch.Supervisor's stateKey doc
	// comment): it narrows which of a context's rows the resource tables
	// show, it never starts or stops anything.
	SelectedContexts map[string][]string // context -> checked namespaces

	// allNamespaces marks a context as showing every namespace unfiltered,
	// distinct from SelectedContexts holding zero checked namespaces — see
	// NamespacesInfo's "a" (toggle-all) key. Takes priority over
	// SelectedContexts for that context's row filter when true.
	allNamespaces map[string]bool

	// Loading states per resource kind
	loading map[msgs.ResourceKind]map[string]bool // kind -> context -> isLoading

	// Errors
	Errors map[string]string // context -> error message

	// Contexts that have completed at least one successful load cycle
	LoadedContexts map[string]bool

	// loadedKinds tracks, per context, which of requiredForLoaded's kinds
	// have each delivered a first successful load — LoadedContexts only
	// flips true once every required kind has (see MarkLoaded).
	loadedKinds map[string]map[msgs.ResourceKind]bool

	// kindLoaded tracks, per resource kind, which contexts have delivered a
	// first successful load for that kind specifically — the per-tab
	// counterpart to loadedKinds/LoadedContexts. Unlike LoadedContexts
	// (gated on Deployments+Pods together, for the Contexts pane checkmark),
	// this lets each tab unlock independently as its own List/watch
	// settles, so switching to e.g. the Services tab doesn't wait on
	// Deployments finishing first.
	kindLoaded map[msgs.ResourceKind]map[string]bool

	// contextColors assigns each context its identity colour (styles.IdentityColor)
	// the first time it's added, in rotation order. Entries are never removed on
	// RemoveContext so a context re-added within the same session keeps its colour.
	contextColors map[string]color.Color
	nextColorIdx  int

	// Mutex to protect concurrent access
	mu sync.RWMutex
}

// Snapshot captures a read-only view of application state data.
type Snapshot struct {
	SelectedContexts map[string][]string
	AllNamespaces    map[string]bool                       // context -> showing every namespace unfiltered
	LoadingStates    map[string]bool                       // Combined across resource kinds
	LoadedContexts   map[string]bool                       // Contexts with at least one successful load
	LoadedKinds      map[msgs.ResourceKind]map[string]bool // kind -> context -> loaded, for per-tab unlock
	Errors           map[string]string
	ContextColors    map[string]color.Color
}

func NewAppState() *AppState {
	loading := make(map[msgs.ResourceKind]map[string]bool)
	kindLoaded := make(map[msgs.ResourceKind]map[string]bool)
	for _, kind := range msgs.Kinds() {
		loading[kind] = make(map[string]bool)
		kindLoaded[kind] = make(map[string]bool)
	}
	return &AppState{
		SelectedContexts: make(map[string][]string),
		allNamespaces:    make(map[string]bool),
		loading:          loading,
		Errors:           make(map[string]string),
		LoadedContexts:   make(map[string]bool),
		loadedKinds:      make(map[string]map[msgs.ResourceKind]bool),
		kindLoaded:       kindLoaded,
		contextColors:    make(map[string]color.Color),
	}
}

// requiredForLoaded is the set of kinds that must each deliver a first
// successful load before a context counts as "loaded" — Deployments (the
// tab a fresh selection lands on) and Pods (the tab every workflow touches
// next), so tab navigation and the Contexts pane's checkmark both wait for
// the two most-used tabs to be genuinely ready, not just the first one.
var requiredForLoaded = map[msgs.ResourceKind]bool{
	msgs.KindDeployments: true,
	msgs.KindPods:        true,
}

// AddContext adds or updates a context selection, seeding its checked
// namespace set with just the kubeconfig default, and assigning it an
// identity colour (in rotation order) the first time it's seen.
func (a *AppState) AddContext(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.SelectedContexts[context] = []string{namespace}
	if _, ok := a.contextColors[context]; !ok {
		a.contextColors[context] = styles.IdentityColor(a.nextColorIdx)
		a.nextColorIdx++
	}
}

// AddNamespace checks one additional namespace for an already-selected
// context — a no-op if it's already checked or the context isn't selected.
func (a *AppState) AddNamespace(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	namespaces, ok := a.SelectedContexts[context]
	if !ok {
		return
	}
	for _, ns := range namespaces {
		if ns == namespace {
			return
		}
	}
	a.SelectedContexts[context] = append(namespaces, namespace)
}

// RemoveNamespace unchecks one namespace for a context. It never removes the
// last remaining namespace — a context with zero checked namespaces would
// watch nothing while still showing as selected, which the Namespaces pane
// should refuse to produce in the first place; this is the defensive
// backstop.
func (a *AppState) RemoveNamespace(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	namespaces, ok := a.SelectedContexts[context]
	if !ok || len(namespaces) <= 1 {
		return
	}
	kept := make([]string, 0, len(namespaces)-1)
	for _, ns := range namespaces {
		if ns != namespace {
			kept = append(kept, ns)
		}
	}
	a.SelectedContexts[context] = kept
}

// SetAllNamespaces sets whether a context's row filter shows every
// namespace unfiltered — see NamespacesInfo's "a" (toggle-all) key.
func (a *AppState) SetAllNamespaces(context string, all bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if all {
		a.allNamespaces[context] = true
	} else {
		delete(a.allNamespaces, context)
	}
}

// SetLoading marks one resource kind as loading (or not) for a context.
func (a *AppState) SetLoading(kind msgs.ResourceKind, context string, loading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.loading[kind][context] = loading
}

// MarkLoaded records a successful row delivery for one resource kind in a
// context: that kind stops counting as loading, and once every kind in
// requiredForLoaded has each delivered at least one successful load, the
// context is marked loaded and its error cleared.
func (a *AppState) MarkLoaded(kind msgs.ResourceKind, context string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.loading[kind][context] = false
	a.kindLoaded[kind][context] = true
	if !requiredForLoaded[kind] {
		return
	}

	if a.loadedKinds[context] == nil {
		a.loadedKinds[context] = make(map[msgs.ResourceKind]bool)
	}
	a.loadedKinds[context][kind] = true
	if len(a.loadedKinds[context]) == len(requiredForLoaded) {
		a.LoadedContexts[context] = true
		delete(a.Errors, context)
	}
}

// SetError stores an error for a context and stops all its loading states.
func (a *AppState) SetError(context string, err string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Errors[context] = err
	for _, byContext := range a.loading {
		byContext[context] = false
	}
}

// RemoveContext removes a context and cleans up its data
func (a *AppState) RemoveContext(context string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.SelectedContexts, context)
	delete(a.allNamespaces, context)
	for _, byContext := range a.loading {
		delete(byContext, context)
	}
	for _, byContext := range a.kindLoaded {
		delete(byContext, context)
	}
	delete(a.Errors, context)
	delete(a.LoadedContexts, context)
	delete(a.loadedKinds, context)
}

// ClearErrors removes all error messages
func (a *AppState) ClearErrors() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Errors = make(map[string]string)
}

// Snapshot returns a read-only copy of the current state.
func (a *AppState) Snapshot() Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	loadedKinds := make(map[msgs.ResourceKind]map[string]bool, len(a.kindLoaded))
	for kind, byContext := range a.kindLoaded {
		loadedKinds[kind] = copyBoolMap(byContext)
	}

	return Snapshot{
		SelectedContexts: copyStringSliceMap(a.SelectedContexts),
		AllNamespaces:    copyBoolMap(a.allNamespaces),
		LoadingStates:    a.combinedLoadingStates(),
		LoadedContexts:   copyBoolMap(a.LoadedContexts),
		LoadedKinds:      loadedKinds,
		Errors:           copyStringMap(a.Errors),
		ContextColors:    copyColorMap(a.contextColors),
	}
}

// combinedLoadingStates returns a map showing if any resource kind is loading
// for each context. Must be called with lock held.
func (a *AppState) combinedLoadingStates() map[string]bool {
	combined := make(map[string]bool)
	for ctx := range a.SelectedContexts {
		anyLoading := false
		for _, byContext := range a.loading {
			if byContext[ctx] {
				anyLoading = true
				break
			}
		}
		combined[ctx] = anyLoading
	}
	return combined
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyStringSliceMap(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return map[string][]string{}
	}
	dst := make(map[string][]string, len(src))
	for k, v := range src {
		cp := make([]string, len(v))
		copy(cp, v)
		dst[k] = cp
	}
	return dst
}

func copyBoolMap(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return map[string]bool{}
	}
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyColorMap(src map[string]color.Color) map[string]color.Color {
	if len(src) == 0 {
		return map[string]color.Color{}
	}
	dst := make(map[string]color.Color, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
