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

	// Loading states
	LoadingDeployments map[string]bool // context -> isLoading

	// Errors
	Errors map[string]string // context -> error message

	// Cache for GetAllDeployments
	cachedAllDeployments []table.Row
	deploymentsDirty     bool

	// Mutex to protect concurrent access
	mu sync.RWMutex
}

// Snapshot captures a read-only view of application state data.
type Snapshot struct {
	SelectedContexts map[string]string
	LoadingStates    map[string]bool
	Errors           map[string]string
	Deployments      []table.Row
}

func NewAppState() *AppState {
	return &AppState{
		SelectedContexts:   make(map[string]string),
		Deployments:        make(map[string][]table.Row),
		LoadingDeployments: make(map[string]bool),
		Errors:             make(map[string]string),
		deploymentsDirty:   true,
	}
}

// AddContext adds or updates a context selection
func (a *AppState) AddContext(context, namespace string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.SelectedContexts[context] = namespace
	// Initialize deployment state for this context
	if _, exists := a.Deployments[context]; !exists {
		a.Deployments[context] = []table.Row{}
	}
	a.deploymentsDirty = true
	a.cachedAllDeployments = nil
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

// SetLoading marks a context as loading
func (a *AppState) SetLoading(context string, loading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LoadingDeployments[context] = loading
}

// SetError stores an error for a context
func (a *AppState) SetError(context string, err string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Errors[context] = err
	a.LoadingDeployments[context] = false
}

// GetAllDeployments returns all deployment rows from all selected contexts
// Uses caching to avoid recomputing on every call
func (a *AppState) GetAllDeployments() []table.Row {
	snapshot := a.Snapshot()
	return snapshot.Deployments
}

// Snapshot returns a read-only view of the current state using a batched read lock.
func (a *AppState) Snapshot() Snapshot {
	a.mu.RLock()
	if !a.deploymentsDirty && a.cachedAllDeployments != nil {
		snapshot := Snapshot{
			SelectedContexts: copyStringMap(a.SelectedContexts),
			LoadingStates:    copyBoolMap(a.LoadingDeployments),
			Errors:           copyStringMap(a.Errors),
			Deployments:      cloneRows(a.cachedAllDeployments),
		}
		a.mu.RUnlock()
		return snapshot
	}
	a.mu.RUnlock()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.deploymentsDirty || a.cachedAllDeployments == nil {
		a.cachedAllDeployments = flattenDeployments(a.SelectedContexts, a.Deployments)
		a.deploymentsDirty = false
	}

	return Snapshot{
		SelectedContexts: copyStringMap(a.SelectedContexts),
		LoadingStates:    copyBoolMap(a.LoadingDeployments),
		Errors:           copyStringMap(a.Errors),
		Deployments:      cloneRows(a.cachedAllDeployments),
	}
}

// IsAnyLoading checks if any context is loading
func (a *AppState) IsAnyLoading() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, loading := range a.LoadingDeployments {
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
	delete(a.LoadingDeployments, context)
	delete(a.Errors, context)
	a.deploymentsDirty = true
	a.cachedAllDeployments = nil
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

func flattenDeployments(selected map[string]string, deployments map[string][]table.Row) []table.Row {
	if len(selected) == 0 {
		return nil
	}

	var all []table.Row
	for context := range selected {
		rows, exists := deployments[context]
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
