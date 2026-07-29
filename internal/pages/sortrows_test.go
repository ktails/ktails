package pages

import (
	"testing"
	"time"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func rowNamed(name, namespace string, createdAt time.Time) msgs.RowData {
	return msgs.RowData{
		msgs.KeyName:      name,
		msgs.KeyNamespace: namespace,
		msgs.KeyCreatedAt: createdAt,
	}
}

// TestSortRows_None is a true no-op, preserving input order.
func TestSortRows_None(t *testing.T) {
	rows := []msgs.RowData{rowNamed("zeta", "ns", time.Time{}), rowNamed("alpha", "ns", time.Time{})}
	got := sortRows(rows, sortNone, sortAsc)
	if got[0][msgs.KeyName] != "zeta" || got[1][msgs.KeyName] != "alpha" {
		t.Fatalf("expected sortNone to preserve input order, got %+v", got)
	}
}

// TestSortRows_ByNameAscendingCaseInsensitive guards both the ordering and
// the case-insensitive compare (mixed-case resource names are common).
func TestSortRows_ByNameAscendingCaseInsensitive(t *testing.T) {
	rows := []msgs.RowData{rowNamed("Zeta", "ns", time.Time{}), rowNamed("alpha", "ns", time.Time{})}
	got := sortRows(rows, sortByName, sortAsc)
	if got[0][msgs.KeyName] != "alpha" || got[1][msgs.KeyName] != "Zeta" {
		t.Fatalf("expected alpha before Zeta, got %+v", got)
	}
}

// TestSortRows_ByNameDescending guards direction reversal.
func TestSortRows_ByNameDescending(t *testing.T) {
	rows := []msgs.RowData{rowNamed("alpha", "ns", time.Time{}), rowNamed("zeta", "ns", time.Time{})}
	got := sortRows(rows, sortByName, sortDesc)
	if got[0][msgs.KeyName] != "zeta" || got[1][msgs.KeyName] != "alpha" {
		t.Fatalf("expected zeta before alpha in descending order, got %+v", got)
	}
}

// TestSortRows_ByNamespace guards sorting on a different column than Name.
func TestSortRows_ByNamespace(t *testing.T) {
	rows := []msgs.RowData{rowNamed("a", "zeta-ns", time.Time{}), rowNamed("b", "alpha-ns", time.Time{})}
	got := sortRows(rows, sortByNamespace, sortAsc)
	if got[0][msgs.KeyNamespace] != "alpha-ns" || got[1][msgs.KeyNamespace] != "zeta-ns" {
		t.Fatalf("expected alpha-ns before zeta-ns, got %+v", got)
	}
}

// TestSortRows_ByAgeUsesRawCreatedAtNotFormattedString guards that Age
// sorts chronologically by the raw KeyCreatedAt timestamp — the displayed
// Age string ("3d", "2h") can't be ordered correctly as text.
func TestSortRows_ByAgeUsesRawCreatedAtNotFormattedString(t *testing.T) {
	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	rows := []msgs.RowData{rowNamed("new", "ns", newer), rowNamed("old", "ns", older)}
	got := sortRows(rows, sortByAge, sortAsc)
	if got[0][msgs.KeyName] != "old" || got[1][msgs.KeyName] != "new" {
		t.Fatalf("expected oldest first in ascending Age order, got %+v", got)
	}
}

// TestCycleSort_TogglesThenClears guards the cycle: a new field starts
// ascending, a second press of the same field flips to descending, a third
// press clears back to sortNone.
func TestCycleSort_TogglesThenClears(t *testing.T) {
	m := &MainPage{tabs: nil}

	m.cycleSort(sortByName)
	if m.sortField != sortByName || m.sortDir != sortAsc {
		t.Fatalf("expected sortByName/sortAsc after first press, got %v/%v", m.sortField, m.sortDir)
	}

	m.cycleSort(sortByName)
	if m.sortField != sortByName || m.sortDir != sortDesc {
		t.Fatalf("expected sortByName/sortDesc after second press, got %v/%v", m.sortField, m.sortDir)
	}

	m.cycleSort(sortByName)
	if m.sortField != sortNone {
		t.Fatalf("expected sortNone after third press, got %v", m.sortField)
	}
}

// TestCycleSort_SwitchingFieldResetsToAscending guards that pressing a
// different column's key doesn't carry over the previous column's direction.
func TestCycleSort_SwitchingFieldResetsToAscending(t *testing.T) {
	m := &MainPage{tabs: nil}
	m.cycleSort(sortByName)
	m.cycleSort(sortByName) // now sortByName/sortDesc

	m.cycleSort(sortByAge)
	if m.sortField != sortByAge || m.sortDir != sortAsc {
		t.Fatalf("expected switching to sortByAge to reset direction to ascending, got %v/%v", m.sortField, m.sortDir)
	}
}
