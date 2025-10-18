// Package pages, it implements maing routing tp different pages.
package pages

import (
	"time"

	teakey "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/termkit/skeleton"
)

func NewMainModel(client *k8s.Client) tea.Model {
	colorPalette := styles.CatppuccinMocha()
	s := skeleton.NewSkeleton()
	s.AddPage("contexts", "Kubernetes Contexts", models.NewContextInfo(s, client))
	// s.AddPage("contexts", "Kubernetes Contexts", nil)
	s.AddPage("pod", "Pods", NewPodPageModel(s, client))
	s.SetActiveTabBorderColor(string(colorPalette.Maroon))
	s.SetBorderColor(string(colorPalette.Flamingo))

	// Add widgets
	s.AddWidget("time", time.Now().Format("15:04:05"))
	s.KeyMap.SetKeyNextTab(teakey.NewBinding(teakey.WithKeys(tea.KeyTab.String())))
	s.KeyMap.SetKeyPrevTab(teakey.NewBinding(teakey.WithKeys(tea.KeyShiftTab.String())))
	s.KeyMap.SetKeyQuit(teakey.NewBinding(teakey.WithKeys("q")))

	// Update system stats every second
	go func() {
		for {
			time.Sleep(time.Second)
			s.TriggerUpdate()
			s.UpdateWidgetValue("time", time.Now().Format("15:04:05"))
		}
	}()
	return s
}
