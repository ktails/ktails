package models

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/ktails/ktails/internal/tui/styles"
)

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
		{Title: "✓", Width: 1}, // checkbox glyph for multi-pod log selection
		{Title: "Name", Width: 30},
		{Title: "Namespace", Width: 15},
		{Title: "Status", Width: 10},
		{Title: "Restarts", Width: 10},
		{Title: "Age", Width: 10},
		{Title: "Context", Width: 0},    // hidden, carries data for the detail tab
		{Title: "Containers", Width: 0}, // hidden, comma-separated container names for the log pane
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

// colorReplicaCell colors a "ready/desired" replica cell (as produced by
// LoadDeploymentInfoCmd): green when fully ready, yellow when partially
// ready, red when zero replicas are ready but some are desired.
func colorReplicaCell(cell string) string {
	ready, desired, ok := strings.Cut(cell, "/")
	if !ok {
		return cell
	}
	readyN, err := strconv.Atoi(ready)
	if err != nil {
		return cell
	}
	desiredN, err := strconv.Atoi(desired)
	if err != nil {
		return cell
	}

	p := styles.CatppuccinMocha()
	color := p.Red
	switch {
	case readyN == desiredN:
		color = p.Green
	case readyN > 0:
		color = p.Yellow
	}
	return lipgloss.NewStyle().Foreground(color).Render(cell)
}

func svcTableColumns() []table.Column {
	return []table.Column{
		{Title: "Name", Width: 25},
		{Title: "Namespace", Width: 15},
		{Title: "Type", Width: 12},
		{Title: "ClusterIP", Width: 15},
		{Title: "Ports", Width: 18},
		{Title: "Age", Width: 10},
		{Title: "Context", Width: 0}, // hidden, carries data for the detail tab
	}
}
