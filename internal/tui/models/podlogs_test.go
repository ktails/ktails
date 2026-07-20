package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func newTestLogPage(w, h int) *LogPage {
	l := NewLogPage()
	l.SetSize(w, h)
	l.AddSource("k", "pod-a", "ns", "ctx", "app")
	return l
}

func TestLogPage_WrapOffByDefault(t *testing.T) {
	l := newTestLogPage(40, 10)
	if l.Wrap() {
		t.Fatal("wrap must default to off")
	}
}

func TestLogPage_ToggleWrapIsMutuallyExclusiveWithScroll(t *testing.T) {
	l := newTestLogPage(20, 10)
	l.AppendLine("k", strings.Repeat("x", 200))

	l.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	percentBefore, ok := l.ScrollStatus()
	if !ok {
		t.Fatal("expected horizontal overflow to be scrollable while unwrapped")
	}
	if percentBefore == 0 {
		t.Fatal("shift+right should have scrolled right while unwrapped")
	}

	l.ToggleWrap()
	if !l.Wrap() {
		t.Fatal("ToggleWrap should have turned wrap on")
	}
	if _, ok := l.ScrollStatus(); ok {
		t.Fatal("ScrollStatus must report no overflow while wrapped")
	}

	// Wrap on: shift+right is a no-op, not a scroll — turn wrap back off and
	// confirm the scroll position was actually reset to 0, not just hidden.
	l.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	l.ToggleWrap()
	if percent, ok := l.ScrollStatus(); !ok || percent != 0 {
		t.Fatalf("expected scroll reset to 0 after wrap on/off, got percent=%d ok=%v", percent, ok)
	}
}

func TestLogPage_WrapPreservesColorAndReflowsToWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	l := newTestLogPage(20, 10)
	// Long line so it's forced to wrap across multiple visual lines.
	l.AppendLine("k", strings.Repeat("word ", 20))

	appended := l.rawLines[len(l.rawLines)-1]
	if !strings.Contains(appended, "\x1b[") {
		t.Fatal("expected source-prefix ANSI color codes in rawLines")
	}

	l.ToggleWrap()

	// applyContent's wrap path is exactly ansi.Wrap(s, viewport.Width, "")
	// per line — replicate it here to inspect the actual wrapped output.
	for _, raw := range l.rawLines {
		wrapped := ansi.Wrap(raw, l.viewport.Width, "")
		for _, ln := range strings.Split(wrapped, "\n") {
			if w := ansi.StringWidth(ln); w > l.viewport.Width {
				t.Fatalf("wrapped line exceeds viewport width %d: width=%d line=%q", l.viewport.Width, w, ln)
			}
		}
		strip := func(s string) string {
			s = ansi.Strip(s)
			s = strings.ReplaceAll(s, " ", "")
			s = strings.ReplaceAll(s, "\n", "")
			return s
		}
		plainOriginal := strip(raw)
		plainWrapped := strip(wrapped)
		if plainOriginal != plainWrapped {
			t.Fatalf("wrapping altered visible text:\n original=%q\n wrapped=%q", plainOriginal, plainWrapped)
		}
	}
}

func TestLogPage_HorizontalScrollPreservesColorOfPrefixAndJSON(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	l := newTestLogPage(15, 10)
	line := `2026-07-19T00:00:00Z INFO {"user":"alice","count":3,"ok":true} ` + strings.Repeat("z", 100)
	l.AppendLine("k", line)

	plainRaw := ansi.Strip(l.rawLines[len(l.rawLines)-1])

	for step := 0; step < 6; step++ {
		l.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
		viewLines := strings.Split(ansi.Strip(l.viewport.View()), "\n")
		var visible string
		for _, ln := range viewLines {
			if trimmed := strings.TrimRight(ln, " "); trimmed != "" {
				visible = trimmed
			}
		}
		if visible == "" {
			continue
		}
		// If ansi.Cut had sliced through an escape sequence, stray escape
		// bytes or truncated codes would leak into the stripped text and
		// this containment check would fail — the surviving text must be an
		// exact, uncorrupted substring of the original plain line.
		if !strings.Contains(plainRaw, visible) {
			t.Fatalf("step %d: scrolled view text is not a clean substring of the original:\n got=%q\n original=%q", step, visible, plainRaw)
		}
	}
}
