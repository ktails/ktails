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
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	n.checked["ctx1"]["default"] = false
	n.checked["ctx1"]["other-ns"] = true
	n.rebuild()

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
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	if cmd := n.confirmChanges(); cmd != nil {
		t.Fatal("expected nil command when nothing changed since the last confirm")
	}
}

// TestNamespaces_RebuildPreservesPendingToggles guards a real bug shape: an
// unrelated context's async namespace-list arriving mid-session (which
// triggers rebuild() for every context) must not discard a not-yet-confirmed
// checkbox toggle already made in a different context's rows — checked is
// now the persistent source of truth precisely so this can't happen.
func TestNamespaces_RebuildPreservesPendingToggles(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	// Pending toggle: check other-ns, without confirming yet.
	n.checked["ctx1"]["other-ns"] = true
	n.rebuild()

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
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	// Pending uncheck of the only namespace — not yet confirmed.
	n.checked["ctx1"]["default"] = false
	n.rebuild()

	// AppState refused the removal (it's the last namespace) — MainPage
	// calls SyncConfirmed with what's still actually selected.
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	for _, item := range n.list.Items() {
		row := item.(namespaceRow)
		if row.Name == "default" && !row.Selected {
			t.Fatal("expected SyncConfirmed to re-check default after a refused removal")
		}
	}
}

// namespaceRowSelected finds a namespace row by context+name and reports
// whether it's currently checked, failing the test if the row isn't found.
func namespaceRowSelected(t *testing.T, n *NamespacesInfo, context, name string) bool {
	t.Helper()
	for _, item := range n.list.Items() {
		row, ok := item.(namespaceRow)
		if ok && !row.IsHeader && row.Context == context && row.Name == name {
			return row.Selected
		}
	}
	t.Fatalf("expected a row for %s/%s, found none", context, name)
	return false
}

// TestNamespaces_ToggleAllEntersAllModeThenRestoresPriorSet guards the "a"
// key: it must enter all-namespaces mode (every row visibly unchecked — see
// rebuild's AllNS header suffix — rather than every box checked), saving
// the prior checked set, then on a second press leave that mode and restore
// exactly that prior set.
func TestNamespaces_ToggleAllEntersAllModeThenRestoresPriorSet(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "kube-system", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	// Cursor sits on the header row (index 0) — toggling from there should
	// still affect the whole context's group.
	n.list.Select(0)
	n.toggleAllNamespaces()

	if !n.allNamespaces["ctx1"] {
		t.Fatal("expected ctx1 to be in all-namespaces mode after toggle-all")
	}
	for _, ns := range []string{"default", "kube-system", "other-ns"} {
		if namespaceRowSelected(t, n, "ctx1", ns) {
			t.Fatalf("expected %s visibly unchecked in all-namespaces mode, got checked", ns)
		}
	}

	// Second press leaves all-namespaces mode, restoring the pre-toggle set
	// (just "default").
	n.toggleAllNamespaces()

	if n.allNamespaces["ctx1"] {
		t.Fatal("expected ctx1 to have left all-namespaces mode")
	}
	if !namespaceRowSelected(t, n, "ctx1", "default") {
		t.Fatal("expected default to be checked again after restoring the prior set")
	}
	if namespaceRowSelected(t, n, "ctx1", "kube-system") {
		t.Fatal("expected kube-system to remain unchecked after restoring the prior set")
	}
	if namespaceRowSelected(t, n, "ctx1", "other-ns") {
		t.Fatal("expected other-ns to remain unchecked after restoring the prior set")
	}
}

// TestNamespaces_ToggleAllConfirmReportsAllNamespaces guards that entering
// all-namespaces mode and confirming reports it on NamespacesStateMsg, and
// that a no-op confirm (nothing changed) leaves it nil.
func TestNamespaces_ToggleAllConfirmReportsAllNamespaces(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	n.list.Select(0)
	n.toggleAllNamespaces()

	state := mustNamespacesState(t, n.confirmChanges())
	if state.AllNamespaces == nil || !*state.AllNamespaces {
		t.Fatalf("expected AllNamespaces=true reported, got %v", state.AllNamespaces)
	}

	// Confirming again with nothing changed must report a nil command.
	if cmd := n.confirmChanges(); cmd != nil {
		t.Fatal("expected nil command when nothing changed since the last confirm")
	}
}

// TestNamespaces_SpaceExitsAllModeSeedingFromEveryoneMinusToggled guards
// toggleRow's all-mode exit: toggling a single namespace while in
// all-namespaces mode must leave every other known namespace checked and
// only the toggled one unchecked, not start from an empty set.
func TestNamespaces_SpaceExitsAllModeSeedingFromEveryoneMinusToggled(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "kube-system", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	n.list.Select(0)
	n.toggleAllNamespaces()
	if !n.allNamespaces["ctx1"] {
		t.Fatal("expected ctx1 in all-namespaces mode before the Space toggle")
	}

	// Move the cursor onto the "kube-system" row and toggle it.
	for i, item := range n.list.Items() {
		if row, ok := item.(namespaceRow); ok && !row.IsHeader && row.Name == "kube-system" {
			n.list.Select(i)
		}
	}
	n.toggleRow()

	if n.allNamespaces["ctx1"] {
		t.Fatal("expected the Space toggle to exit all-namespaces mode")
	}
	if namespaceRowSelected(t, n, "ctx1", "kube-system") {
		t.Fatal("expected kube-system unchecked — it's the one just toggled")
	}
	if !namespaceRowSelected(t, n, "ctx1", "default") {
		t.Fatal("expected default checked — seeded from everyone minus kube-system")
	}
	if !namespaceRowSelected(t, n, "ctx1", "other-ns") {
		t.Fatal("expected other-ns checked — seeded from everyone minus kube-system")
	}
}

// TestNamespaces_FilterHidesNonMatchingRowsWithoutLosingState guards the "/"
// filter: a namespace checked before filtering must still count as checked
// (and get included in a confirm) even while hidden from view by the
// filter, and clearing the filter must bring it back visibly checked.
func TestNamespaces_FilterHidesNonMatchingRowsWithoutLosingState(t *testing.T) {
	n := NewNamespacesInfo()
	n.SetContextNamespaces("ctx1", []string{"default", "kube-system", "other-ns"})
	n.SyncConfirmed("ctx1", []string{"default"}, false)

	// Check kube-system, then filter it out of view.
	n.checked["ctx1"]["kube-system"] = true
	n.filterQuery = "other"
	n.rebuild()

	found := false
	for _, item := range n.list.Items() {
		row, ok := item.(namespaceRow)
		if ok && !row.IsHeader {
			found = true
			if row.Name != "other-ns" {
				t.Fatalf("expected only other-ns visible under the filter, also saw %s", row.Name)
			}
		}
	}
	if !found {
		t.Fatal("expected other-ns to still be visible (it matches the filter)")
	}

	// Confirming while filtered must still report kube-system as added —
	// its checked state was never lost, just hidden.
	state := mustNamespacesState(t, n.confirmChanges())
	found = false
	for _, ns := range state.Added {
		if ns == "kube-system" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected kube-system in Added despite being filtered out of view, got %v", state.Added)
	}

	// Clearing the filter must show it checked again.
	n.filterQuery = ""
	n.rebuild()
	if !namespaceRowSelected(t, n, "ctx1", "kube-system") {
		t.Fatal("expected kube-system to show checked again once the filter clears")
	}
}
