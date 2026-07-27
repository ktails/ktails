package models

import "testing"

// TestClustersRefresh_GroupsByClusterAndTracksAllSelected guards the core
// Clusters-pane behavior: contexts sharing a cluster are grouped together,
// and a group only reports AllSelected once every context under it is
// checked in the Contexts pane.
func TestClustersRefresh_GroupsByClusterAndTracksAllSelected(t *testing.T) {
	contexts := newTestContextsInfo(
		contextList{Name: "ctx-a", Cluster: "prod", DefaultNamespace: "default", Selected: true},
		contextList{Name: "ctx-b", Cluster: "prod", DefaultNamespace: "default", Selected: false},
		contextList{Name: "ctx-c", Cluster: "staging", DefaultNamespace: "default", Selected: true},
	)
	cl := NewClustersInfo(contexts)
	cl.Refresh()

	items := cl.list.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 cluster groups, got %d: %+v", len(items), items)
	}

	var prod, staging clusterGroup
	for _, item := range items {
		g := item.(clusterGroup)
		switch g.Name {
		case "prod":
			prod = g
		case "staging":
			staging = g
		}
	}

	if prod.AllSelected {
		t.Fatalf("expected prod not fully selected (ctx-b unchecked), got %+v", prod)
	}
	if len(prod.ContextNames) != 2 {
		t.Fatalf("expected prod to have 2 contexts, got %v", prod.ContextNames)
	}
	if !staging.AllSelected {
		t.Fatalf("expected staging fully selected, got %+v", staging)
	}
}

// TestClustersToggleGroup_BulkSelectsEveryContext guards the bulk
// select/deselect behavior: toggling a cluster group with Space must flip
// every context under it in the underlying Contexts pane, not just the
// group's own aggregate flag.
func TestClustersToggleGroup_BulkSelectsEveryContext(t *testing.T) {
	contexts := newTestContextsInfo(
		contextList{Name: "ctx-a", Cluster: "prod", DefaultNamespace: "default"},
		contextList{Name: "ctx-b", Cluster: "prod", DefaultNamespace: "default"},
	)
	cl := NewClustersInfo(contexts)
	cl.Refresh()
	cl.list.Select(0)

	cl.toggleGroup()

	for _, item := range contexts.list.Items() {
		ctx := item.(contextList)
		if !ctx.Selected {
			t.Fatalf("expected %s to be selected after bulk toggle, got %+v", ctx.Name, ctx)
		}
	}

	// Toggling again with everything selected must deselect the whole group.
	cl.toggleGroup()
	for _, item := range contexts.list.Items() {
		ctx := item.(contextList)
		if ctx.Selected {
			t.Fatalf("expected %s to be deselected after second bulk toggle, got %+v", ctx.Name, ctx)
		}
	}
}

// TestClustersConfirm_DrivesContextsConfirmChanges guards that Enter on the
// Clusters pane reaches the same diff/confirm pipeline the Contexts pane
// uses, rather than a separate, potentially-diverging implementation.
func TestClustersConfirm_DrivesContextsConfirmChanges(t *testing.T) {
	contexts := newTestContextsInfo(
		contextList{Name: "ctx-a", Cluster: "prod", DefaultNamespace: "default"},
	)
	cl := NewClustersInfo(contexts)
	cl.Refresh()
	cl.list.Select(0)
	cl.toggleGroup()

	state := mustState(t, cl.contexts.ConfirmChanges())
	if len(state.Added) != 1 || state.Added[0].ContextName != "ctx-a" {
		t.Fatalf("expected ctx-a added via cluster confirm, got %+v", state.Added)
	}
}
