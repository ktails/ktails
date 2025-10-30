package models

import "github.com/charmbracelet/bubbles/table"

// Helper functions (shared with deployment.go - consider moving to shared utils)
func rowsEqual(a, b []table.Row) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}

		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}

	return true
}

func cloneRows(rows []table.Row) []table.Row {
	if len(rows) == 0 {
		return make([]table.Row, 0)
	}

	cloned := make([]table.Row, len(rows))
	for i := range rows {
		if len(rows[i]) == 0 {
			cloned[i] = nil
			continue
		}

		cells := make(table.Row, len(rows[i]))
		copy(cells, rows[i])
		cloned[i] = cells
	}

	return cloned
}
