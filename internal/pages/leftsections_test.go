package pages

import "testing"

func TestSplitLeftSections_SumsExactlyToTotal(t *testing.T) {
	// views.MinHeight (24) yields a Context List content height around 20;
	// also cover much taller terminals and the theoretical floor.
	for _, total := range []int{3, 4, 6, 10, 18, 21, 50, 200} {
		contextsH, namespacesH, clustersH := splitLeftSections(total)

		got := contextsH + namespacesH + clustersH + leftSections
		want := total
		if total < leftSections*(minLeftSectionHeight+1) {
			// Below this floor, splitLeftSections clamps remaining upward
			// rather than degenerating to zero/negative sections, so the
			// sum can exceed a pathologically small total.
			want = leftSections*minLeftSectionHeight + leftSections
		}
		if got != want {
			t.Errorf("total=%d: contextsH=%d namespacesH=%d clustersH=%d (+%d headers) = %d, want %d",
				total, contextsH, namespacesH, clustersH, leftSections, got, want)
		}

		for name, h := range map[string]int{"contexts": contextsH, "namespaces": namespacesH, "clusters": clustersH} {
			if h < minLeftSectionHeight {
				t.Errorf("total=%d: %s section height %d below minimum %d", total, name, h, minLeftSectionHeight)
			}
		}
	}
}

func TestSplitLeftSections_ContextsGetsTheMostRoom(t *testing.T) {
	contextsH, namespacesH, clustersH := splitLeftSections(50)
	if contextsH <= namespacesH || contextsH <= clustersH {
		t.Fatalf("expected Contexts to get the largest share, got contexts=%d namespaces=%d clusters=%d",
			contextsH, namespacesH, clustersH)
	}
}
