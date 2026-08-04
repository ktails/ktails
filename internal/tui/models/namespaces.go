package models

import (
	"fmt"
	"io"
	"sort"
	"strings"

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
	// AllNS marks a loaded header row whose context is in all-namespaces
	// mode (see NamespacesInfo.toggleAllNamespaces) — shown as a suffix on
	// the header text, since individual namespace rows show unchecked in
	// this mode rather than all appearing individually selected.
	AllNS bool
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
// there's no per-row description to show. focused mirrors
// NamespacesInfo.Focused (see contextDelegate's doc comment for why this
// matters).
type namespaceDelegate struct{ focused bool }

func (d namespaceDelegate) Height() int                         { return 1 }
func (d namespaceDelegate) Spacing() int                        { return 0 }
func (d namespaceDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d namespaceDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(namespaceRow)
	if !ok {
		return
	}

	p := styles.CatppuccinMocha()
	paneWidth := paneRowWidth(m)

	if row.IsHeader {
		text := row.Context
		switch {
		case row.Err != "":
			text += "  " + lipgloss.NewStyle().Foreground(styles.StatusError).Render("⚠ "+row.Err)
		case row.Loading:
			text += "  " + lipgloss.NewStyle().Foreground(p.Overlay1).Render("loading…")
		case row.AllNS:
			text += "  " + lipgloss.NewStyle().Foreground(p.Green).Render("(all namespaces)")
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

	// One uniform highlight color, matching the resource tables' selected-
	// row style (styles.CatppuccinBubbleTableStyle().Highlight:
	// Background(FocusColor), Foreground(Base), Bold) — the dot keeps its
	// own tint everywhere else, but blending it into the highlight here
	// keeps the focused row reading as a single solid block instead of a
	// background color fighting a separately-tinted glyph. No description
	// line — namespace rows are single-line (styledDesc left "").
	fmt.Fprint(w, renderPaneRow(paneWidth, paneRowContent{
		plainTitle:  "    " + dot + " " + name,
		styledTitle: "    " + dotStr + " " + name,
	}, isCursor, d.focused))
}

// NamespacesInfo is the left-pane model that lets a user multi-select which
// namespaces each selected context watches. Contexts always start with
// exactly the kubeconfig default namespace checked (see
// state.AppState.AddContext) — this pane is what lets that set grow beyond
// one. Space toggles a namespace under the cursor; "a" bulk-toggles every
// namespace under the context at the cursor between "all checked" and
// whatever was checked before (see toggleAllNamespaces); "/" filters
// namespace rows by name substring. Enter confirms, diffing against the
// last confirm per context (mirroring ContextsInfo's own Space/Enter
// two-phase flow) and reporting the result as one msgs.NamespacesStateMsg
// per changed context.
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
	// checked is the pending, not-yet-confirmed checked set per context —
	// the single source of truth for what's currently ticked, independent
	// of which rows the "/" filter currently has visible. Space/"a" mutate
	// this directly; confirmChanges diffs it against previouslyConfirmed;
	// rebuild reads it (never the other way around) when deciding each
	// visible row's Selected state.
	checked map[string]map[string]bool
	// previouslyConfirmed is what's actually being shown as of the last
	// confirm (or the last SyncConfirmed reconciliation) — the diff base,
	// same role as ContextsInfo.previouslySelected.
	previouslyConfirmed map[string]map[string]bool
	// allNamespaces is the pending, not-yet-confirmed all-namespaces flag per
	// context — true means "show every namespace unfiltered", set/cleared by
	// the "a" key (see toggleAllNamespaces) or by manually toggling a single
	// namespace while in that mode (see toggleRow). Mirrors checked's role:
	// the single source of truth rebuild reads from.
	allNamespaces map[string]bool
	// previouslyConfirmedAll is allNamespaces' diff base, same role as
	// previouslyConfirmed.
	previouslyConfirmedAll map[string]bool
	// preSelectAll holds, per context, the checked set as it was right
	// before entering all-namespaces mode via "a" — restored by pressing "a"
	// again to leave that mode, so it acts as a toggle rather than a
	// one-way action. Cleared once restored, or once a confirm/sync makes it
	// stale.
	preSelectAll map[string]map[string]bool

	// filter is a "/" substring filter over namespace rows (header rows are
	// never hidden, for orientation) — reuses ResourceTable's rowFilter
	// type (table.go) for its query/filtering fields and its handleKey's
	// enter/esc/backspace/typing state machine, rather than duplicating
	// that switch here. Only query and filtering are used: rebuild()
	// re-derives which rows are visible from scratch on every query change
	// (it isn't index-position-based like ResourceTable's rows, so
	// rowFilter's matches/absolute/len — built for a flat, already-loaded
	// row slice — don't apply here and are left unused).
	filter rowFilter
}

func NewNamespacesInfo() *NamespacesInfo {
	// See newPaneList's doc comment for why every option it sets is needed.
	newList := newPaneList(namespaceDelegate{})
	return &NamespacesInfo{
		list:                   newList,
		namespacesByContext:    make(map[string][]string),
		errByContext:           make(map[string]string),
		checked:                make(map[string]map[string]bool),
		previouslyConfirmed:    make(map[string]map[string]bool),
		allNamespaces:          make(map[string]bool),
		previouslyConfirmedAll: make(map[string]bool),
		preSelectAll:           make(map[string]map[string]bool),
	}
}

// SetContextNamespaces records a successful namespace-list fetch for one
// context and rebuilds the pane. Callers should follow this with
// SyncConfirmed to seed which namespace(s) already show checked.
func (n *NamespacesInfo) SetContextNamespaces(context string, namespaces []string) {
	delete(n.errByContext, context)
	n.namespacesByContext[context] = namespaces
	if n.checked[context] == nil {
		n.checked[context] = make(map[string]bool)
	}
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
	delete(n.checked, context)
	delete(n.previouslyConfirmed, context)
	delete(n.allNamespaces, context)
	delete(n.previouslyConfirmedAll, context)
	delete(n.preSelectAll, context)
	n.rebuild()
}

// SyncConfirmed resets one context's confirmed *and* pending-checked set to
// exactly match `selected`, and its all-namespaces flag to match `allNS` —
// called right after a fetch completes (to seed the initial checked
// namespace), and after MainPage reconciles a confirm with AppState, since
// AppState may have refused part of the checked-set diff (e.g. it never
// lets the last checked namespace be removed) — without this, the pane
// could keep showing an unchecked box for a namespace still actually
// shown. Also drops any in-flight "a" toggle checkpoint for the context,
// since it no longer corresponds to a real prior state once reality has
// been resynced from outside.
func (n *NamespacesInfo) SyncConfirmed(context string, selected []string, allNS bool) {
	set := make(map[string]bool, len(selected))
	for _, ns := range selected {
		set[ns] = true
	}
	n.previouslyConfirmed[context] = set
	n.checked[context] = copyBoolSet(set)
	n.previouslyConfirmedAll[context] = allNS
	if allNS {
		n.allNamespaces[context] = true
	} else {
		delete(n.allNamespaces, context)
	}
	delete(n.preSelectAll, context)
	n.rebuild()
}

// copyBoolSet duplicates state.AppState's private copyBoolMap helper
// (internal/state/state.go). Left undeduplicated rather than exporting
// that one and importing internal/state here: models is a "dumb" UI-widget
// package with no domain-layer dependency today (pages orchestrates both
// models and state), and pulling in internal/state for one 6-line map-copy
// helper isn't worth establishing that coupling.
func copyBoolSet(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// rebuild reconstructs the flattened item list from namespacesByContext/
// errByContext/checked, sorted by context name, applying the "/" filter (if
// any) to which namespace rows are included. Selected always comes from the
// persistent `checked` map, never from the outgoing item list — so
// filtering a row out of view can never lose its checked state, unlike
// deriving pending state from the live list itself would.
func (n *NamespacesInfo) rebuild() {
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

	query := strings.ToLower(n.filter.query)

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
		items = append(items, namespaceRow{Context: c, IsHeader: true, AllNS: n.allNamespaces[c]})
		for _, ns := range namespaces {
			if query != "" && !strings.Contains(strings.ToLower(ns), query) {
				continue
			}
			items = append(items, namespaceRow{Context: c, Name: ns, Selected: n.checked[c][ns]})
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
	n.list.SetDelegate(namespaceDelegate{focused: f})
}

// FilterStatus reports the current filter text and visible-row count, for
// the status bar's "/query (N matches)" indicator — mirrors
// ResourceTable.FilterStatus. ok is false when no filter is active (neither
// being typed nor already committed).
func (n *NamespacesInfo) FilterStatus() (query string, matches int, typing bool, ok bool) {
	if !n.filter.filtering && n.filter.query == "" {
		return "", 0, false, false
	}
	count := 0
	for _, item := range n.list.Items() {
		if row, isRow := item.(namespaceRow); isRow && !row.IsHeader {
			count++
		}
	}
	return n.filter.query, count, n.filter.filtering, true
}

func (n *NamespacesInfo) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	if n.filter.filtering {
		// n.rebuild() re-derives the visible row set from n.filter.query
		// from scratch (it isn't index-position-based, so handleKey's own
		// recompute — which n.filter.query no-op matchFn feeds — is inert
		// here; see the filter field's doc comment). Calling it after every
		// key, including Enter (which doesn't actually change the query),
		// is a harmless no-op re-render rather than a behavior change.
		n.filter.handleKey(key, 0, func(int) bool { return false })
		n.rebuild()
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
	case "a":
		n.toggleAllNamespaces()
		return nil
	case "/":
		n.filter.filtering = true
		return nil
	case "enter":
		return n.confirmChanges()
	}
	return nil
}

// toggleRow toggles a single namespace's checked state. If its context is
// currently in all-namespaces mode, this exits that mode first, seeding the
// checked set from scratch with *only* the namespace under the cursor
// checked — every row renders unchecked in all-mode (see rebuild), so
// pressing Space on one should check just that one, matching what's on
// screen, rather than implicitly checking every other namespace too.
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

	if n.allNamespaces[row.Context] {
		n.checked[row.Context] = map[string]bool{row.Name: true}
		delete(n.allNamespaces, row.Context)
		delete(n.preSelectAll, row.Context)
		n.rebuild()
		return
	}

	if n.checked[row.Context] == nil {
		n.checked[row.Context] = make(map[string]bool)
	}
	n.checked[row.Context][row.Name] = !n.checked[row.Context][row.Name]
	n.rebuild()
}

// toggleAllNamespaces toggles all-namespaces mode for the context at the
// cursor (the header row, or any namespace row beneath it): entering it
// saves the current checked set (for later restore) and clears the checked
// set — rows show unchecked, with the header's "(all namespaces)" suffix
// standing in for the selection instead — while leaving it restores the
// saved checked set, so repeated presses act as a toggle rather than a
// one-way action.
func (n *NamespacesInfo) toggleAllNamespaces() {
	idx := n.list.Index()
	items := n.list.Items()
	if idx < 0 || idx >= len(items) {
		return
	}
	row, ok := items[idx].(namespaceRow)
	if !ok {
		return
	}
	context := row.Context
	if len(n.namespacesByContext[context]) == 0 {
		return
	}

	if n.allNamespaces[context] {
		saved := n.preSelectAll[context]
		n.checked[context] = copyBoolSet(saved)
		delete(n.preSelectAll, context)
		delete(n.allNamespaces, context)
	} else {
		n.preSelectAll[context] = copyBoolSet(n.checked[context])
		n.checked[context] = make(map[string]bool)
		n.allNamespaces[context] = true
	}
	n.rebuild()
}

// confirmChanges diffs every context's currently-checked namespaces against
// previouslyConfirmed, and its all-namespaces flag against
// previouslyConfirmedAll, returning one command per context with a change,
// each carrying a msgs.NamespacesStateMsg — nil if nothing changed anywhere.
func (n *NamespacesInfo) confirmChanges() tea.Cmd {
	contexts := make(map[string]bool)
	for c := range n.checked {
		contexts[c] = true
	}
	for c := range n.previouslyConfirmed {
		contexts[c] = true
	}
	for c := range n.allNamespaces {
		contexts[c] = true
	}
	for c := range n.previouslyConfirmedAll {
		contexts[c] = true
	}

	var cmds []tea.Cmd
	for context := range contexts {
		var added, removed []string
		for ns, isChecked := range n.checked[context] {
			if isChecked && !n.previouslyConfirmed[context][ns] {
				added = append(added, ns)
			}
		}
		for ns := range n.previouslyConfirmed[context] {
			if !n.checked[context][ns] {
				removed = append(removed, ns)
			}
		}

		var allNS *bool
		if n.allNamespaces[context] != n.previouslyConfirmedAll[context] {
			v := n.allNamespaces[context]
			allNS = &v
		}

		if len(added) == 0 && len(removed) == 0 && allNS == nil {
			continue
		}
		sort.Strings(added)
		sort.Strings(removed)
		state := msgs.NamespacesStateMsg{Context: context, Added: added, Removed: removed, AllNamespaces: allNS}
		cmds = append(cmds, func() tea.Msg { return state })
	}

	n.previouslyConfirmed = make(map[string]map[string]bool, len(n.checked))
	for c, set := range n.checked {
		n.previouslyConfirmed[c] = copyBoolSet(set)
	}
	n.previouslyConfirmedAll = make(map[string]bool, len(n.allNamespaces))
	for c, v := range n.allNamespaces {
		n.previouslyConfirmedAll[c] = v
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (n *NamespacesInfo) View() string {
	return n.list.View()
}
