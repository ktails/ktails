package kinds

import "testing"

// TestHasPods guards the aggregate-log kind set: exactly the workload kinds
// that own Pods answer true, everything else (including Pods itself, which
// resolves directly rather than needing aggregation) answers false.
func TestHasPods(t *testing.T) {
	want := map[ResourceKind]bool{
		KindDeployments:              true,
		KindStatefulSets:             true,
		KindDaemonSets:               true,
		KindJobs:                     true,
		KindCronJobs:                 true,
		KindPods:                     false,
		KindServices:                 false,
		KindConfigMaps:               false,
		KindSecrets:                  false,
		KindIngresses:                false,
		KindPodDisruptionBudgets:     false,
		KindHorizontalPodAutoscalers: false,
		KindNodes:                    false,
	}
	for kind, expected := range want {
		if got := kind.HasPods(); got != expected {
			t.Errorf("%s.HasPods() = %v, want %v", kind.Title(), got, expected)
		}
	}
}
