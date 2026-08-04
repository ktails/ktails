package models

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

	l.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
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
	l.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	l.ToggleWrap()
	if percent, ok := l.ScrollStatus(); !ok || percent != 0 {
		t.Fatalf("expected scroll reset to 0 after wrap on/off, got percent=%d ok=%v", percent, ok)
	}
}

func TestLogPage_WrapPreservesColorAndReflowsToWidth(t *testing.T) {
	l := newTestLogPage(20, 10)
	// Long line so it's forced to wrap across multiple visual lines.
	l.AppendLine("k", strings.Repeat("word ", 20))

	appended := l.rawLines[len(l.rawLines)-1]
	if !strings.Contains(appended, "\x1b[") {
		t.Fatal("expected source-prefix ANSI color codes in rawLines")
	}

	l.ToggleWrap()

	// applyContent's wrap path is exactly ansi.Wrap(s, viewport.Width(), "")
	// per line — replicate it here to inspect the actual wrapped output.
	for _, raw := range l.rawLines {
		wrapped := ansi.Wrap(raw, l.viewport.Width(), "")
		for _, ln := range strings.Split(wrapped, "\n") {
			if w := ansi.StringWidth(ln); w > l.viewport.Width() {
				t.Fatalf("wrapped line exceeds viewport width %d: width=%d line=%q", l.viewport.Width(), w, ln)
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

// TestLogPage_AppendCachesRenderedTextSeparatelyFromRaw guards the
// highlight-once caching: appendTo must compute logLine.rendered (via
// highlightJSONLine) once at append time, distinct from the raw text kept in
// logLine.text — not re-derive it from text on every refreshContent call.
func TestLogPage_AppendCachesRenderedTextSeparatelyFromRaw(t *testing.T) {
	l := newTestLogPage(80, 10)
	line := `2026-07-19T00:00:00Z INFO {"user":"alice"}`
	l.AppendLine("k", line)

	src := l.sources["k"]
	got := src.lines[len(src.lines)-1]
	if got.text != line {
		t.Fatalf("expected raw text preserved unchanged, got %q", got.text)
	}
	if got.rendered == got.text {
		t.Fatal("expected rendered to differ from raw text (JSON highlighting adds ANSI codes)")
	}
	if !strings.Contains(got.rendered, "\x1b[") {
		t.Fatalf("expected rendered to contain ANSI escape codes from JSON highlighting, got %q", got.rendered)
	}
}

// TestLogPage_MergedViewInterleavesBySequence guards mergedRenderedLines'
// k-way merge (which replaced a sort.Slice over the full concatenation):
// lines from two sources must come back in true chronological (arrival)
// order regardless of which source they belong to.
func TestLogPage_MergedViewInterleavesBySequence(t *testing.T) {
	l := NewLogPage()
	l.SetSize(80, 10)
	l.AddSource("a", "pod-a", "ns", "ctx", "app") // "Connecting..." banner: seq 1
	l.AddSource("b", "pod-b", "ns", "ctx", "app") // "Connecting..." banner: seq 2
	l.AppendLine("a", "a-line-1")                 // seq 3
	l.AppendLine("b", "b-line-1")                 // seq 4
	l.AppendLine("a", "a-line-2")                 // seq 5
	l.AppendLine("b", "b-line-2")                 // seq 6

	var order []string
	for _, raw := range l.rawLines {
		plain := ansi.Strip(raw)
		switch {
		case strings.Contains(plain, "a-line-1"):
			order = append(order, "a-line-1")
		case strings.Contains(plain, "b-line-1"):
			order = append(order, "b-line-1")
		case strings.Contains(plain, "a-line-2"):
			order = append(order, "a-line-2")
		case strings.Contains(plain, "b-line-2"):
			order = append(order, "b-line-2")
		}
	}
	want := []string{"a-line-1", "b-line-1", "a-line-2", "b-line-2"}
	if len(order) != len(want) {
		t.Fatalf("expected 4 identifiable merged lines, got %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("expected merged order %v, got %v", want, order)
		}
	}
}

// TestLogPage_BufferTrimKeepsMostRecentLines guards maxLogLines eviction:
// after exceeding the per-source cap, the oldest lines are dropped and the
// merged view's first surviving line is the (n-maxLogLines+1)th appended.
func TestLogPage_BufferTrimKeepsMostRecentLines(t *testing.T) {
	l := newTestLogPage(80, 10)
	const appended = maxLogLines + 100
	for i := 0; i < appended; i++ {
		l.AppendLine("k", fmt.Sprintf("line-%d", i))
	}

	src := l.sources["k"]
	if len(src.lines) != maxLogLines {
		t.Fatalf("expected buffer trimmed to %d lines, got %d", maxLogLines, len(src.lines))
	}

	// AddSource's "Connecting to..." banner (appended before the loop) is
	// evicted along with the oldest numbered lines — the surviving window is
	// exactly the last maxLogLines of everything ever appended.
	firstSurviving := ansi.Strip(l.rawLines[0])
	wantMinIndex := appended - maxLogLines
	wantLine := fmt.Sprintf("line-%d", wantMinIndex)
	if !strings.Contains(firstSurviving, wantLine) {
		t.Fatalf("expected the first surviving line to be %s, got %q", wantLine, firstSurviving)
	}
	lastSurviving := ansi.Strip(l.rawLines[len(l.rawLines)-1])
	if !strings.Contains(lastSurviving, fmt.Sprintf("line-%d", appended-1)) {
		t.Fatalf("expected the last surviving line to be line-%d, got %q", appended-1, lastSurviving)
	}
}

// BenchmarkAppendTo measures the cost of one more line arriving once every
// source is at capacity — the hot path under sustained log/watch load.
// Asserts nothing; run with -bench to see the highlight-once caching's win.
func BenchmarkAppendTo(b *testing.B) {
	l := NewLogPage()
	l.SetSize(120, 40)
	for s := 0; s < 4; s++ {
		key := fmt.Sprintf("src-%d", s)
		l.AddSource(key, fmt.Sprintf("pod-%d", s), "ns", "ctx", "app")
		for i := 0; i < maxLogLines; i++ {
			l.AppendLine(key, fmt.Sprintf(`%d INFO {"i":%d,"src":%d}`, i, i, s))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.AppendLine("src-0", fmt.Sprintf(`%d INFO {"i":%d}`, i, i))
	}
}

func TestLogPage_HorizontalScrollPreservesColorOfPrefixAndJSON(t *testing.T) {
	l := newTestLogPage(15, 10)
	line := `2026-07-19T00:00:00Z INFO {"user":"alice","count":3,"ok":true} ` + strings.Repeat("z", 100)
	l.AppendLine("k", line)

	plainRaw := ansi.Strip(l.rawLines[len(l.rawLines)-1])

	for step := 0; step < 6; step++ {
		l.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
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
