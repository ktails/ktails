package models

import "github.com/ktails/ktails/internal/tui/msgs"

// Helper functions (shared with deployment.go - consider moving to shared utils)
func rowsEqual(a, b []msgs.RowData) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}

		for k, v := range a[i] {
			if bv, ok := b[i][k]; !ok || bv != v {
				return false
			}
		}
	}

	return true
}

func cloneRows(rows []msgs.RowData) []msgs.RowData {
	if len(rows) == 0 {
		return make([]msgs.RowData, 0)
	}

	cloned := make([]msgs.RowData, len(rows))
	for i := range rows {
		if len(rows[i]) == 0 {
			cloned[i] = nil
			continue
		}

		cells := make(msgs.RowData, len(rows[i]))
		for k, v := range rows[i] {
			cells[k] = v
		}
		cloned[i] = cells
	}

	return cloned
}
