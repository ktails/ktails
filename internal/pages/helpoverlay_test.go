package pages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestHelpOverlay_KeyLabelsFitColumn guards the regression this fix
// targets: a key label wider than helpKeyColWidth breaks column alignment
// for every row below it (lipgloss.Style.Width pads short content but
// doesn't truncate long content, so an over-long label just renders wider
// than the column, pushing the description out of alignment).
func TestHelpOverlay_KeyLabelsFitColumn(t *testing.T) {
	for _, b := range helpBindings {
		if w := ansi.StringWidth(b.key); w > helpKeyColWidth {
			t.Errorf("key %q is %d cols wide, exceeds helpKeyColWidth=%d", b.key, w, helpKeyColWidth)
		}
	}
}

// TestHelpOverlay_NeverExceedsTerminalWidth guards the box-width cap: no
// rendered line may be wider than the terminal, across both the
// single-column (narrow) and two-column (wide) layouts.
func TestHelpOverlay_NeverExceedsTerminalWidth(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{80, 24},  // the app's hard floor
		{100, 24}, // the two-column threshold
		{125, 33}, // the original layout's stated requirement
		{200, 50},
	} {
		m := &MainPage{width: size.w, height: size.h}
		out := m.renderHelpOverlay()
		for i, line := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(line); w > size.w {
				t.Errorf("%dx%d: line %d is %d cols wide, exceeds terminal width %d: %q", size.w, size.h, i, w, size.w, line)
			}
		}
	}
}

// TestHelpOverlay_WideTerminalFitsWithoutOverflow guards that a terminal at
// or above the original ~125x33 requirement the help overlay used to need
// now renders within its own height budget — the two-column fallback
// halving the row count is what makes this possible.
func TestHelpOverlay_WideTerminalFitsWithoutOverflow(t *testing.T) {
	m := &MainPage{width: 150, height: 40}
	out := m.renderHelpOverlay()
	if lines := len(strings.Split(out, "\n")); lines > m.height {
		t.Errorf("expected the help overlay to fit within a generous 150x40 terminal, got %d lines for height %d", lines, m.height)
	}
}
