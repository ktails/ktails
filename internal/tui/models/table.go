package models

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	btable "github.com/evertras/bubble-table/table"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

// newBubbleTable builds a bubble-table Model with the options common to
// every resource table: no pagination (the spec keeps today's continuous
// full-list scroll, not discrete pages), and no border — bubble-table draws
// a full grid border by default, which the old bubbles/table never had and
// which just duplicates the app's own pane border around the tab content.
func newBubbleTable(cols []btable.Column) btable.Model {
	return btable.New(cols).WithNoPagination().Border(btable.Border{})
}

// checkColWidth is the content width (before padding) of the Pods checkbox
// column glyph. The old bubbles/table implementation applied one shared
// Padding(0,1) cell style across every column including the checkbox, so it
// needs the same padding here (via paddedColumn) to avoid rendering flush
// against the Name column.
const checkColWidth = 1

// columnPadStyle mirrors the old bubbles/table cell look (Padding(0,1)).
// bubble-table only honors padding set directly on a Column's style (see
// cell.go), not on the table's base style, so every data column applies it
// individually.
func columnPadStyle() lipgloss.Style {
	return lipgloss.NewStyle().Padding(0, 1)
}

// paddedColumn builds a fixed-width wide-mode column sized to contentWidth
// plus the 2 columns of horizontal padding columnPadStyle adds.
func paddedColumn(key, title string, contentWidth int) btable.Column {
	return btable.NewColumn(key, title, contentWidth+2).WithStyle(columnPadStyle())
}

func paddedFlexColumn(key, title string, flexFactor int) btable.Column {
	return btable.NewFlexColumn(key, title, flexFactor).WithStyle(columnPadStyle())
}

// widestValue returns the widest string found under key across rows,
// falling back to the header's own width — used to auto-fit wide-mode
// column widths to whatever data is currently loaded (recomputed on every
// SetRows, per the wide-mode spec).
func widestValue(rows []msgs.RowData, key, header string) int {
	widest := lipgloss.Width(header)
	for _, row := range rows {
		s, ok := row[key].(string)
		if !ok {
			continue
		}
		if w := lipgloss.Width(s); w > widest {
			widest = w
		}
	}
	return widest
}

// totalColumnsWidth sums rendered column widths plus the border overhead
// bubble-table itself adds (one column of border per column, plus one),
// mirroring its own recalculateWidth so callers can tell whether a set of
// wide-mode columns will overflow a given viewport width.
func totalColumnsWidth(cols []btable.Column) int {
	total := 0
	for _, c := range cols {
		total += c.Width()
	}
	return total + len(cols) + 1
}

// statusColor maps a pod phase (PodInfo.Status) to its Catppuccin Mocha
// status color, per the Status Colors spec: Running=Green, Pending=Yellow,
// Failed/Unknown=Red, Succeeded=Overlay1 (dim). Unrecognized phases are
// left uncolored.
func statusColor(status string) (color.Color, bool) {
	p := styles.CatppuccinMocha()
	switch status {
	case "Running":
		return p.Green, true
	case "Pending":
		return p.Yellow, true
	case "Failed", "Unknown":
		return p.Red, true
	case "Succeeded":
		return p.Overlay1, true
	default:
		return nil, false
	}
}

// statusCellStyle is a btable.StyledCellFunc that colors the Status cell by
// phase (see statusColor). Cell style is applied by bubble-table at render
// time, after content-width truncation — the whole reason this migration
// dropped the old post-render ANSI-recoloring workaround.
func statusCellStyle(input btable.StyledCellFuncInput) lipgloss.Style {
	status, _ := input.Data.(string)
	if col, ok := statusColor(status); ok {
		return lipgloss.NewStyle().Foreground(col)
	}
	return lipgloss.NewStyle()
}

// replicaCellStyle is a btable.StyledCellFunc that colors a "ready/desired"
// replica cell (as produced by LoadDeploymentInfoCmd): green when fully
// ready, yellow when partially ready, red when zero replicas are ready but
// some are desired.
func replicaCellStyle(input btable.StyledCellFuncInput) lipgloss.Style {
	cell, _ := input.Data.(string)
	ready, desired, ok := strings.Cut(cell, "/")
	if !ok {
		return lipgloss.NewStyle()
	}
	readyN, err := strconv.Atoi(ready)
	if err != nil {
		return lipgloss.NewStyle()
	}
	desiredN, err := strconv.Atoi(desired)
	if err != nil {
		return lipgloss.NewStyle()
	}

	p := styles.CatppuccinMocha()
	color := p.Red
	switch {
	case readyN == desiredN:
		color = p.Green
	case readyN > 0:
		color = p.Yellow
	}
	return lipgloss.NewStyle().Foreground(color)
}

func podNarrowColumns() []btable.Column {
	return []btable.Column{
		paddedColumn(msgs.PodKeyCheck, "✓", checkColWidth),
		paddedFlexColumn(msgs.PodKeyName, "Name", 10),
		paddedFlexColumn(msgs.PodKeyNamespace, "Namespace", 5),
		paddedFlexColumn(msgs.PodKeyStatus, "Status", 4),
		paddedFlexColumn(msgs.PodKeyRestarts, "Restarts", 3),
		paddedFlexColumn(msgs.PodKeyAge, "Age", 3),
	}
}

func podWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		paddedColumn(msgs.PodKeyCheck, "✓", checkColWidth),
		paddedColumn(msgs.PodKeyName, "Name", widestValue(rows, msgs.PodKeyName, "Name")),
		paddedColumn(msgs.PodKeyNamespace, "Namespace", widestValue(rows, msgs.PodKeyNamespace, "Namespace")),
		paddedColumn(msgs.PodKeyStatus, "Status", widestValue(rows, msgs.PodKeyStatus, "Status")),
		paddedColumn(msgs.PodKeyReady, "Ready", widestValue(rows, msgs.PodKeyReady, "Ready")),
		paddedColumn(msgs.PodKeyRestarts, "Restarts", widestValue(rows, msgs.PodKeyRestarts, "Restarts")),
		paddedColumn(msgs.PodKeyAge, "Age", widestValue(rows, msgs.PodKeyAge, "Age")),
		paddedColumn(msgs.PodKeyNode, "Node", widestValue(rows, msgs.PodKeyNode, "Node")),
		paddedColumn(msgs.PodKeyNodeIP, "Node IP", widestValue(rows, msgs.PodKeyNodeIP, "Node IP")),
		paddedColumn(msgs.PodKeyPodIP, "Pod IP", widestValue(rows, msgs.PodKeyPodIP, "Pod IP")),
	}
}

func deploymentNarrowColumns() []btable.Column {
	return []btable.Column{
		paddedFlexColumn(msgs.DeployKeyName, "Name", 8),
		paddedFlexColumn(msgs.DeployKeyAge, "Age", 3),
		paddedFlexColumn(msgs.DeployKeyReplicas, "ReadyReplicas", 4),
		paddedFlexColumn(msgs.DeployKeyContext, "Context", 5),
	}
}

func deploymentWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		paddedColumn(msgs.DeployKeyName, "Name", widestValue(rows, msgs.DeployKeyName, "Name")),
		paddedColumn(msgs.DeployKeyAge, "Age", widestValue(rows, msgs.DeployKeyAge, "Age")),
		paddedColumn(msgs.DeployKeyReplicas, "ReadyReplicas", widestValue(rows, msgs.DeployKeyReplicas, "ReadyReplicas")),
		paddedColumn(msgs.DeployKeyAvailable, "Available", widestValue(rows, msgs.DeployKeyAvailable, "Available")),
		paddedColumn(msgs.DeployKeyUpdated, "Updated", widestValue(rows, msgs.DeployKeyUpdated, "Updated")),
		paddedColumn(msgs.DeployKeyStrategy, "Strategy", widestValue(rows, msgs.DeployKeyStrategy, "Strategy")),
		paddedColumn(msgs.DeployKeyContext, "Context", widestValue(rows, msgs.DeployKeyContext, "Context")),
		paddedColumn(msgs.DeployKeySelector, "Selector", widestValue(rows, msgs.DeployKeySelector, "Selector")),
	}
}

func svcNarrowColumns() []btable.Column {
	return []btable.Column{
		paddedFlexColumn(msgs.SvcKeyName, "Name", 28),
		paddedFlexColumn(msgs.SvcKeyNamespace, "Namespace", 18),
		paddedFlexColumn(msgs.SvcKeyType, "Type", 14),
		paddedFlexColumn(msgs.SvcKeyClusterIP, "ClusterIP", 18),
		paddedFlexColumn(msgs.SvcKeyPorts, "Ports", 12),
		paddedFlexColumn(msgs.SvcKeyAge, "Age", 10),
	}
}

func svcWideColumns(rows []msgs.RowData) []btable.Column {
	return []btable.Column{
		paddedColumn(msgs.SvcKeyName, "Name", widestValue(rows, msgs.SvcKeyName, "Name")),
		paddedColumn(msgs.SvcKeyNamespace, "Namespace", widestValue(rows, msgs.SvcKeyNamespace, "Namespace")),
		paddedColumn(msgs.SvcKeyType, "Type", widestValue(rows, msgs.SvcKeyType, "Type")),
		paddedColumn(msgs.SvcKeyClusterIP, "ClusterIP", widestValue(rows, msgs.SvcKeyClusterIP, "ClusterIP")),
		paddedColumn(msgs.SvcKeyPorts, "Ports", widestValue(rows, msgs.SvcKeyPorts, "Ports")),
		paddedColumn(msgs.SvcKeyAge, "Age", widestValue(rows, msgs.SvcKeyAge, "Age")),
		paddedColumn(msgs.SvcKeySelector, "Selector", widestValue(rows, msgs.SvcKeySelector, "Selector")),
		paddedColumn(msgs.SvcKeyExternalIP, "ExternalIP", widestValue(rows, msgs.SvcKeyExternalIP, "ExternalIP")),
		paddedColumn(msgs.SvcKeyEndpointIPs, "EndpointIPs", widestValue(rows, msgs.SvcKeyEndpointIPs, "EndpointIPs")),
	}
}
