package models

import "github.com/charmbracelet/bubbles/table"

// func contextTableColumns() []table.Column {
// 	return []table.Column{
// 		{Title: "Context Name", Width: 20},
// 		{Title: "Current", Width: 10},
// 		{Title: "Cluster", Width: 15},
// 		{Title: "Auth Info", Width: 25},
// 		{Title: "Namespace", Width: 15},
// 		{Title: "Extensions", Width: 15},
// 		{Title: "Cluster Endpoint", Width: 25},
// 	}
// }

func podTableColumns() []table.Column {
	return []table.Column{
		{Title: "Name", Width: 30},
		{Title: "Namespace", Width: 15},
		{Title: "Status", Width: 10},
		{Title: "Restarts", Width: 10},
		{Title: "Age", Width: 10},
		{Title: "Context", Width: 0}, // hidden, carries data for the detail tab
	}
}

func deploymentTableColumns() []table.Column {
	return []table.Column{
		{Title: "Name", Width: 30},
		{Title: "Age", Width: 10},
		{Title: "ReadyReplicas", Width: 15},
		{Title: "Contexts", Width: 12},
		{Title: "Namespace", Width: 0}, // hidden, carries data for the detail panel
	}
}
