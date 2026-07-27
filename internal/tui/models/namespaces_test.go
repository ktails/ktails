package models

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ktails/ktails/internal/tui/msgs"
)

func mustNamespacesState(t *testing.T, cmd tea.Cmd) msgs.NamespacesStateMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command carrying a NamespacesStateMsg, got nil")
	}
	msg := cmd()
	state, ok := msg.(msgs.NamespacesStateMsg)
	if !ok {
		t.Fatalf("expected NamespacesStateMsg, got %T", msg)
	}
	return state
}

// TestNamespaces_ConfirmDiffsAddedAndRemoved guards the core confirm flow:
// checking a new namespace and unchecking the seeded default in the same
// confirm must report both in one diff.
func TestNamespaces_ConfirmDiffsAddedAndRemoved(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "kube-system", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"})

	items := n.list.Items()
	for idx, item := range items {
		row := item.(namespaceRow)
		switch row.Name {
		case "default":
			row.Selected = false
			items[idx] = row
		case "other-ns":
			row.Selected = true
			items[idx] = row
		}
	}
	n.list.SetItems(items)

	state := mustNamespacesState(t, n.confirmChanges())
	if state.Context != "ctx1" {
		t.Fatalf("expected ctx1, got %s", state.Context)
	}
	if len(state.Added) != 1 || state.Added[0] != "other-ns" {
		t.Fatalf("expected other-ns added, got %v", state.Added)
	}
	if len(state.Removed) != 1 || state.Removed[0] != "default" {
		t.Fatalf("expected default removed, got %v", state.Removed)
	}
}

// TestNamespaces_ConfirmNoOpWhenUnchanged guards against spurious re-confirms.
func TestNamespaces_ConfirmNoOpWhenUnchanged(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default"})
	n.SyncConfirmed("ctx1", []string{"default"})

	if cmd := n.confirmChanges(); cmd != nil {
		t.Fatal("expected nil command when nothing changed since the last confirm")
	}
}

// TestNamespaces_RebuildPreservesPendingToggles guards a real bug shape: an
// unrelated context's async namespace-list arriving mid-session (which
// triggers rebuild() for every context) must not discard a not-yet-confirmed
// checkbox toggle already made in a different context's rows.
func TestNamespaces_RebuildPreservesPendingToggles(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"})

	// Pending toggle: check other-ns, without confirming yet.
	items := n.list.Items()
	for idx, item := range items {
		row := item.(namespaceRow)
		if row.Context == "ctx1" && row.Name == "other-ns" {
			row.Selected = true
			items[idx] = row
		}
	}
	n.list.SetItems(items)

	// A second, unrelated context's fetch lands and triggers another rebuild.
	n.SetContextNamespaces("ctx2", []string{"default"})

	found := false
	for _, item := range n.list.Items() {
		row := item.(namespaceRow)
		if row.Context == "ctx1" && row.Name == "other-ns" {
			found = true
			if !row.Selected {
				t.Fatal("expected the pending toggle on ctx1/other-ns to survive an unrelated rebuild")
			}
		}
	}
	if !found {
		t.Fatal("expected ctx1/other-ns row to still exist after rebuild")
	}
}

// TestNamespaces_SyncConfirmedReconcilesRefusedRemoval guards the
// MainPage-driven reconciliation path: when AppState refuses to drop the
// last checked namespace, SyncConfirmed must re-check that row rather than
// leaving it showing unchecked while still actually watched.
func TestNamespaces_SyncConfirmedReconcilesRefusedRemoval(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default"})
	n.SyncConfirmed("ctx1", []string{"default"})

	items := n.list.Items()
	for idx, item := range items {
		row := item.(namespaceRow)
		if row.Name == "default" {
			row.Selected = false
			items[idx] = row
		}
	}
	n.list.SetItems(items)

	// AppState refused the removal (it's the last namespace) — MainPage
	// calls SyncConfirmed with what's still actually selected.
	n.SyncConfirmed("ctx1", []string{"default"})

	for _, item := range n.list.Items() {
		row := item.(namespaceRow)
		if row.Name == "default" && !row.Selected {
			t.Fatal("expected SyncConfirmed to re-check default after a refused removal")
		}
	}
}
