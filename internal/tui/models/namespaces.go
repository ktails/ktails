package models

import (
	"fmt"
	"io"
	"sort"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

// namespaceRow is one row in the Namespaces pane's flattened list: either a
// non-selectable context header (IsHeader, carrying that context's load
// state) or a selectable namespace checkbox beneath it.
type namespaceRow struct {
	Context  string
	Name     string
	Selected bool
	IsHeader bool
	Loading  bool
	Err      string
}

func (r namespaceRow) Title() string {
	if r.IsHeader {
		return r.Context
	}
	return r.Name
}
func (r namespaceRow) Description() string { return "" }
func (r namespaceRow) FilterValue() string { return r.Context + "/" + r.Name }

// namespaceDelegate renders header rows as a bold, non-interactive context
// name (with a loading/error suffix), and namespace rows as an indented
// checkbox — one line each, unlike Contexts/Clusters' two-line rows, since
// there's no per-row description to show.
type namespaceDelegate struct{}

func (d namespaceDelegate) Height() int                         { return 1 }
func (d namespaceDelegate) Spacing() int                        { return 0 }
func (d namespaceDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d namespaceDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(namespaceRow)
	if !ok {
		return
	}

	p := styles.CatppuccinMocha()
	paneWidth := m.Width()
	if paneWidth <= 0 {
		paneWidth = 30
	}

	if row.IsHeader {
		text := row.Context
		switch {
		case row.Err != "":
			text += "  " + lipgloss.NewStyle().Foreground(styles.StatusError).Render("⚠ "+row.Err)
		case row.Loading:
			text += "  " + lipgloss.NewStyle().Foreground(p.Overlay1).Render("loading…")
		}
		line := ansi.TruncateWc(text, paneWidth, "…")
		fmt.Fprint(w, lipgloss.NewStyle().Foreground(p.Lavender).Bold(true).Width(paneWidth).Render(line))
		return
	}

	isCursor := index == m.Index()
	dot := "○"
	dotColor := p.Overlay1
	if row.Selected {
		dot = "●"
		dotColor = p.Green
	}
	name := ansi.TruncateWc(row.Name, max(paneWidth-6, 0), "…")
	dotStr := lipgloss.NewStyle().Foreground(dotColor).Render(dot)
	content := "    " + dotStr + " " + name

	if isCursor {
		fmt.Fprint(w, lipgloss.NewStyle().Background(styles.FocusColor).Foreground(p.Base).Width(paneWidth).Render(content))
	} else {
		fmt.Fprint(w, lipgloss.NewStyle().Width(paneWidth).Render(content))
	}
}

// NamespacesInfo is the left-pane model that lets a user multi-select which
// namespaces each selected context watches. Contexts always start with
// exactly the kubeconfig default namespace checked (see
// state.AppState.AddContext) — this pane is what lets that set grow beyond
// one. Space toggles a namespace under the cursor; Enter confirms, diffing
// against the last confirm per context (mirroring ContextsInfo's own
// Space/Enter two-phase flow) and reporting the result as one
// msgs.NamespacesStateMsg per changed context.
type NamespacesInfo struct {
	list    list.Model
	width   int
	height  int
	Focused bool

	// namespacesByContext holds each context's full namespace list, once
	// loaded; errByContext holds a load failure instead (e.g. an RBAC
	// denial) — mutually exclusive per context, and a context with neither
	// yet is still loading.
	namespacesByContext map[string][]string
	errByContext        map[string]string
	// previouslyConfirmed is what's actually being watched as of the last
	// confirm (or the last SyncConfirmed reconciliation) — the diff base,
	// same role as ContextsInfo.previouslySelected.
	previouslyConfirmed map[string]map[string]bool
}

func NewNamespacesInfo() *NamespacesInfo {
	newList := list.New([]list.Item{}, namespaceDelegate{}, 0, 0)
	newList.SetShowStatusBar(false)
	newList.SetShowHelp(false)
	newList.SetShowPagination(false)
	// See contexts.go's NewContextInfo for why this must stay off.
	newList.SetFilteringEnabled(false)
	newList.Title = ""
	return &NamespacesInfo{
		list:                newList,
		namespacesByContext: make(map[string][]string),
		errByContext:        make(map[string]string),
		previouslyConfirmed: make(map[string]map[string]bool),
	}
}

// SetContextNamespaces records a successful namespace-list fetch for one
// context and rebuilds the pane. Callers should follow this with
// SyncConfirmed to seed which namespace(s) already show checked.
func (n *NamespacesInfo) SetContextNamespaces(context string, namespaces []string) {
	delete(n.errByContext, context)
	n.namespacesByContext[context] = namespaces
	n.rebuild()
}

// SetContextError records a failed namespace-list fetch (e.g. RBAC denial)
// for one context, shown inline on its header row instead of a namespace
// list.
func (n *NamespacesInfo) SetContextError(context string, err string) {
	delete(n.namespacesByContext, context)
	n.errByContext[context] = err
	n.rebuild()
}

// RemoveContext drops all pane state for a deselected context.
func (n *NamespacesInfo) RemoveContext(context string) {
	delete(n.namespacesByContext, context)
	delete(n.errByContext, context)
	delete(n.previouslyConfirmed, context)
	n.rebuild()
}

// SyncConfirmed resets one context's confirmed (and visibly checked) set to
// exactly match `selected` — called right after a fetch completes (to seed
// the initial checked namespace), and after MainPage reconciles a confirm
// with AppState, since AppState may have refused part of it (e.g. it never
// lets the last checked namespace be removed) — without this, the pane
// could keep showing an unchecked box for a namespace still actually being
// watched.
func (n *NamespacesInfo) SyncConfirmed(context string, selected []string) {
	set := make(map[string]bool, len(selected))
	for _, ns := range selected {
		set[ns] = true
	}
	n.previouslyConfirmed[context] = set

	items := n.list.Items()
	updated := false
	for idx, item := range items {
		row, ok := item.(namespaceRow)
		if !ok || row.IsHeader || row.Context != context {
			continue
		}
		want := set[row.Name]
		if row.Selected != want {
			row.Selected = want
			items[idx] = row
			updated = true
		}
	}
	if updated {
		n.list.SetItems(items)
	}
}

// rebuild reconstructs the flattened item list from namespacesByContext/
// errByContext, sorted by context name. Any row already checked but not yet
// confirmed (a pending Space toggle) is preserved across the rebuild by
// reading the live list first — otherwise an unrelated context's async load
// landing mid-session would silently discard this context's in-progress
// checkbox changes.
func (n *NamespacesInfo) rebuild() {
	pending := make(map[string]map[string]bool)
	for _, item := range n.list.Items() {
		row, ok := item.(namespaceRow)
		if !ok || row.IsHeader {
			continue
		}
		if pending[row.Context] == nil {
			pending[row.Context] = make(map[string]bool)
		}
		pending[row.Context][row.Name] = row.Selected
	}

	seen := make(map[string]bool)
	var contexts []string
	for c := range n.namespacesByContext {
		if !seen[c] {
			seen[c] = true
			contexts = append(contexts, c)
		}
	}
	for c := range n.errByContext {
		if !seen[c] {
			seen[c] = true
			contexts = append(contexts, c)
		}
	}
	sort.Strings(contexts)

	var items []list.Item
	for _, c := range contexts {
		if errMsg, ok := n.errByContext[c]; ok {
			items = append(items, namespaceRow{Context: c, IsHeader: true, Err: errMsg})
			continue
		}
		namespaces, ok := n.namespacesByContext[c]
		if !ok {
			items = append(items, namespaceRow{Context: c, IsHeader: true, Loading: true})
			continue
		}
		items = append(items, namespaceRow{Context: c, IsHeader: true})
		confirmed := n.previouslyConfirmed[c]
		for _, ns := range namespaces {
			selected := confirmed[ns]
			if p, ok := pending[c][ns]; ok {
				selected = p
			}
			items = append(items, namespaceRow{Context: c, Name: ns, Selected: selected})
		}
	}

	idx := n.list.Index()
	n.list.SetItems(items)
	if idx >= 0 && idx < len(items) {
		n.list.Select(idx)
	}
}

func (n *NamespacesInfo) setDimensions() {
	n.list.SetWidth(n.width)
	n.list.SetHeight(n.height)
}

func (n *NamespacesInfo) SetSize(w, h int) {
	n.width = w
	n.height = h
	n.setDimensions()
}

func (n *NamespacesInfo) SetFocused(f bool) {
	n.Focused = f
}

func (n *NamespacesInfo) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	var cmd tea.Cmd
	switch key.String() {
	case "up", "k", "down", "j":
		n.list, cmd = n.list.Update(msg)
		return cmd
	case "space":
		n.toggleRow()
		return nil
	case "enter":
		return n.confirmChanges()
	}
	return nil
}

func (n *NamespacesInfo) toggleRow() {
	idx := n.list.Index()
	items := n.list.Items()
	if idx < 0 || idx >= len(items) {
		return
	}
	row, ok := items[idx].(namespaceRow)
	if !ok || row.IsHeader {
		return
	}
	row.Selected = !row.Selected
	items[idx] = row
	n.list.SetItems(items)
	n.list.Select(idx)
}

// confirmChanges diffs every context's currently-checked namespaces against
// previouslyConfirmed and returns one command per context with a change,
// each carrying a msgs.NamespacesStateMsg — nil if nothing changed anywhere.
func (n *NamespacesInfo) confirmChanges() tea.Cmd {
	current := make(map[string]map[string]bool)
	for _, item := range n.list.Items() {
		row, ok := item.(namespaceRow)
		if !ok || row.IsHeader || !row.Selected {
			continue
		}
		if current[row.Context] == nil {
			current[row.Context] = make(map[string]bool)
		}
		current[row.Context][row.Name] = true
	}

	contexts := make(map[string]bool)
	for c := range current {
		contexts[c] = true
	}
	for c := range n.previouslyConfirmed {
		contexts[c] = true
	}

	var cmds []tea.Cmd
	for context := range contexts {
		var added, removed []string
		for ns := range current[context] {
			if !n.previouslyConfirmed[context][ns] {
				added = append(added, ns)
			}
		}
		for ns := range n.previouslyConfirmed[context] {
			if !current[context][ns] {
				removed = append(removed, ns)
			}
		}
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		sort.Strings(added)
		sort.Strings(removed)
		state := msgs.NamespacesStateMsg{Context: context, Added: added, Removed: removed}
		cmds = append(cmds, func() tea.Msg { return state })
	}

	n.previouslyConfirmed = current
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (n *NamespacesInfo) View() string {
	return n.list.View()
}
