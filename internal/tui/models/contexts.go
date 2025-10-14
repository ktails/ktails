package models

import "github.com/charmbracelet/bubbles/list"

// Ensure ContextsInfo implements list.Item
var _ list.Item = ContextsInfo{}

type ContextsInfo struct {
	Name      string
	Cluster   string
	AuthInfo  string
	Namespace string
	IsCurrent bool
	Selected  bool
}

func (c ContextsInfo) Title() string {
	// Prefix with selection marker and current-context star
	sel := "[ ] "
	if c.Selected {
		sel = "[x] "
	}
	star := ""
	if c.IsCurrent {
		star = "* "
	}
	return sel + star + c.Name
}

func (c ContextsInfo) Description() string {
	// You can customize the description; keep minimal for now
	return c.Namespace
}

func (c ContextsInfo) FilterValue() string {
	return c.Name
}
