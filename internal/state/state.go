// Package state
package state

import (
	"sync"

	"github.com/charmbracelet/bubbles/table"
)

type AppState struct {
	// Selected contexts and their namespaces
	SelectedContexts map[string]string // context -> namespace

	// Deployment data per context
	Deployments map[string][]table.Row // context -> rows
	
	// Pod data per context
	Pods map[string][]table.Row // context -> rows

	// Loading states
	LoadingDeployments map[string]bool // context -> isLoading
	LoadingPods        map[string]bool // context -> isLoading

	// Errors
	Errors map[string]string // context -> error message

	// Cache for GetAllDeployments and GetAllPods
	cachedAllDeployments []table.Row
	cachedAllPods        []table.Row
	deploymentsDirty     bool
	podsDirty            bool

	// Mutex to protect concurrent access
	mu sync.RWMutex
}

// Snapshot captures a read-only view of application state data.
// This pattern allows us to do a single batched read lock instead of
// multiple individual reads, reducing lock contention and improving performance.
type Snapshot struct {
	SelectedContexts map[string]string
	LoadingStates    map[string]bool // Combined deployment + pod loading
	Errors           map[string]string
	Deployments      []table.Row
	Pods             []table.Row
}

func NewAppState() *AppState {
	return &AppState{
		SelectedContexts:   make(map[string]string),
		Deployments:        make(map[string][]table.Row),
		Pods:               make(map[string][]table.Row),
		LoadingDeployments: make(map[string]bool),
		LoadingPods:        make(map[string]bool),
		Errors:             make(map[string]string),
		deploymentsDirty:   true,
		podsDirty:          true,
	}
}

// AddContext adds or updates a context selection
func (a *AppState) AddContext(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.SelectedContexts[context] = namespace
	// Initialize deployment and pod state for this context
	if _, exists := a.Deployments[context]; !exists {
		a.Deployments[context] = []table.Row{}
	}
	if _, exists := a.Pods[context]; !exists {
		a.Pods[context] = []table.Row{}
	}
	a.deploymentsDirty = true
	a.podsDirty = true
	a.cachedAllDeployments = nil
	a.cachedAllPods = nil
}

// SetDeployments replaces deployment rows for a context
func (a *AppState) SetDeployments(context string, rows []table.Row) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Deployments[context] = cloneRows(rows)
	a.LoadingDeployments[context] = false
	delete(a.Errors, context)
	a.deploymentsDirty = true
	a.cachedAllDeployments = nil
}

// SetPods replaces pod rows for a context
func (a *AppState) SetPods(context string, rows []table.Row) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Pods[context] = cloneRows(rows)
	a.LoadingPods[context] = false
	// Don't delete errors here - only clear if both deployments AND pods succeed
	a.podsDirty = true
	a.cachedAllPods = nil
}

// SetLoading marks a context as loading (for deployments)
func (a *AppState) SetLoading(context string, loading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LoadingDeployments[context] = loading
}

// SetLoadingPods marks a context as loading pods
func (a *AppState) SetLoadingPods(context string, loading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LoadingPods[context] = loading
}

// SetError stores an error for a context
func (a *AppState) SetError(context string, err string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Errors[context] = err
	a.LoadingDeployments[context] = false
	a.LoadingPods[context] = false
}

// GetAllDeployments returns all deployment rows from all selected contexts
// Uses caching to avoid recomputing on every call
func (a *AppState) GetAllDeployments() []table.Row {
	snapshot := a.Snapshot()
	return snapshot.Deployments
}

// GetAllPods returns all pod rows from all selected contexts
// Uses caching to avoid recomputing on every call
func (a *AppState) GetAllPods() []table.Row {
	snapshot := a.Snapshot()
	return snapshot.Pods
}

// Snapshot returns a read-only view of the current state using a batched read lock.
func (a *AppState) Snapshot() Snapshot {
	a.mu.RLock()
	
	// Check if we can use cached data
	deploymentsClean := !a.deploymentsDirty && a.cachedAllDeployments != nil
	podsClean := !a.podsDirty && a.cachedAllPods != nil
	
	if deploymentsClean && podsClean {
		snapshot := Snapshot{
			SelectedContexts: copyStringMap(a.SelectedContexts),
			LoadingStates:    a.combinedLoadingStates(),
			Errors:           copyStringMap(a.Errors),
			Deployments:      cloneRows(a.cachedAllDeployments),
			Pods:             cloneRows(a.cachedAllPods),
		}
		a.mu.RUnlock()
		return snapshot
	}
	a.mu.RUnlock()

	// Need to recompute
	a.mu.Lock()
	defer a.mu.Unlock()

	// Recompute deployments if dirty
	if a.deploymentsDirty || a.cachedAllDeployments == nil {
		a.cachedAllDeployments = flattenRows(a.SelectedContexts, a.Deployments)
		a.deploymentsDirty = false
	}

	// Recompute pods if dirty
	if a.podsDirty || a.cachedAllPods == nil {
		a.cachedAllPods = flattenRows(a.SelectedContexts, a.Pods)
		a.podsDirty = false
	}

	return Snapshot{
		SelectedContexts: copyStringMap(a.SelectedContexts),
		LoadingStates:    a.combinedLoadingStates(),
		Errors:           copyStringMap(a.Errors),
		Deployments:      cloneRows(a.cachedAllDeployments),
		Pods:             cloneRows(a.cachedAllPods),
	}
}

// combinedLoadingStates returns a map showing if any resource type is loading for each context
// Must be called with lock held
func (a *AppState) combinedLoadingStates() map[string]bool {
	combined := make(map[string]bool)
	
	for ctx := range a.SelectedContexts {
		// Context is loading if EITHER deployments OR pods are loading
		combined[ctx] = a.LoadingDeployments[ctx] || a.LoadingPods[ctx]
	}
	
	return combined
}

// IsAnyLoading checks if any context is loading any resource
func (a *AppState) IsAnyLoading() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, loading := range a.LoadingDeployments {
		if loading {
			return true
		}
	}
	
	for _, loading := range a.LoadingPods {
		if loading {
			return true
		}
	}
	
	return false
}

// GetNamespace returns namespace for a context
func (a *AppState) GetNamespace(context string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.SelectedContexts[context]
}

// RemoveContext removes a context and cleans up its data
func (a *AppState) RemoveContext(context string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.SelectedContexts, context)
	delete(a.Deployments, context)
	delete(a.Pods, context)
	delete(a.LoadingDeployments, context)
	delete(a.LoadingPods, context)
	delete(a.Errors, context)
	a.deploymentsDirty = true
	a.podsDirty = true
	a.cachedAllDeployments = nil
	a.cachedAllPods = nil
}

// ClearErrors removes all error messages
func (a *AppState) ClearErrors() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Errors = make(map[string]string)
}

// GetErrors returns a copy of all errors (safe for display)
func (a *AppState) GetErrors() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	errors := make(map[string]string, len(a.Errors))
	for k, v := range a.Errors {
		errors[k] = v
	}
	return errors
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

// flattenRows combines rows from multiple contexts (renamed from flattenDeployments for reuse)
func flattenRows(selected map[string]string, rowsByContext map[string][]table.Row) []table.Row {
	if len(selected) == 0 {
		return nil
	}

	var all []table.Row
	for context := range selected {
		rows, exists := rowsByContext[context]
		if !exists {
			continue
		}

		for _, row := range rows {
			cloned := make(table.Row, len(row))
			copy(cloned, row)
			all = append(all, cloned)
		}
	}

	return all
}

func cloneRows(rows []table.Row) []table.Row {
	if len(rows) == 0 {
		return nil
	}

	cloned := make([]table.Row, len(rows))
	for i, row := range rows {
		if len(row) == 0 {
			cloned[i] = nil
			continue
		}

		cells := make(table.Row, len(row))
		copy(cells, row)
		cloned[i] = cells
	}

	return cloned
}