package pages

import (
	"testing"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func row(context, namespace string) msgs.RowData {
	return msgs.RowData{msgs.KeyContext: context, msgs.KeyNamespace: namespace}
}

// TestFilterRowsByNamespace_ChecksSelectedOnly guards the core local filter:
// only rows whose (context, namespace) is checked survive, unrelated
// namespaces in the same context are dropped.
func TestFilterRowsByNamespace_ChecksSelectedOnly(t *testing.T) {
	rows := []msgs.RowData{row("ctx1", "default"), row("ctx1", "kube-system")}
	selected := map[string][]string{"ctx1": {"default"}}

	got := filterRowsByNamespace(rows, selected, nil)
	if len(got) != 1 || got[0][msgs.KeyNamespace] != "default" {
		t.Fatalf("expected only the default-namespace row to survive, got %+v", got)
	}
}

// TestFilterRowsByNamespace_AllNamespacesBypassesTheCheckedSet guards
// all-namespaces mode: every row for that context survives regardless of
// what's checked (or even if nothing is checked at all).
func TestFilterRowsByNamespace_AllNamespacesBypassesTheCheckedSet(t *testing.T) {
	rows := []msgs.RowData{row("ctx1", "default"), row("ctx1", "kube-system")}
	selected := map[string][]string{"ctx1": {}}
	allNS := map[string]bool{"ctx1": true}

	got := filterRowsByNamespace(rows, selected, allNS)
	if len(got) != 2 {
		t.Fatalf("expected every row to survive in all-namespaces mode, got %+v", got)
	}
}

// TestFilterRowsByNamespace_MultiContextIndependence guards that one
// context's all-namespaces mode doesn't leak into another context's
// checked-set filtering in the same row set.
func TestFilterRowsByNamespace_MultiContextIndependence(t *testing.T) {
	rows := []msgs.RowData{row("ctx1", "default"), row("ctx1", "kube-system"), row("ctx2", "default"), row("ctx2", "kube-system")}
	selected := map[string][]string{"ctx1": {"default"}, "ctx2": {"default"}}
	allNS := map[string]bool{"ctx2": true}

	got := filterRowsByNamespace(rows, selected, allNS)
	if len(got) != 3 {
		t.Fatalf("expected ctx1's one checked row plus ctx2's two rows, got %+v", got)
	}
	for _, r := range got {
		if r[msgs.KeyContext] == "ctx1" && r[msgs.KeyNamespace] != "default" {
			t.Fatalf("expected ctx1 still filtered to its checked namespace, got %+v", r)
		}
	}
}
