package models

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

type Pods struct {
	table table.Model
}

func (p *Pods) Init() tea.Cmd {
	return nil
}

func (p *Pods) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return p, nil
}

func (p *Pods) View() string {
	return ""
}
