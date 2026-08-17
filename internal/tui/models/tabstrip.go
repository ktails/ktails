package models

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ktails/ktails/internal/tui/styles"
)

// RenderTabStrip renders a "Title1 · Title2 · Title3" strip, bolding the
// active tab and tinting it FocusColor when focused — shared by MainPage's
// top resource-kind tab strip and ResourceDetailPage's internal Status/
// Conditions/Events/YAML strip, so the two don't duplicate the same
// title-join-and-style logic.
func RenderTabStrip(titles []string, active int, focused bool) string {
	p := styles.CatppuccinMocha()
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(" · ")
	parts := make([]string, 0, len(titles))
	for i, title := range titles {
		st := lipgloss.NewStyle().Foreground(p.Overlay1)
		if i == active {
			st = st.Bold(true).Foreground(p.Subtext1)
			if focused {
				st = st.Foreground(styles.FocusColor)
			}
		}
		parts = append(parts, st.Render(title))
	}
	return strings.Join(parts, sep)
}
