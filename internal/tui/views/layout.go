package views

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Rects is the solved layout budget for one terminal size: every pane's
// exact box and content rectangle. The layout is two titled boxes side by
// side over a one-line status bar; content areas are what's inside a box's
// border plus one column of horizontal padding per side.
type Rects struct {
	// BoxH is the height of both boxes (terminal height minus the status bar).
	BoxH int
	// LeftBoxW / RightBoxW are the two boxes' total widths, summing to the
	// terminal width.
	LeftBoxW  int
	RightBoxW int
	// *ContentW/*ContentH are the usable areas inside each box.
	LeftContentW  int
	LeftContentH  int
	RightContentW int
	RightContentH int
}

// boxFrameW is the horizontal overhead of a titled box: two border columns
// plus one column of padding per side.
const boxFrameW = 4

// boxFrameH is the vertical overhead of a titled box: the top and bottom
// border rows.
const boxFrameH = 2

// Solve computes the whole layout from the terminal size. It is the single
// place the height/width budget lives: every pane renders into exactly the
// rectangle solved here, so the two boxes' borders always land on the same
// rows by construction.
func Solve(w, h int) Rects {
	boxH := h - FooterHeight

	// The Context List's content (context names, a namespace/cluster line
	// per entry) never needs a full quarter of the window — a narrower
	// fixed-ish column reads more like lazygit's sidebar and leaves more
	// room for the Tab Area's table columns.
	leftW := w / 5
	if leftW > MaxLeftPaneWidth {
		leftW = MaxLeftPaneWidth
	} else if leftW < MinLeftPaneWidth {
		leftW = MinLeftPaneWidth
	}
	rightW := w - leftW

	return Rects{
		BoxH:          boxH,
		LeftBoxW:      leftW,
		RightBoxW:     rightW,
		LeftContentW:  leftW - boxFrameW,
		LeftContentH:  boxH - boxFrameH,
		RightContentW: rightW - boxFrameW,
		RightContentH: boxH - boxFrameH,
	}
}

// FitBlock forces content into exactly w columns by h rows: each line is
// right-padded or ANSI-aware truncated to w, missing lines are added, extra
// lines are dropped. Rendering into a pre-fitted block is what lets
// TitledBox draw its border around content without trusting any style's
// frame math.
func FitBlock(content string, w, h int) string {
	if w < 1 || h < 1 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	fitted := make([]string, h)
	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		switch pad := w - lipgloss.Width(line); {
		case pad > 0:
			line += strings.Repeat(" ", pad)
		case pad < 0:
			line = ansi.Truncate(line, w, "")
		}
		fitted[i] = line
	}
	return strings.Join(fitted, "\n")
}

// FitsTitle reports whether a title (already styled) fits in a box's top
// border alongside its "╭─ " / " ─…─╮" decoration — the same check
// TitledBox itself uses before silently falling back to a bare rule.
// Callers with more than one candidate title (e.g. a full tab strip vs. just
// the active tab's name) use this to pick the better one, rather than
// letting a too-wide title vanish entirely.
func FitsTitle(title string, boxW int) bool {
	span := boxW - 2
	return lipgloss.Width(title) > 0 && lipgloss.Width(title)+3 <= span
}

// TitledBox draws a single-line rounded border of exactly w×h cells around
// content, with title (already styled by the caller) embedded in the top
// border — the "╭─ Title ────╮" look. Content is fitted to the box's
// content area first, so the output is always exactly w columns by h rows.
func TitledBox(title, content string, w, h int, borderColor color.Color) string {
	if w < boxFrameW || h < boxFrameH {
		return FitBlock(content, w, h)
	}
	bs := lipgloss.NewStyle().Foreground(borderColor)

	// Top border: ╭─ Title ────╮ — falls back to a plain rule when the
	// title doesn't fit.
	span := w - 2 // cells between the corners
	titleW := lipgloss.Width(title)
	var top string
	if titleW > 0 && titleW+3 <= span {
		top = bs.Render("╭─ ") + title + bs.Render(" "+strings.Repeat("─", span-titleW-3)+"╮")
	} else {
		top = bs.Render("╭" + strings.Repeat("─", span) + "╮")
	}

	left := bs.Render("│ ")
	right := bs.Render(" │")
	body := strings.Split(FitBlock(content, w-boxFrameW, h-boxFrameH), "\n")
	rows := make([]string, 0, h)
	rows = append(rows, top)
	for _, line := range body {
		rows = append(rows, left+line+right)
	}
	rows = append(rows, bs.Render("╰"+strings.Repeat("─", span)+"╯"))
	return strings.Join(rows, "\n")
}
