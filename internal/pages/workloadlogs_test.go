package pages

import (
	"errors"
	"testing"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// TestApplyWorkloadPods_ResolvesFromPodsCache guards the aggregate-log path:
// once a workload's owned Pod names come back (msgs.WorkloadPodsMsg), each
// must be looked up in the already-loaded Pods-tab cache (for its
// containers) and merged into the Logs pane — pods not present in that
// cache are silently skipped rather than failing the whole aggregate.
func TestApplyWorkloadPods_ResolvesFromPodsCache(t *testing.T) {
	m := NewMainPageModel(&k8s.Client{}, 5, 500)
	m.tables[msgs.KindPods].SetRows([]msgs.Row{
		{Context: "ctx1", Namespace: "default", Name: "web-1", Containers: []string{"app"}},
		{Context: "ctx1", Namespace: "default", Name: "web-2", Containers: []string{"app"}},
		{Context: "ctx1", Namespace: "default", Name: "unrelated", Containers: []string{"app"}},
	})

	m.applyWorkloadPods(msgs.WorkloadPodsMsg{
		Context: "ctx1", Namespace: "default", Name: "web", Kind: msgs.KindDeployments,
		// "web-3" isn't in the Pods cache above — it must be skipped rather
		// than erroring the whole aggregate.
		PodNames: []string{"web-1", "web-2", "web-3"},
	})

	if !m.showLogs {
		t.Fatal("expected the Logs pane to open once pods were resolved")
	}
	if !m.podLogs.HasContent() {
		t.Fatal("expected log sources to be added for the resolved pods")
	}
}

// TestApplyWorkloadPods_ErrSetsErrorMessage guards the resolve-failure path
// (e.g. an RBAC denial listing the workload's Pods) — it must surface as the
// existing single-error overlay, not open an empty Logs pane silently.
func TestApplyWorkloadPods_ErrSetsErrorMessage(t *testing.T) {
	m := NewMainPageModel(&k8s.Client{}, 5, 500)

	m.applyWorkloadPods(msgs.WorkloadPodsMsg{
		Context: "ctx1", Namespace: "default", Name: "web", Kind: msgs.KindDeployments,
		Err: errors.New("denied"),
	})

	if m.errorMessage == "" {
		t.Fatal("expected errorMessage to be set on a resolve failure")
	}
	if m.showLogs {
		t.Fatal("expected the Logs pane to stay closed on a resolve failure")
	}
}
