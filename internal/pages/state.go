package pages

type AppState struct {
	SelectedContextsNamespace map[string]string
}

func (a *AppState) GetNamespace(kubeCtxName string) string {
	return a.SelectedContextsNamespace[kubeCtxName]
}

func NewAppState() *AppState {
	m := map[string]string{}
	return &AppState{
		SelectedContextsNamespace: m,
	}
}
