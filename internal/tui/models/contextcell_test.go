package models

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
)

// ansiFg/ansiBg return the truecolor foreground/background escape sequence
// lipgloss would emit for c, so a test can check whether that exact color
// appears (or doesn't) in a rendered view.
func ansiFg(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func ansiBg(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// TestContextCellDotAlwaysCarriesIdentityColor is a regression test for the
// "cursor selection with context-name color is painful on the eye" report.
// Coloring the whole Context cell (dot + short code) the same identity hue
// clashed badly once that hue sat on top of the highlighted row's Blue
// focus-accent background. The fix keeps the identity colour on the dot
// glyph only, unconditionally — no row-highlight state needed at all — so
// this checks the dot's colour shows up the same way whether or not its row
// is the one under the cursor.
func TestContextCellDotAlwaysCarriesIdentityColor(t *testing.T) {
	identity := lipgloss.Color("#f5c2e7") // Pink, an identity rotation color
	p := NewResourceTable(msgs.KindDeployments)
	p.SetFocused(true)
	p.SetSize(80, 20)
	p.SetContextColors(map[string]color.Color{"ctx-a": identity})
	p.SetRows([]msgs.RowData{
		{msgs.DeployKeyName: "aaa", msgs.DeployKeyNamespace: "ns", msgs.DeployKeyContext: "ctx-a", msgs.DeployKeyAge: "1d", msgs.DeployKeyReplicas: "1/1"},
		{msgs.DeployKeyName: "bbb", msgs.DeployKeyNamespace: "ns", msgs.DeployKeyContext: "ctx-a", msgs.DeployKeyAge: "1d", msgs.DeployKeyReplicas: "1/1"},
	})

	identityCode := ansiFg(identity)
	focusBg := ansiBg(styles.FocusColor)
	view := p.View()
	lines := strings.Split(view, "\n")
	var highlightedLine, plainLine string
	for _, l := range lines {
		if strings.Contains(l, "aaa") {
			highlightedLine = l // row 0 is the cursor row by default
		}
		if strings.Contains(l, "bbb") {
			plainLine = l
		}
	}
	if highlightedLine == "" || plainLine == "" {
		t.Fatalf("couldn't find both rows in rendered view:\n%s", view)
	}

	if !strings.Contains(plainLine, identityCode) {
		t.Errorf("non-highlighted row's dot should carry its identity colour:\n%s", plainLine)
	}
	if !strings.Contains(highlightedLine, identityCode) {
		t.Errorf("highlighted row's dot should still carry its identity colour:\n%s", highlightedLine)
	}
	if !strings.Contains(highlightedLine, focusBg) {
		t.Errorf("highlighted row's background should survive past the dot's own foreground reset:\n%q", highlightedLine)
	}
}
