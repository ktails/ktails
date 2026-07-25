package models

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ktails/ktails/internal/tui/msgs"
)

func newTestContextsInfo(items ...contextList) *ContextsInfo {
	c := &ContextsInfo{
		list:               list.New([]list.Item{}, contextDelegate{}, 0, 0),
		previouslySelected: make(map[string]bool),
	}
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	c.list.SetItems(listItems)
	return c
}

func mustState(t *testing.T, cmd tea.Cmd) msgs.ContextsStateMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command carrying a ContextsStateMsg, got nil")
	}
	msg := cmd()
	state, ok := msg.(msgs.ContextsStateMsg)
	if !ok {
		t.Fatalf("expected ContextsStateMsg, got %T", msg)
	}
	return state
}

// TestConfirmSelection_QuickSelectPersistsCheckbox guards the fix for a
// latent bug: pressing Enter with nothing Space-toggled (the quick-select
// shortcut) must actually flip the cursor row's own Selected flag — not
// just report it in the outgoing message — so the row's "◉ selected" icon
// is correct before the watch finishes loading.
func TestConfirmSelection_QuickSelectPersistsCheckbox(t *testing.T) {
	c := newTestContextsInfo(
		contextList{Name: "ctx-a", DefaultNamespace: "default"},
		contextList{Name: "ctx-b", DefaultNamespace: "default"},
	)
	c.list.Select(0)

	state := mustState(t, c.confirmSelection())
	if len(state.Added) != 1 || state.Added[0].ContextName != "ctx-a" {
		t.Fatalf("expected ctx-a added via quick-select, got %+v", state.Added)
	}

	item, ok := c.list.Items()[0].(contextList)
	if !ok || !item.Selected {
		t.Fatalf("expected the cursor row's Selected flag to be set, got %+v", item)
	}
}

func TestConfirmSelection_NoOpWhenNothingChanged(t *testing.T) {
	c := newTestContextsInfo(contextList{Name: "ctx-a", DefaultNamespace: "default"})
	c.list.Select(0)

	// First confirm: quick-select adds ctx-a.
	mustState(t, c.confirmSelection())

	// Second confirm with no change in between must be a no-op — not a
	// re-add, and not a spurious quick-select of an already-checked row.
	if cmd := c.confirmSelection(); cmd != nil {
		t.Fatalf("expected nil command on an unchanged re-confirm, got a command")
	}
}

func TestGetAllContextStates_DiffsAddedAndDeselected(t *testing.T) {
	c := newTestContextsInfo(
		contextList{Name: "ctx-a", DefaultNamespace: "default", Selected: true},
		contextList{Name: "ctx-b", DefaultNamespace: "default"},
	)

	// First confirm: ctx-a is added, nothing deselected yet.
	state := c.getAllContextStates()
	if len(state.Added) != 1 || state.Added[0].ContextName != "ctx-a" {
		t.Fatalf("expected ctx-a added, got %+v", state.Added)
	}
	if len(state.Deselected) != 0 {
		t.Fatalf("expected nothing deselected yet, got %v", state.Deselected)
	}

	// Re-running with no widget changes must report nothing new.
	state = c.getAllContextStates()
	if len(state.Added) != 0 || len(state.Deselected) != 0 {
		t.Fatalf("expected no changes on unchanged state, got Added=%v Deselected=%v", state.Added, state.Deselected)
	}

	// Deselect ctx-a, select ctx-b: one of each.
	items := c.list.Items()
	items[0] = contextList{Name: "ctx-a", DefaultNamespace: "default", Selected: false}
	items[1] = contextList{Name: "ctx-b", DefaultNamespace: "default", Selected: true}
	c.list.SetItems(items)

	state = c.getAllContextStates()
	if len(state.Added) != 1 || state.Added[0].ContextName != "ctx-b" {
		t.Fatalf("expected ctx-b added, got %+v", state.Added)
	}
	if len(state.Deselected) != 1 || state.Deselected[0] != "ctx-a" {
		t.Fatalf("expected ctx-a deselected, got %v", state.Deselected)
	}
}
