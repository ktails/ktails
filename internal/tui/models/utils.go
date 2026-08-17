package models

import (
	"slices"

	"github.com/ktails/ktails/internal/tui/msgs"
)

// Helper functions (shared with deployment.go - consider moving to shared utils)
func rowsEqual(a, b []msgs.Row) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Name != b[i].Name || a[i].Namespace != b[i].Namespace || a[i].Context != b[i].Context || a[i].CreatedAt != b[i].CreatedAt {
			return false
		}
		if !slices.Equal(a[i].Containers, b[i].Containers) {
			return false
		}
		if len(a[i].Cells) != len(b[i].Cells) {
			return false
		}
		for k, v := range a[i].Cells {
			if bv, ok := b[i].Cells[k]; !ok || bv != v {
				return false
			}
		}
	}

	return true
}

// halfViewportStep is the horizontal-scroll step size shared by the Detail
// and Log panes' Shift+Left/Right handling: half the viewport's width, so a
// press reaches far-right content in a couple of steps regardless of
// terminal size.
func halfViewportStep(viewportWidth int) int {
	step := viewportWidth / 2
	if step < 1 {
		step = 1
	}
	return step
}
