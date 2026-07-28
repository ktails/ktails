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

// TestMarkLoaded_TracksEachKindIndependently guards the per-tab unlock: each
// kind must flip loaded in LoadedKinds the moment it delivers, independent
// of every other kind — unlike LoadedContexts (gated on Deployments+Pods
// together for the Contexts pane checkmark), a tab like Services must not
// wait on Deployments finishing first.
func TestMarkLoaded_TracksEachKindIndependently(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")

	if a.Snapshot().LoadedKinds[msgs.KindServices]["ctx1"] {
		t.Fatal("expected Services not loaded yet")
	}

	a.MarkLoaded(msgs.KindServices, "ctx1")
	snap := a.Snapshot()
	if !snap.LoadedKinds[msgs.KindServices]["ctx1"] {
		t.Fatal("expected Services loaded immediately after MarkLoaded, without Deployments/Pods")
	}
	if snap.LoadedKinds[msgs.KindDeployments]["ctx1"] {
		t.Fatal("expected Deployments to remain unloaded — only Services has delivered")
	}

	a.RemoveContext("ctx1")
	if a.Snapshot().LoadedKinds[msgs.KindServices]["ctx1"] {
		t.Fatal("expected RemoveContext to clear per-kind loaded state too")
	}
}

// TestSetAllNamespaces_DistinctFromEmptyCheckedSet guards the tri-state
// namespace filter: a context with zero checked namespaces and a context in
// all-namespaces mode must be told apart, and RemoveContext must clear the
// flag along with everything else.
func TestSetAllNamespaces_DistinctFromEmptyCheckedSet(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "default")

	if a.Snapshot().AllNamespaces["ctx1"] {
		t.Fatal("expected ctx1 not in all-namespaces mode by default")
	}

	a.SetAllNamespaces("ctx1", true)
	if !a.Snapshot().AllNamespaces["ctx1"] {
		t.Fatal("expected ctx1 in all-namespaces mode after SetAllNamespaces(true)")
	}

	a.SetAllNamespaces("ctx1", false)
	if a.Snapshot().AllNamespaces["ctx1"] {
		t.Fatal("expected ctx1 out of all-namespaces mode after SetAllNamespaces(false)")
	}

	a.SetAllNamespaces("ctx1", true)
	a.RemoveContext("ctx1")
	if a.Snapshot().AllNamespaces["ctx1"] {
		t.Fatal("expected RemoveContext to clear all-namespaces mode too")
	}
}

// TestAddContext_EmptyNamespaceEntersAllNamespacesMode guards a real-cluster
// regression: a kubeconfig context with no namespace directive at all
// (raw value "", distinct from an explicit "default") must default to
// all-namespaces mode, not a checked set containing the empty string —
// filterRowsByNamespace matches checked namespaces against a row's real
// namespace string, which "" could never match, so seeding checked=[""]
// silently filtered every row out.
func TestAddContext_EmptyNamespaceEntersAllNamespacesMode(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "")

	snap := a.Snapshot()
	if !snap.AllNamespaces["ctx1"] {
		t.Fatal("expected an empty kubeconfig namespace to enter all-namespaces mode")
	}
	if len(snap.SelectedContexts["ctx1"]) != 0 {
		t.Fatalf("expected no checked namespaces in all-namespaces mode, got %v", snap.SelectedContexts["ctx1"])
	}

	// An explicit namespace must still scope normally, not stay in
	// all-namespaces mode from a prior AddContext call.
	a.AddContext("ctx2", "default")
	snap = a.Snapshot()
	if snap.AllNamespaces["ctx2"] {
		t.Fatal("expected an explicit namespace to not enter all-namespaces mode")
	}
	if got := snap.SelectedContexts["ctx2"]; len(got) != 1 || got[0] != "default" {
		t.Fatalf("expected ctx2 scoped to just 'default', got %v", got)
	}
}

// TestDefaultNamespace_TracksOriginalAndClearsOnRemove guards the fallback
// MainPage uses when a context confirms with nothing checked (see
// applyNamespacesState): DefaultNamespace must keep reporting the original
// value AddContext seeded a context with, regardless of later
// Add/RemoveNamespace/SetAllNamespaces churn, until RemoveContext clears it.
func TestDefaultNamespace_TracksOriginalAndClearsOnRemove(t *testing.T) {
	a := NewAppState()
	a.AddContext("ctx1", "kube-system")

	if got := a.DefaultNamespace("ctx1"); got != "kube-system" {
		t.Fatalf("expected DefaultNamespace to report kube-system, got %q", got)
	}

	// Churning the checked set and all-namespaces mode must not change it.
	a.AddNamespace("ctx1", "other-ns")
	a.RemoveNamespace("ctx1", "kube-system")
	a.SetAllNamespaces("ctx1", true)
	if got := a.DefaultNamespace("ctx1"); got != "kube-system" {
		t.Fatalf("expected DefaultNamespace to stay kube-system despite churn, got %q", got)
	}

	a.RemoveContext("ctx1")
	if got := a.DefaultNamespace("ctx1"); got != "" {
		t.Fatalf("expected DefaultNamespace cleared after RemoveContext, got %q", got)
	}
}

// TestRemoveNamespace_CanDropTheLastOne guards a real regression: checked
// namespaces are a display-only filter now (see filterRowsByNamespace), not
// tied to a watch's lifecycle, so dropping the last checked namespace is a
// legitimate "show nothing for this context" state — RemoveNamespace must
// not silently refuse it. A prior version did refuse, which caused the
// Namespaces pane to visibly re-check a box the user had just deselected
// the moment they confirmed.
func TestRemoveNamespace_CanDropTheLastOne(t *testing.T) {
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

	// Removing the last one must now succeed, leaving zero checked.
	a.RemoveNamespace("ctx1", "other-ns")
	if got := a.Snapshot().SelectedContexts["ctx1"]; len(got) != 0 {
		t.Fatalf("expected zero namespaces left, got %v", got)
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
