package pages

import "testing"

// TestAllContextsLoaded_PerKindIndependence guards the per-tab unlock: the
// gate is now evaluated per target-kind (the caller passes
// snapshot.LoadedKinds[targetKind]), so one kind being loaded must not be
// mistaken for another kind also being loaded.
func TestAllContextsLoaded_PerKindIndependence(t *testing.T) {
	selected := map[string][]string{"ctx1": {"default"}}

	deploymentsLoaded := map[string]bool{"ctx1": true}
	servicesLoaded := map[string]bool{}

	if !allContextsLoaded(selected, deploymentsLoaded, nil) {
		t.Fatal("expected Deployments tab unlocked once its own kind has loaded")
	}
	if allContextsLoaded(selected, servicesLoaded, nil) {
		t.Fatal("expected Services tab to remain locked — only Deployments has loaded so far")
	}
}

// TestAllContextsLoaded_ErrorAlsoUnlocks guards that a context which gave up
// with an error still counts as "settled" for a kind it never loaded — an
// RBAC denial on one context must not permanently lock every tab.
func TestAllContextsLoaded_ErrorAlsoUnlocks(t *testing.T) {
	selected := map[string][]string{"ctx1": {"default"}}
	loaded := map[string]bool{}
	errors := map[string]string{"ctx1": "RBAC: not permitted"}

	if !allContextsLoaded(selected, loaded, errors) {
		t.Fatal("expected a context with an error to count as settled even without a load")
	}
}

// TestAllContextsLoaded_EmptySelectionNeverUnlocks guards that with no
// context selected at all, tab navigation stays locked rather than
// vacuously unlocking.
func TestAllContextsLoaded_EmptySelectionNeverUnlocks(t *testing.T) {
	if allContextsLoaded(nil, map[string]bool{}, nil) {
		t.Fatal("expected no selected contexts to never report loaded")
	}
}

// TestAllContextsLoaded_RequiresEverySelectedContext guards the multi-context
// case: a kind must be loaded (or errored) for every selected context, not
// just some, before that tab unlocks.
func TestAllContextsLoaded_RequiresEverySelectedContext(t *testing.T) {
	selected := map[string][]string{"ctx1": {"default"}, "ctx2": {"default"}}
	loaded := map[string]bool{"ctx1": true}

	if allContextsLoaded(selected, loaded, nil) {
		t.Fatal("expected the tab to stay locked until ctx2 also settles")
	}

	loaded["ctx2"] = true
	if !allContextsLoaded(selected, loaded, nil) {
		t.Fatal("expected the tab to unlock once every selected context settled")
	}
}
