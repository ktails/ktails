package pages

type AppState struct {
	SelectedContexts []string
	currentContext   string
	currentNamespace string
}

func (a *AppState) SetSelectedContexts(ctxNames []string) {
	a.SelectedContexts = ctxNames
}

func (a AppState) GetCurrentContext() string {
	return a.currentContext
}

func (a *AppState) GetCurrentNamespace() string {
	return a.currentNamespace
}

func NewAppState() *AppState {
	return &AppState{
		SelectedContexts: []string{},
		currentContext:   "",
		currentNamespace: "",
	}
}
