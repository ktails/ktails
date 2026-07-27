package models

import (
	"fmt"
	"io"
	"sort"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/tui/styles"
)

// clusterGroup holds one kubeconfig cluster's display state: every context
// name backed by it, and whether every one of those contexts is currently
// checked in the Contexts pane.
type clusterGroup struct {
	Name         string
	ContextNames []string
	AllSelected  bool
}

func (g clusterGroup) Title() string       { return g.Name }
func (g clusterGroup) Description() string { return fmt.Sprintf("%d context(s)", len(g.ContextNames)) }
func (g clusterGroup) FilterValue() string { return g.Name }

// clusterDelegate renders each cluster group the same two-line shape as
// contextDelegate (dot + name, indented description), just with one dot
// state (all-selected / not) instead of the Contexts row's richer
// identity/status iconography — a cluster group has no loading/error state
// of its own, only aggregate selection.
type clusterDelegate struct{}

func (d clusterDelegate) Height() int                         { return 2 }
func (d clusterDelegate) Spacing() int                        { return 0 }
func (d clusterDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d clusterDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	g, ok := item.(clusterGroup)
	if !ok {
		return
	}

	p := styles.CatppuccinMocha()
	isCursor := index == m.Index()

	paneWidth := m.Width()
	if paneWidth <= 0 {
		paneWidth = 30
	}

	dot := "○"
	dotColor := p.Overlay1
	if g.AllSelected {
		dot = "●"
		dotColor = p.Green
	}

	prefixW := 1 + lipgloss.Width(dot) + 1
	name := ansi.TruncateWc(g.Name, max(paneWidth-prefixW, 0), "…")

	descFixed := 4
	descText := ansi.TruncateWc(g.Description(), max(paneWidth-descFixed, 0), "…")

	dotStr := lipgloss.NewStyle().Foreground(dotColor).Render(dot)
	nameStr := lipgloss.NewStyle().Foreground(p.Text).Bold(g.AllSelected).Render(name)
	descStr := lipgloss.NewStyle().Foreground(p.Overlay1).Render(descText)

	titleContent := " " + dotStr + " " + nameStr
	descContent := "    " + descStr

	if isCursor {
		titleLine := lipgloss.NewStyle().Background(styles.FocusColor).Foreground(p.Base).Width(paneWidth).Render(titleContent)
		descLine := lipgloss.NewStyle().Background(styles.FocusColor).Foreground(p.Base).Width(paneWidth).Render(descContent)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	} else {
		titleLine := lipgloss.NewStyle().Width(paneWidth).Render(titleContent)
		descLine := lipgloss.NewStyle().Foreground(p.Overlay0).Width(paneWidth).Render(descContent)
		fmt.Fprintf(w, "%s\n%s", titleLine, descLine)
	}
}

// ClustersInfo is the left-pane model that groups the Contexts pane's
// contexts by their kubeconfig cluster, for bulk select/deselect. It reads
// group membership straight from ContextsInfo (no separate data fetch) and
// drives selection through ContextsInfo.SetSelected/ConfirmChanges, so the
// diff/confirm pipeline the Contexts pane already owns is the only place
// that logic lives.
type ClustersInfo struct {
	contexts *ContextsInfo
	list     list.Model
	width    int
	height   int
	Focused  bool
}

func NewClustersInfo(contexts *ContextsInfo) *ClustersInfo {
	newList := list.New([]list.Item{}, clusterDelegate{}, 0, 0)
	newList.SetShowStatusBar(false)
	newList.SetShowHelp(false)
	newList.SetShowPagination(false)
	newList.Title = ""
	return &ClustersInfo{contexts: contexts, list: newList}
}

// Refresh rebuilds the cluster groups from the Contexts pane's current
// context list. Cheap enough (a handful of contexts, typically) to call on
// every render rather than threading invalidation through every place the
// Contexts pane's selection can change.
func (cl *ClustersInfo) Refresh() {
	byCluster := make(map[string][]string)
	selected := make(map[string]bool)
	for _, ctx := range cl.contexts.contextsSnapshot() {
		name := ctx.Cluster
		if name == "" {
			name = "—"
		}
		byCluster[name] = append(byCluster[name], ctx.Name)
		selected[ctx.Name] = ctx.Selected
	}

	names := make([]string, 0, len(byCluster))
	for name := range byCluster {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		ctxNames := byCluster[name]
		sort.Strings(ctxNames)
		allSelected := len(ctxNames) > 0
		for _, c := range ctxNames {
			if !selected[c] {
				allSelected = false
				break
			}
		}
		items = append(items, clusterGroup{Name: name, ContextNames: ctxNames, AllSelected: allSelected})
	}

	// Preserve the cursor position across a rebuild so repeated Refresh
	// calls (every render) don't bounce the cursor back to the top.
	idx := cl.list.Index()
	cl.list.SetItems(items)
	if idx >= 0 && idx < len(items) {
		cl.list.Select(idx)
	}
}

func (cl *ClustersInfo) setDimensions() {
	cl.list.SetWidth(cl.width)
	cl.list.SetHeight(cl.height)
}

func (cl *ClustersInfo) SetSize(w, h int) {
	cl.width = w
	cl.height = h
	cl.setDimensions()
}

func (cl *ClustersInfo) SetFocused(f bool) {
	cl.Focused = f
}

func (cl *ClustersInfo) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	var cmd tea.Cmd
	switch key.String() {
	case "up", "k", "down", "j":
		cl.list, cmd = cl.list.Update(msg)
		return cmd
	case "space":
		cl.toggleGroup()
		return nil
	case "enter":
		return cl.contexts.ConfirmChanges()
	}
	return nil
}

// toggleGroup flips every context under the cluster row at the cursor —
// unselecting the whole group if every context in it is currently selected,
// otherwise selecting every context in it (so one Space always moves the
// group toward a uniform state instead of toggling each context
// individually, which could leave it mixed).
func (cl *ClustersInfo) toggleGroup() {
	idx := cl.list.Index()
	items := cl.list.Items()
	if idx < 0 || idx >= len(items) {
		return
	}
	g, ok := items[idx].(clusterGroup)
	if !ok {
		return
	}
	cl.contexts.SetSelected(g.ContextNames, !g.AllSelected)
	cl.Refresh()
	cl.list.Select(idx)
}

func (cl *ClustersInfo) View() string {
	return cl.list.View()
}
