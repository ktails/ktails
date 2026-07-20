package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	// Move the cursor down a few times, same as a user pressing the down key.
	for i := 0; i < 3; i++ {
		p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if row := p.SelectedRow(); row == nil || row[msgs.PodKeyName] != p.rows[3][msgs.PodKeyName] {
		t.Fatalf("expected cursor at row 3, got %v", row)
	}

	// A refresh with the same row count/content is a no-op per rowsEqual,
	// but a refresh with genuinely new data must still preserve the cursor.
	newRows := samplePodRows(10)
	newRows[0][msgs.PodKeyRestarts] = "5" // force rowsEqual to see a change
	p.SetRows(newRows)

	if row := p.SelectedRow(); row == nil || row[msgs.PodKeyName] != newRows[3][msgs.PodKeyName] {
		t.Fatalf("expected cursor preserved at row 3 after refresh, got %v", row)
	}
}

// TestPodPageLargeRowSetIsWindowed guards against the perf/clipping bug on
// real clusters with thousands of pods: WithNoPagination makes bubble-table
// render every row it's given on every frame (see VisibleIndices in
// bubble-table's pagination.go), so PodPage must only ever hand it a bounded
// window (see rowWindowSize in table.go) regardless of how many rows are
// loaded, and must keep the cursor tracking the right row as it scrolls past
// a window edge.
func TestPodPageLargeRowSetIsWindowed(t *testing.T) {
	p := NewPodPageModel(nil)
	p.SetSize(60, 20)
	p.SetFocused(true)
	rows := samplePodRows(2000)
	p.SetRows(rows)

	if got := len(p.table.GetVisibleRows()); got > rowWindowSize {
		t.Fatalf("expected bubble-table to hold at most %d rows, got %d", rowWindowSize, got)
	}

	// Walk the cursor past the first window's edge and confirm both the
	// selected row and the table's own highlighted row stay in sync with it.
	for i := 0; i < rowWindowSize; i++ {
		p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if row := p.SelectedRow(); row == nil || row[msgs.PodKeyName] != rows[rowWindowSize][msgs.PodKeyName] {
		t.Fatalf("expected cursor at row %d after scrolling past window edge, got %v", rowWindowSize, row)
	}
	if got := len(p.table.GetVisibleRows()); got > rowWindowSize {
		t.Fatalf("expected window to stay bounded at %d rows after scrolling, got %d", rowWindowSize, got)
	}
	wantHighlighted := p.cursorIdx - p.windowStart
	if got := p.table.GetHighlightedRowIndex(); got != wantHighlighted {
		t.Fatalf("expected table highlighted row %d, got %d", wantHighlighted, got)
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
