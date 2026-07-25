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
	// Selected contexts and their namespaces
	SelectedContexts map[string]string // context -> namespace

	// Loading states per resource kind
	loading map[msgs.ResourceKind]map[string]bool // kind -> context -> isLoading

	// Errors
	Errors map[string]string // context -> error message

	// Contexts that have completed at least one successful load cycle
	LoadedContexts map[string]bool

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
	SelectedContexts map[string]string
	LoadingStates    map[string]bool // Combined across resource kinds
	LoadedContexts   map[string]bool // Contexts with at least one successful load
	Errors           map[string]string
	ContextColors    map[string]color.Color
}

func NewAppState() *AppState {
	loading := make(map[msgs.ResourceKind]map[string]bool)
	for _, kind := range msgs.Kinds() {
		loading[kind] = make(map[string]bool)
	}
	return &AppState{
		SelectedContexts: make(map[string]string),
		loading:          loading,
		Errors:           make(map[string]string),
		LoadedContexts:   make(map[string]bool),
		contextColors:    make(map[string]color.Color),
	}
}

// AddContext adds or updates a context selection, assigning it an identity
// colour (in rotation order) the first time it's seen.
func (a *AppState) AddContext(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.SelectedContexts[context] = namespace
	if _, ok := a.contextColors[context]; !ok {
		a.contextColors[context] = styles.IdentityColor(a.nextColorIdx)
		a.nextColorIdx++
	}
}

// SetLoading marks one resource kind as loading (or not) for a context.
func (a *AppState) SetLoading(kind msgs.ResourceKind, context string, loading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.loading[kind][context] = loading
}

// MarkLoaded records a successful row delivery for one resource kind in a
// context: that kind stops counting as loading, and — for Deployments only,
// preserving the pre-Supervisor behavior — the context is marked loaded and
// its error cleared. (Deployments is the tab a fresh selection lands on, so
// it's the kind whose first delivery means "this context works".)
func (a *AppState) MarkLoaded(kind msgs.ResourceKind, context string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.loading[kind][context] = false
	if kind == msgs.KindDeployments {
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
	for _, byContext := range a.loading {
		delete(byContext, context)
	}
	delete(a.Errors, context)
	delete(a.LoadedContexts, context)
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

	return Snapshot{
		SelectedContexts: copyStringMap(a.SelectedContexts),
		LoadingStates:    a.combinedLoadingStates(),
		LoadedContexts:   copyBoolMap(a.LoadedContexts),
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
