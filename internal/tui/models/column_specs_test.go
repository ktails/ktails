package models

import (
	"testing"

	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// TestColumnSpecs_CompactDropsExactlyOneAgeColumn guards the §8.3
// column-priority contract every kind's *NarrowSpec relies on:
// dropWhenCompact must mark exactly one entry (Age, by convention), so
// TierCompact drops exactly one column and nothing else.
func TestColumnSpecs_CompactDropsExactlyOneAgeColumn(t *testing.T) {
	for _, kind := range msgs.Kinds() {
		spec := specFor(kind)
		full := spec.narrowColumns(false)
		compact := spec.narrowColumns(true)

		if len(full)-len(compact) != 1 {
			t.Errorf("%v: expected compact mode to drop exactly 1 column, dropped %d (full=%d compact=%d)",
				kind, len(full)-len(compact), len(full), len(compact))
			continue
		}

		// The dropped column must be Age — diff the key sets rather than
		// assuming position, since Age isn't always last (see colEntry's
		// doc comment).
		compactKeys := make(map[string]bool, len(compact))
		for _, c := range compact {
			compactKeys[c.Key()] = true
		}
		var dropped string
		for _, c := range full {
			if !compactKeys[c.Key()] {
				dropped = c.Title()
			}
		}
		if dropped != "Age" {
			t.Errorf("%v: expected the dropped compact-mode column to be Age, got %q", kind, dropped)
		}
	}
}

// TestColumnSpecs_NoDuplicateKeysPerSet guards against a copy-paste error
// in one of the declarative *NarrowSpec/*WideSpec tables (e.g. reusing a
// sibling kind's key by accident) — bubble-table would silently render two
// columns backed by the same row field, or crash the row-mapping lookup,
// either way not something the existing render-behavior tests would catch.
func TestColumnSpecs_NoDuplicateKeysPerSet(t *testing.T) {
	for _, kind := range msgs.Kinds() {
		spec := specFor(kind)
		for _, mode := range []struct {
			name string
			cols []btable.Column
		}{
			{"narrow", spec.narrowColumns(false)},
			{"wide", spec.wideColumns(nil)},
		} {
			seen := make(map[string]bool, len(mode.cols))
			for _, c := range mode.cols {
				if seen[c.Key()] {
					t.Errorf("%v %s: duplicate column key %q", kind, mode.name, c.Key())
				}
				seen[c.Key()] = true
			}
		}
	}
}

// TestColumnSpecs_EveryKindHasAContextColumn guards the shared column
// order every *NarrowSpec/*WideSpec is documented to follow (§3.4): every
// kind's Context column must be present, fixed-width (not flexed), and
// exactly contextColWidth+2 wide (paddedColumn adds columnPadStyle's 2
// columns of horizontal padding on top of contextColWidth).
func TestColumnSpecs_EveryKindHasAContextColumn(t *testing.T) {
	for _, kind := range msgs.Kinds() {
		spec := specFor(kind)
		for _, mode := range []struct {
			name string
			cols []btable.Column
		}{
			{"narrow", spec.narrowColumns(false)},
			{"wide", spec.wideColumns(nil)},
		} {
			found := false
			for _, c := range mode.cols {
				if c.Title() != "Context" {
					continue
				}
				found = true
				if c.IsFlex() {
					t.Errorf("%v %s: Context column must not be a flex column", kind, mode.name)
				}
				if c.Width() != contextColWidth+2 {
					t.Errorf("%v %s: Context column width = %d, want %d", kind, mode.name, c.Width(), contextColWidth+2)
				}
			}
			if !found {
				t.Errorf("%v %s: missing a Context column", kind, mode.name)
			}
		}
	}
}
