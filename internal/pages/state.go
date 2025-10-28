package pages

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
}

// SetDeployments replaces deployment rows for a context
func (a *AppState) SetDeployments(context string, rows []table.Row) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Deployments[context] = rows
	a.LoadingDeployments[context] = false
	delete(a.Errors, context)
	a.deploymentsDirty = true
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
	a.mu.RLock()
	if !a.deploymentsDirty && a.cachedAllDeployments != nil {
		defer a.mu.RUnlock()
		return a.cachedAllDeployments
	}
	a.mu.RUnlock()

	// Need to recompute
	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-check after acquiring write lock
	if !a.deploymentsDirty && a.cachedAllDeployments != nil {
		return a.cachedAllDeployments
	}

	var allRows []table.Row
	for context := range a.SelectedContexts {
		if rows, exists := a.Deployments[context]; exists {
			allRows = append(allRows, rows...)
		}
	}

	a.cachedAllDeployments = allRows
	a.deploymentsDirty = false
	return allRows
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