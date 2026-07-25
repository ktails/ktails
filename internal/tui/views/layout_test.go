package views

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSolveBudgetsAddUp(t *testing.T) {
	sizes := []struct{ w, h int }{
		{MinContentWidth, MinHeight},
		{100, 30},
		{143, 41}, // odd sizes exercise integer division
		{250, 60},
	}
	for _, size := range sizes {
		r := Solve(size.w, size.h)

		if r.LeftBoxW+r.RightBoxW != size.w {
			t.Errorf("Solve(%d,%d): boxes %d+%d don't span the width", size.w, size.h, r.LeftBoxW, r.RightBoxW)
		}
		if r.BoxH+FooterHeight != size.h {
			t.Errorf("Solve(%d,%d): box height %d + footer != height", size.w, size.h, r.BoxH)
		}
		if r.LeftBoxW > MaxLeftPaneWidth {
			t.Errorf("Solve(%d,%d): left box %d exceeds MaxLeftPaneWidth", size.w, size.h, r.LeftBoxW)
		}
		if r.LeftContentW != r.LeftBoxW-boxFrameW || r.RightContentW != r.RightBoxW-boxFrameW {
			t.Errorf("Solve(%d,%d): content widths don't match box minus frame", size.w, size.h)
		}
		if r.LeftContentH != r.BoxH-boxFrameH || r.RightContentH != r.BoxH-boxFrameH {
			t.Errorf("Solve(%d,%d): content heights don't match box minus frame", size.w, size.h)
		}
		if r.LeftContentW < 1 || r.RightContentW < 1 || r.LeftContentH < 1 {
			t.Errorf("Solve(%d,%d): degenerate content area %+v (at or above the minimum size)", size.w, size.h, r)
		}
	}
}

func TestFitBlockExactDimensions(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"short and narrow", "hi"},
		{"too many lines", strings.Repeat("line\n", 30)},
		{"too wide", strings.Repeat("x", 200)},
		{"empty", ""},
		{"ansi styled", lipgloss.NewStyle().Bold(true).Render("styled") + "\nplain"},
	}
	const w, h = 20, 5
	for _, tc := range cases {
		got := FitBlock(tc.content, w, h)
		lines := strings.Split(got, "\n")
		if len(lines) != h {
			t.Errorf("%s: expected %d lines, got %d", tc.name, h, len(lines))
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("%s: line %d is %d cells wide, want %d", tc.name, i, lw, w)
			}
		}
	}
}

func TestTitledBoxExactDimensions(t *testing.T) {
	borderColor := lipgloss.Color("#cba6f7")
	const w, h = 30, 8

	for _, title := range []string{"Contexts", "", strings.Repeat("very long title ", 5)} {
		box := TitledBox(title, "content\nlines", w, h, borderColor)
		lines := strings.Split(box, "\n")
		if len(lines) != h {
			t.Fatalf("title %q: expected %d rows, got %d", title, h, len(lines))
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("title %q: row %d is %d cells wide, want %d", title, i, lw, w)
			}
		}
	}
}

func TestTitledBoxEmbedsTitle(t *testing.T) {
	box := TitledBox("Contexts", "x", 30, 5, lipgloss.Color("#cba6f7"))
	topRow := strings.Split(box, "\n")[0]
	if !strings.Contains(topRow, "Contexts") {
		t.Fatalf("expected title embedded in top border, got %q", topRow)
	}
}

func TestFitsTitleAgreesWithTitledBoxFallback(t *testing.T) {
	titles := []string{"Contexts", "Contexts · Namespaces · Clusters", "", "X"}
	for _, title := range titles {
		for _, boxW := range []int{10, 22, 24, 30, 40, 60} {
			fits := FitsTitle(title, boxW)
			box := TitledBox(title, "x", boxW, 5, lipgloss.Color("#cba6f7"))
			topRow := strings.Split(box, "\n")[0]
			embedded := title != "" && strings.Contains(topRow, title)
			if fits != embedded {
				t.Errorf("FitsTitle(%q, %d) = %v, but TitledBox embedding = %v", title, boxW, fits, embedded)
			}
		}
	}
}
