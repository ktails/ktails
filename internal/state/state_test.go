package state

import (
	"testing"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// TestMarkLoaded_RequiresBothDeploymentsAndPods guards the tab-navigation
// sync: a context must not count as "loaded" (LoadedContexts) until both
// Deployments and Pods have each delivered a first successful load — not
// just Deployments alone.
func TestMarkLoaded_RequiresBothDeploymentsAndPods(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")

	a.MarkLoaded(msgs.KindDeployments, "ctx1")
	if a.Snapshot().LoadedContexts["ctx1"] {
		t.Fatal("expected ctx1 not loaded yet — only Deployments has delivered")
	}

	// Other kinds delivering in between must not satisfy the requirement.
	a.MarkLoaded(msgs.KindConfigMaps, "ctx1")
	if a.Snapshot().LoadedContexts["ctx1"] {
		t.Fatal("expected ctx1 still not loaded — ConfigMaps isn't one of the required kinds")
	}

	a.MarkLoaded(msgs.KindPods, "ctx1")
	if !a.Snapshot().LoadedContexts["ctx1"] {
		t.Fatal("expected ctx1 loaded once both Deployments and Pods have delivered")
	}
}

// TestMarkLoaded_ClearsErrorOnceBothRequiredKindsDeliver guards that a
// context's error is cleared exactly when LoadedContexts flips true, not
// prematurely on the first required kind.
func TestMarkLoaded_ClearsErrorOnceBothRequiredKindsDeliver(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")
	a.SetError("ctx1", "stale error")

	a.MarkLoaded(msgs.KindDeployments, "ctx1")
	if a.Snapshot().Errors["ctx1"] == "" {
		t.Fatal("expected the error to survive until Pods also delivers")
	}

	a.MarkLoaded(msgs.KindPods, "ctx1")
	if a.Snapshot().Errors["ctx1"] != "" {
		t.Fatal("expected the error cleared once both required kinds delivered")
	}
}

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
