package tui

import (
	"time"

	"github.com/ivyascorp-net/ktails/internal/k8s"
)

// === Selection Messages ===

// ContextsLoadedMsg is sent when contexts are loaded from kubeconfig
type ContextsLoadedMsg struct {
	Contexts []string
	Current  string
	Err      error
}

// NamespacesLoadedMsg is sent when namespaces are loaded
type NamespacesLoadedMsg struct {
	Namespaces []string
	Context    string
	Err        error
}

// PodsLoadedMsg is sent when pods are loaded
type PodsLoadedMsg struct {
	Pods      []PodItem
	Context   string
	Namespace string
	Err       error
}

// PodSelectedMsg is sent when a pod is fully selected
type PodSelectedMsg struct {
	PaneIndex int
	Context   string
	Namespace string
	Pod       string
}

// NamespaceSelectedMsg is sent when a namespace is selected from the list
type NamespaceSelectedMsg struct {
	PaneIndex int
	Context   string
	Namespace string
}

// === Viewing Messages ===

// LogLineMsg is sent when a new log line arrives
type LogLineMsg struct {
	PaneIndex int
	Line      string
	Timestamp time.Time
}

// PodInfoMsg is sent when pod info is fetched
type PodInfoMsg struct {
	PaneIndex int
	Info      *k8s.PodInfo
	Err       error
}

// StreamStartedMsg is sent when log streaming begins
type StreamStartedMsg struct {
	PaneIndex int
}

// StreamEndedMsg is sent when log streaming ends
type StreamEndedMsg struct {
	PaneIndex int
	Err       error
}

// === Error Messages ===

// ErrorMsg represents a general error
type ErrorMsg struct {
	PaneIndex int
	Err       error
	Context   string // Additional context about the error
}

// === UI Messages ===

// FocusChangedMsg is sent when focus changes between panes
type FocusChangedMsg struct {
	NewFocusIndex int
}

// ModeChangedMsg is sent when the app mode changes
type ModeChangedMsg struct {
	NewMode Mode
}
