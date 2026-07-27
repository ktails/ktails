package state

import "testing"

// TestAddRemoveNamespace_NeverDropsTheLastOne guards the safety backstop:
// unchecking every namespace for a context would leave it selected but
// watching nothing, so RemoveNamespace must refuse to remove the last one.
func TestAddRemoveNamespace_NeverDropsTheLastOne(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")

	a.AddNamespace("ctx1", "other-ns")
	if got := a.Snapshot().SelectedContexts["ctx1"]; len(got) != 2 {
		t.Fatalf("expected 2 namespaces after AddNamespace, got %v", got)
	}

	a.RemoveNamespace("ctx1", "default")
	if got := a.Snapshot().SelectedContexts["ctx1"]; len(got) != 1 || got[0] != "other-ns" {
		t.Fatalf("expected only other-ns left, got %v", got)
	}

	// Removing the last one must be a no-op.
	a.RemoveNamespace("ctx1", "other-ns")
	if got := a.Snapshot().SelectedContexts["ctx1"]; len(got) != 1 || got[0] != "other-ns" {
		t.Fatalf("expected the last namespace to survive removal, got %v", got)
	}
}

// TestAddNamespace_DedupesAndIgnoresUnselectedContext guards two edge cases:
// re-adding an already-checked namespace shouldn't duplicate it, and adding
// to a context that was never selected (already removed, or never added)
// must be a safe no-op rather than resurrecting a stale entry.
func TestAddNamespace_DedupesAndIgnoresUnselectedContext(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")

	a.AddNamespace("ctx1", "default")
	if got := a.Snapshot().SelectedContexts["ctx1"]; len(got) != 1 {
		t.Fatalf("expected re-adding the existing namespace to be a no-op, got %v", got)
	}

	a.AddNamespace("ctx-never-selected", "default")
	if _, ok := a.Snapshot().SelectedContexts["ctx-never-selected"]; ok {
		t.Fatal("expected AddNamespace to ignore a context that was never selected")
	}
}
