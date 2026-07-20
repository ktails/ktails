package models

import (
	"strings"
	"testing"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func samplePodRows(n int) []msgs.RowData {
	rows := make([]msgs.RowData, n)
	statuses := []string{"Running", "Pending", "Failed", "Succeeded", "Unknown"}
	for i := 0; i < n; i++ {
		rows[i] = msgs.RowData{
			msgs.PodKeyName:       "pod-with-a-fairly-long-name-" + strings.Repeat("x", i%5),
			msgs.PodKeyNamespace:  "ns",
			msgs.PodKeyStatus:     statuses[i%len(statuses)],
			msgs.PodKeyRestarts:   "0",
			msgs.PodKeyAge:        "1d",
			msgs.PodKeyContext:    "ctx-a",
			msgs.PodKeyContainers: "app,sidecar",
		}
	}
	return rows
}

func TestPodPageNarrowNoTruncationArtifacts(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(30, 20) // narrow terminal
	p.SetFocused(true)
	p.SetRows(samplePodRows(5))

	view := p.View()
	if strings.Contains(view, "�") {
		t.Fatalf("view contains replacement glyph (truncation artifact):\n%s", view)
	}
	if !strings.Contains(view, "\x1b") {
		t.Fatalf("expected ANSI color codes for Status cells, found none:\n%s", view)
	}
}

func TestPodPageWideModeAutoFitNoTruncation(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(40, 20)
	p.SetRows(samplePodRows(5))
	p.ToggleWideMode()
	if !p.WideMode() {
		t.Fatalf("expected wide mode on")
	}

	view := p.View()
	if strings.Contains(view, "…") {
		t.Fatalf("wide mode should never truncate, found ellipsis:\n%s", view)
	}
}

func TestPodPageCursorPreservedAcrossRefresh(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(60, 20)
	p.SetFocused(true)
	p.SetRows(samplePodRows(10))

	// Move highlighted row down a couple of times via the underlying table.
	p.table = p.table.WithHighlightedRow(3)
	if idx := p.table.GetHighlightedRowIndex(); idx != 3 {
		t.Fatalf("expected cursor at 3, got %d", idx)
	}

	// A refresh with the same row count/content is a no-op per rowsEqual,
	// but a refresh with genuinely new data must still preserve the cursor.
	newRows := samplePodRows(10)
	newRows[0][msgs.PodKeyRestarts] = "5" // force rowsEqual to see a change
	p.SetRows(newRows)

	if idx := p.table.GetHighlightedRowIndex(); idx != 3 {
		t.Fatalf("expected cursor preserved at 3 after refresh, got %d", idx)
	}
}

func TestPodPageScrollPersistsAcrossRefreshResetsOnResize(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(30, 20)
	p.SetRows(samplePodRows(5))
	p.ToggleWideMode()
	p.ScrollRight()
	offsetBefore, _, ok := p.ScrollStatus()
	if !ok {
		t.Skip("not enough columns to overflow at this width; scroll indicator not applicable")
	}

	newRows := samplePodRows(5)
	newRows[0][msgs.PodKeyRestarts] = "9"
	p.SetRows(newRows)
	offsetAfter, _, _ := p.ScrollStatus()
	if offsetAfter != offsetBefore {
		t.Fatalf("expected scroll offset to survive refresh: before=%d after=%d", offsetBefore, offsetAfter)
	}

	p.SetSize(31, 20)
	if p.WideMode() {
		t.Fatalf("expected wide mode reset to narrow on resize")
	}
	if _, _, ok := p.ScrollStatus(); ok {
		t.Fatalf("expected scroll indicator hidden after resize")
	}
}

func TestPodRowKeyAndCheckToggle(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(60, 20)
	rows := samplePodRows(3)
	p.SetRows(rows)

	key := PodRowKey(rows[0])
	if key == "" {
		t.Fatalf("expected non-empty row key")
	}
	p.ToggleChecked(key)
	if !p.IsChecked(key) {
		t.Fatalf("expected row to be checked")
	}
	if got := p.CheckedRow(key); got == nil {
		t.Fatalf("expected CheckedRow to find the row")
	}
	p.ClearChecked()
	if p.IsChecked(key) {
		t.Fatalf("expected checks cleared")
	}
}

func TestDeploymentReplicaColoringViaStyledCell(t *testing.T) {
	d := NewDeploymentPage(nil)
	d.SetSize(40, 20)
	d.SetRows([]msgs.RowData{
		{
			msgs.DeployKeyName:      "dep-a",
			msgs.DeployKeyAge:       "2d",
			msgs.DeployKeyReplicas:  "1/3",
			msgs.DeployKeyContext:   "ctx-a",
			msgs.DeployKeyNamespace: "ns",
		},
	})
	view := d.View()
	if strings.Contains(view, "�") {
		t.Fatalf("view contains truncation artifact:\n%s", view)
	}
	if !strings.Contains(view, "\x1b") {
		t.Fatalf("expected the ready/desired cell to carry ANSI color from StyledCell")
	}
}
