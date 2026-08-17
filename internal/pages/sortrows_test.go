package pages

import (
	"testing"
	"time"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func rowNamed(name, namespace string, createdAt time.Time) msgs.Row {
	return msgs.Row{Name: name, Namespace: namespace, CreatedAt: createdAt}
}

// TestSortRows_None is a true no-op, preserving input order.
func TestSortRows_None(t *testing.T) {
	rows := []msgs.Row{rowNamed("zeta", "ns", time.Time{}), rowNamed("alpha", "ns", time.Time{})}
	got := sortRows(rows, sortNone, sortAsc)
	if got[0].Name != "zeta" || got[1].Name != "alpha" {
		t.Fatalf("expected sortNone to preserve input order, got %+v", got)
	}
}

// TestSortRows_ByNameAscendingCaseInsensitive guards both the ordering and
// the case-insensitive compare (mixed-case resource names are common).
func TestSortRows_ByNameAscendingCaseInsensitive(t *testing.T) {
	rows := []msgs.Row{rowNamed("Zeta", "ns", time.Time{}), rowNamed("alpha", "ns", time.Time{})}
	got := sortRows(rows, sortByName, sortAsc)
	if got[0].Name != "alpha" || got[1].Name != "Zeta" {
		t.Fatalf("expected alpha before Zeta, got %+v", got)
	}
}

// TestSortRows_ByNameDescending guards direction reversal.
func TestSortRows_ByNameDescending(t *testing.T) {
	rows := []msgs.Row{rowNamed("alpha", "ns", time.Time{}), rowNamed("zeta", "ns", time.Time{})}
	got := sortRows(rows, sortByName, sortDesc)
	if got[0].Name != "zeta" || got[1].Name != "alpha" {
		t.Fatalf("expected zeta before alpha in descending order, got %+v", got)
	}
}

// TestSortRows_ByNamespace guards sorting on a different column than Name.
func TestSortRows_ByNamespace(t *testing.T) {
	rows := []msgs.Row{rowNamed("a", "zeta-ns", time.Time{}), rowNamed("b", "alpha-ns", time.Time{})}
	got := sortRows(rows, sortByNamespace, sortAsc)
	if got[0].Namespace != "alpha-ns" || got[1].Namespace != "zeta-ns" {
		t.Fatalf("expected alpha-ns before zeta-ns, got %+v", got)
	}
}

// TestSortRows_ByAgeUsesRawCreatedAtNotFormattedString guards that Age
// sorts chronologically by the raw CreatedAt timestamp — the displayed
// Age string ("3d", "2h") can't be ordered correctly as text.
func TestSortRows_ByAgeUsesRawCreatedAtNotFormattedString(t *testing.T) {
	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	rows := []msgs.Row{rowNamed("new", "ns", newer), rowNamed("old", "ns", older)}
	got := sortRows(rows, sortByAge, sortAsc)
	if got[0].Name != "old" || got[1].Name != "new" {
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
