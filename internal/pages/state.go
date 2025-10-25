package pages

type AppState struct {
	SelectedContextsNamespace map[string]string
}

func (a *AppState) GetNamespace(kubeCtxName string) string {
	return a.SelectedContextsNamespace[kubeCtxName]

}
func NewAppState() *AppState {
	return &AppState{
		SelectedContextsNamespace: map[string]string{},
	}
}
