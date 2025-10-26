package pages

import "github.com/charmbracelet/bubbles/table"

type AppState struct {
	// Selected contexts and their namespaces
	SelectedContexts map[string]string // context -> namespace

	// Deployment data per context
	Deployments map[string][]table.Row // context -> rows

	// Loading states
	LoadingDeployments map[string]bool // context -> isLoading

	// Errors
	Errors map[string]string // context -> error message
}

func NewAppState() *AppState {
	return &AppState{
		SelectedContexts:   make(map[string]string),
		Deployments:        make(map[string][]table.Row),
		LoadingDeployments: make(map[string]bool),
		Errors:             make(map[string]string),
	}
}

// AddContext adds or updates a context selection
func (a *AppState) AddContext(context, namespace string) {
	a.SelectedContexts[context] = namespace
	// Initialize deployment state for this context
	if _, exists := a.Deployments[context]; !exists {
		a.Deployments[context] = []table.Row{}
	}
}

// SetDeployments replaces deployment rows for a context
func (a *AppState) SetDeployments(context string, rows []table.Row) {
	a.Deployments[context] = rows
	a.LoadingDeployments[context] = false
	delete(a.Errors, context)
}

// SetLoading marks a context as loading
func (a *AppState) SetLoading(context string, loading bool) {
	a.LoadingDeployments[context] = loading
}

// SetError stores an error for a context
func (a *AppState) SetError(context string, err string) {
	a.Errors[context] = err
	a.LoadingDeployments[context] = false
}

// GetAllDeployments returns all deployment rows from all selected contexts
func (a *AppState) GetAllDeployments() []table.Row {
	var allRows []table.Row
	for context := range a.SelectedContexts {
		if rows, exists := a.Deployments[context]; exists {
			allRows = append(allRows, rows...)
		}
	}
	return allRows
}

// IsAnyLoading checks if any context is loading
func (a *AppState) IsAnyLoading() bool {
	for _, loading := range a.LoadingDeployments {
		if loading {
			return true
		}
	}
	return false
}

// GetNamespace returns namespace for a context
func (a *AppState) GetNamespace(context string) string {
	return a.SelectedContexts[context]
}
