package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ktails/ktails/internal/k8s"
)

func wideResourceDetail(name string) k8s.ResourceDetail {
	return k8s.ResourceDetail{
		Kind:      "Deployment",
		Name:      name,
		Namespace: "ns",
		Age:       "1d",
		Summary:   "Ready Replicas: 2",
		Status:    []string{"Warning  SomeReason  " + strings.Repeat("x", 200)},
		Events: []k8s.EventInfo{
			{Age: "1m", Reason: "Scheduled", Type: "Warning", Message: strings.Repeat("y", 200), Count: 1},
		},
		YAML: strings.Repeat("z", 200) + "\n" + strings.Repeat("w", 200),
	}
}

func TestResourceDetailHorizontalScroll(t *testing.T) {
	d := NewResourceDetailPage()
	d.SetSize(40, 10)
	d.StartLoading("Deployment", "foo", "ctx")
	d.SetDetail(wideResourceDetail("foo"))

	if _, ok := d.HScrollStatus(); !ok {
		t.Fatalf("expected overflow indicator to be visible for wide content")
	}

	// shift+right should scroll by ~half the viewport width per press.
	d.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if d.hOffset != 20 {
		t.Fatalf("expected hOffset 20 after one shift+right on a 40-wide viewport, got %d", d.hOffset)
	}

	pct1, ok := d.HScrollStatus()
	if !ok || pct1 <= 0 {
		t.Fatalf("expected positive scroll percentage after scrolling right, got %d (ok=%v)", pct1, ok)
	}

	// Repeated presses must eventually reach (and clamp at) the far right,
	// in a small, terminal-size-independent number of presses — the whole
	// point of scaling the step to half the viewport width.
	for i := 0; i < 10; i++ {
		d.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	}
	pctMax, ok := d.HScrollStatus()
	if !ok || pctMax != 100 {
		t.Fatalf("expected to reach 100%% scroll, got %d (ok=%v)", pctMax, ok)
	}

	d.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	if d.hOffset >= 200 {
		t.Fatalf("expected shift+left to move the offset back left, got %d", d.hOffset)
	}
}

func TestResourceDetailScrollResetsOnNewResource(t *testing.T) {
	d := NewResourceDetailPage()
	d.SetSize(40, 10)
	d.StartLoading("Deployment", "foo", "ctx")
	d.SetDetail(wideResourceDetail("foo"))
	d.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if d.hOffset == 0 {
		t.Fatalf("expected non-zero offset after scrolling")
	}

	// Same resource reload (e.g. re-pressing Enter, or Ctrl+R-style refresh)
	// must preserve the offset.
	d.StartLoading("Deployment", "foo", "ctx")
	d.SetDetail(wideResourceDetail("foo"))
	if d.hOffset == 0 {
		t.Fatalf("expected offset to survive a same-resource reload")
	}

	// A different resource must reset it.
	d.StartLoading("Deployment", "bar", "ctx")
	if d.hOffset != 0 {
		t.Fatalf("expected offset to reset when a different resource is opened, got %d", d.hOffset)
	}
	d.SetDetail(wideResourceDetail("bar"))
	if d.hOffset != 0 {
		t.Fatalf("expected offset to remain 0 after loading a different resource, got %d", d.hOffset)
	}
}

func TestResourceDetailScrollResetsOnResize(t *testing.T) {
	d := NewResourceDetailPage()
	d.SetSize(40, 10)
	d.StartLoading("Deployment", "foo", "ctx")
	d.SetDetail(wideResourceDetail("foo"))
	d.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if d.hOffset == 0 {
		t.Fatalf("expected non-zero offset after scrolling")
	}

	// Same width re-applied (e.g. re-focusing the pane) must NOT reset.
	d.SetSize(40, 12)
	if d.hOffset == 0 {
		t.Fatalf("expected offset to survive a same-width SetSize call")
	}

	// A genuine width change (terminal resize) must reset.
	d.SetSize(60, 12)
	if d.hOffset != 0 {
		t.Fatalf("expected offset to reset on resize, got %d", d.hOffset)
	}
}

func TestResourceDetailNoOverflowHidesIndicator(t *testing.T) {
	d := NewResourceDetailPage()
	d.SetSize(200, 10)
	d.StartLoading("Deployment", "foo", "ctx")
	d.SetDetail(k8s.ResourceDetail{
		Kind: "Deployment", Name: "foo", Namespace: "ns", Age: "1d",
		Summary: "Ready Replicas: 2", YAML: "short",
	})
	if _, ok := d.HScrollStatus(); ok {
		t.Fatalf("expected indicator to be hidden when content fits within the viewport")
	}
}
