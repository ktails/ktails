package models

import (
	"strings"
	"testing"

	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/views"
)

// TestCompactTierDropsAgeColumn checks the §8.3 column-priority rule: Age
// is the lowest-priority fixed narrow-mode column and the first (only, in
// this implementation) one dropped under TierCompact, while Context stays
// visible in every tier.
func TestCompactTierDropsAgeColumn(t *testing.T) {
	p := NewResourceTable(msgs.KindPods)
	p.SetTier(views.TierCompact)
	p.SetSize(60, 20)
	p.SetRows(samplePodRows(3))

	view := p.View()
	if strings.Contains(view, "Age") {
		t.Fatalf("TierCompact should drop the Age column:\n%s", view)
	}
	if !strings.Contains(view, "Context") {
		t.Fatalf("TierCompact should still show the Context column:\n%s", view)
	}

	p.SetTier(views.TierStandard)
	p.SetSize(90, 20) // wider than the TierCompact case above: Pods now has 2 more flex columns (CPU/Memory), so 60 cols isn't enough room for every column's header to render un-truncated
	view = p.View()
	if !strings.Contains(view, "Age") {
		t.Fatalf("TierStandard should keep the Age column:\n%s", view)
	}
}

// TestSetTierNoOpDoesNotInvalidateView guards against SetTier churning the
// cached view (see ResourceTable.viewDirty) when the tier hasn't actually
// changed — called on every resize, so a same-tier resize shouldn't force a
// pointless rebuild.
func TestSetTierNoOpDoesNotInvalidateView(t *testing.T) {
	p := NewResourceTable(msgs.KindPods)
	p.SetTier(views.TierStandard) // establish a known tier (zero value is TierCompact)
	p.SetSize(60, 20)
	p.SetRows(samplePodRows(3))
	_ = p.View() // populate the cache

	p.SetTier(views.TierStandard) // same tier again: should be a no-op
	if p.viewDirty {
		t.Fatalf("SetTier with an unchanged tier should not invalidate the cached view")
	}
}
