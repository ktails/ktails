// Package models
package models

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ktails/ktails/internal/tui/styles"
)

// maxLogLines bounds the in-memory scrollback per source, dropping each
// source's own oldest lines once exceeded — a noisy container can't evict a
// quiet one's history.
const maxLogLines = 500

// sourceColors is the rotation of Catppuccin Mocha accents used to color
// each source's line prefix. Red/Mauve/Green/Peach are excluded: they
// already carry other meaning elsewhere in the UI (errors, focus/selection,
// loaded state, this pane's own title).
func sourceColors() []color.Color {
	p := styles.CatppuccinMocha()
	return []color.Color{
		p.Blue, p.Lavender, p.Sapphire, p.Sky, p.Teal,
		p.Pink, p.Flamingo, p.Rosewater, p.Yellow, p.Maroon,
	}
}

// highlightJSONLine finds the first JSON object or array embedded in a log
// line (e.g. after a "2026-07-19 INFO " prefix) and, if the text from there
// to the end of the line parses as valid JSON, colors its tokens in place.
// The line is otherwise returned unchanged: no pretty-printing, no
// reformatting, and lines without a parseable JSON tail are left as plain
// text. This uses a separate color palette from sourceColors so a JSON
// payload never collides with its own line's source-prefix color.
func highlightJSONLine(line string, p styles.Palette) string {
	idx := strings.IndexAny(line, "{[")
	if idx < 0 {
		return line
	}
	prefix, jsonPart := line[:idx], line[idx:]
	if !json.Valid([]byte(jsonPart)) {
		return line
	}
	colored, ok := colorizeJSON(jsonPart, p)
	if !ok {
		return line
	}
	return prefix + colored
}

// splitLeadingGap separates a leading run of separator bytes (whitespace,
// ':', ',') that json.Decoder attaches to the front of a scalar token's raw
// span from the token's own text, based on where that token's value is
// known to actually begin.
func splitLeadingGap(raw string, tok interface{}) (gap, content string) {
	var cut int
	switch tok.(type) {
	case json.Delim:
		return "", raw
	case string:
		cut = strings.IndexByte(raw, '"')
	case json.Number:
		cut = strings.IndexAny(raw, "-0123456789")
	case bool:
		cut = strings.IndexAny(raw, "tf")
	case nil:
		cut = strings.IndexByte(raw, 'n')
	}
	if cut < 0 {
		cut = 0
	}
	return raw[:cut], raw[cut:]
}

// colorizeJSON walks s (already known to be valid JSON) token by token via
// json.Decoder, coloring each token's original raw text in place and
// leaving everything between tokens (whitespace, colons, commas) dimmed as
// punctuation. Token-based scanning, rather than unmarshaling into a map,
// preserves the source's original key order and formatting exactly.
func colorizeJSON(s string, p styles.Palette) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()

	type frame struct {
		isObj     bool
		expectKey bool
	}
	var stack []frame

	punctStyle := lipgloss.NewStyle().Foreground(p.Overlay1)
	keyStyle := lipgloss.NewStyle().Foreground(p.Blue)
	strStyle := lipgloss.NewStyle().Foreground(p.Green)
	numStyle := lipgloss.NewStyle().Foreground(p.Yellow)
	litStyle := lipgloss.NewStyle().Foreground(p.Mauve)

	var sb strings.Builder
	var prevEnd int64

	for {
		startOff := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		endOff := dec.InputOffset()

		if gap := s[prevEnd:startOff]; gap != "" {
			sb.WriteString(punctStyle.Render(gap))
		}
		raw := s[startOff:endOff]

		// The decoder's offsets lag by one token: any separator between the
		// previous token and this one (a colon, comma, or whitespace) shows
		// up as a leading prefix of raw rather than its own gap, so it must
		// be split off here and colored as punctuation to avoid bleeding
		// into this token's value color.
		lead, content := splitLeadingGap(raw, tok)
		if lead != "" {
			sb.WriteString(punctStyle.Render(lead))
		}
		raw = content

		switch v := tok.(type) {
		case json.Delim:
			sb.WriteString(punctStyle.Render(raw))
			switch v {
			case '{':
				stack = append(stack, frame{isObj: true, expectKey: true})
			case '[':
				stack = append(stack, frame{isObj: false})
			case '}', ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].isObj && stack[len(stack)-1].expectKey {
				sb.WriteString(keyStyle.Render(raw))
				stack[len(stack)-1].expectKey = false
			} else {
				sb.WriteString(strStyle.Render(raw))
				if len(stack) > 0 && stack[len(stack)-1].isObj {
					stack[len(stack)-1].expectKey = true
				}
			}
		case json.Number:
			sb.WriteString(numStyle.Render(raw))
			if len(stack) > 0 && stack[len(stack)-1].isObj {
				stack[len(stack)-1].expectKey = true
			}
		case bool, nil:
			sb.WriteString(litStyle.Render(raw))
			if len(stack) > 0 && stack[len(stack)-1].isObj {
				stack[len(stack)-1].expectKey = true
			}
		}
		prevEnd = endOff
	}
	sb.WriteString(s[prevEnd:])

	return sb.String(), true
}

// logLine is a single buffered line tagged with a global arrival sequence
// number, so lines from different sources can be interleaved back into
// true chronological order when rendering the merged view.
type logLine struct {
	seq  int64
	text string
}

// logSource is one pod/container being tailed into the merged pane. It
// holds only its own ring-buffered scrollback — stream I/O orchestration
// lives in cmds + MainPage, mirroring how ResourceDetailPage stays free of
// k8s/io concerns.
type logSource struct {
	key       string
	podName   string
	namespace string
	context   string
	container string
	color     color.Color

	lines     []logLine
	streaming bool
	streamErr string
}

func (s *logSource) label() string {
	return fmt.Sprintf("%s/%s", s.podName, s.container)
}

// LogPage renders a live-tailing merged log viewport for one or more
// pod/container sources, in the shared bottom Log pane. It holds only
// render state — opening/reading the underlying streams is orchestrated by
// cmds + MainPage.
type LogPage struct {
	viewport viewport.Model

	sources map[string]*logSource
	order   []string // insertion order: stable color assignment + isolate-cycle order
	nextSeq int64

	// isolatedIdx selects a single source (by index into order) to render
	// alone; -1 means the full merged view. Other sources keep streaming
	// into their buffers in the background regardless of isolation state.
	isolatedIdx int

	focused bool

	// wrap toggles soft-wrap of rawLines. Off by default (no behavior change
	// for existing users on their first log view), and mutually exclusive
	// with horizontal scroll: wrapping reflows lines to fit the viewport, so
	// there's nothing left to scroll to. This is a single in-memory flag on
	// this one long-lived LogPage instance — it persists across different
	// pods/log views for the process's lifetime, but is never written to
	// disk (no config-file I/O exists in this codebase yet).
	wrap bool

	// rawLines is the last computed set of already-colored, unwrapped lines
	// (source-prefixed and JSON-highlighted) backing the viewport's content.
	// Kept around so toggling wrap, or resizing, can re-derive the viewport
	// content without re-running the merge/highlight pipeline. maxLineWidth
	// is its longest line's on-screen width, cached alongside it so the
	// horizontal-scroll status indicator doesn't rescan every render.
	rawLines     []string
	maxLineWidth int
}

func NewLogPage() *LogPage {
	return &LogPage{
		viewport:    viewport.New(),
		sources:     make(map[string]*logSource),
		isolatedIdx: -1,
	}
}

func (l *LogPage) Init() tea.Cmd {
	return nil
}

// HasContent reports whether any source has ever been added to this pane.
func (l *LogPage) HasContent() bool {
	return len(l.order) > 0
}

// HasSource reports whether the given source key is currently open.
func (l *LogPage) HasSource(key string) bool {
	_, ok := l.sources[key]
	return ok
}

// Keys returns the currently open source keys, in insertion order.
func (l *LogPage) Keys() []string {
	keys := make([]string, len(l.order))
	copy(keys, l.order)
	return keys
}

// AddSource opens a new source in the pane, idempotently (a no-op if the
// key is already present). Assigns the next color in the rotation.
func (l *LogPage) AddSource(key, podName, namespace, context, container string) {
	if _, exists := l.sources[key]; exists {
		return
	}
	colors := sourceColors()
	color := colors[len(l.order)%len(colors)]

	src := &logSource{
		key:       key,
		podName:   podName,
		namespace: namespace,
		context:   context,
		container: container,
		color:     color,
		streaming: true,
	}
	l.sources[key] = src
	l.order = append(l.order, key)
	l.appendTo(src, fmt.Sprintf("Connecting to %s...", src.label()))
}

// RemoveSource closes and forgets a source. Isolation resets to the full
// merged view if the isolated source (or its index) no longer applies,
// keeping isolatedIdx simple rather than tracking it through reordering.
func (l *LogPage) RemoveSource(key string) {
	if _, exists := l.sources[key]; !exists {
		return
	}
	delete(l.sources, key)
	for i, k := range l.order {
		if k == key {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
	l.isolatedIdx = -1
	l.refreshContent()
}

// Clear removes every source, resetting the pane to empty.
func (l *LogPage) Clear() {
	l.sources = make(map[string]*logSource)
	l.order = nil
	l.isolatedIdx = -1
	l.viewport.SetContent("")
}

// CycleIsolation advances through -1 (full merge) -> 0 -> ... ->
// len(order)-1 -> -1, changing only what's rendered. Every source keeps
// streaming into its own buffer regardless of isolation state, so
// returning to the full merge shows no gaps.
func (l *LogPage) CycleIsolation() {
	if len(l.order) == 0 {
		l.isolatedIdx = -1
		return
	}
	l.isolatedIdx++
	if l.isolatedIdx >= len(l.order) {
		l.isolatedIdx = -1
	}
	l.refreshContent()
}

// IsolatedLabel returns the currently isolated source's "pod/container"
// label, or "" when showing the full merge.
func (l *LogPage) IsolatedLabel() string {
	if l.isolatedIdx < 0 || l.isolatedIdx >= len(l.order) {
		return ""
	}
	return l.sources[l.order[l.isolatedIdx]].label()
}

// AppendLine adds a line read from source key's live stream. The viewport
// only auto-follows to the bottom if it was already there.
func (l *LogPage) AppendLine(key, line string) {
	src, ok := l.sources[key]
	if !ok {
		return
	}
	l.appendTo(src, line)
}

func (l *LogPage) appendTo(src *logSource, text string) {
	wasAtBottom := l.viewport.AtBottom()

	l.nextSeq++
	src.lines = append(src.lines, logLine{seq: l.nextSeq, text: text})
	if len(src.lines) > maxLogLines {
		src.lines = src.lines[len(src.lines)-maxLogLines:]
	}

	l.refreshContent()

	if wasAtBottom {
		l.viewport.GotoBottom()
	}
}

// SetStreamEnded records that source key's stream stopped (server-closed or
// errored) and appends an inline banner line for just that source,
// preserving scrollback rather than replacing the pane. There's no
// auto-retry — the source simply stops.
func (l *LogPage) SetStreamEnded(key string, err error) {
	src, ok := l.sources[key]
	if !ok {
		return
	}
	src.streaming = false
	if err != nil {
		src.streamErr = err.Error()
	} else {
		src.streamErr = "stream ended"
	}

	p := styles.CatppuccinMocha()
	banner := lipgloss.NewStyle().Foreground(p.Red).
		Render(fmt.Sprintf("! log stream ended for %s: %s", src.label(), src.streamErr))
	l.appendTo(src, banner)
}

// refreshContent rebuilds rawLines from either the isolated source or a
// chronological merge of every source's buffer (interleaved by global
// arrival sequence — each source's own lines are already seq-ordered, so
// this is a straightforward merge-and-sort over a bounded number of lines),
// then hands the result to applyContent to become the viewport's content.
func (l *LogPage) refreshContent() {
	p := styles.CatppuccinMocha()

	var rendered []string
	if l.isolatedIdx >= 0 && l.isolatedIdx < len(l.order) {
		src := l.sources[l.order[l.isolatedIdx]]
		rendered = make([]string, len(src.lines))
		for i, ln := range src.lines {
			rendered[i] = highlightJSONLine(ln.text, p)
		}
	} else {
		var all []logLine
		for _, key := range l.order {
			src := l.sources[key]
			prefix := lipgloss.NewStyle().Foreground(src.color).Bold(true).Render(src.label() + " |")
			for _, ln := range src.lines {
				all = append(all, logLine{seq: ln.seq, text: prefix + " " + highlightJSONLine(ln.text, p)})
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })

		rendered = make([]string, len(all))
		for i, ln := range all {
			rendered[i] = ln.text
		}
	}

	l.rawLines = rendered
	l.maxLineWidth = 0
	for _, s := range rendered {
		if w := ansi.StringWidth(s); w > l.maxLineWidth {
			l.maxLineWidth = w
		}
	}
	l.applyContent()
}

// applyContent renders rawLines into the viewport for the current wrap
// state. Both paths reflow/crop the already-colored line text rather than
// re-deriving colors, so the per-source-prefix and JSON-highlighting colors
// survive intact: unwrapped, the lines are handed to the viewport as-is and
// its own xOffset-based cropping (github.com/charmbracelet/x/ansi.Cut under
// the hood) handles horizontal scroll ANSI-aware at render time; wrapped,
// ansi.Wrap reflows each line to the viewport width, preserving ANSI escape
// sequences and only ever splitting on grapheme boundaries.
func (l *LogPage) applyContent() {
	if !l.wrap || l.viewport.Width() < 1 {
		l.viewport.SetContent(strings.Join(l.rawLines, "\n"))
		return
	}

	wrapped := make([]string, len(l.rawLines))
	for i, s := range l.rawLines {
		wrapped[i] = ansi.Wrap(s, l.viewport.Width(), "")
	}
	l.viewport.SetContent(strings.Join(wrapped, "\n"))
}

// ToggleWrap flips soft-wrap on/off. Wrap and horizontal scroll are
// mutually exclusive, so turning wrap on resets the scroll position back to
// the left edge — wrapped lines reflow to fit, leaving nothing to scroll to.
func (l *LogPage) ToggleWrap() {
	l.wrap = !l.wrap
	if l.wrap {
		l.viewport.SetXOffset(0)
	}
	l.applyContent()
}

// Wrap reports whether soft-wrap is currently enabled.
func (l *LogPage) Wrap() bool {
	return l.wrap
}

// ScrollStatus reports the current horizontal scroll position as a
// percentage, for the status bar's "< N% >" indicator. ok is false while
// wrapped (nothing to scroll) or when no line overflows the viewport width
// (nothing to scroll to).
func (l *LogPage) ScrollStatus() (percent int, ok bool) {
	if l.wrap || l.maxLineWidth <= l.viewport.Width() {
		return 0, false
	}
	return int(l.viewport.HorizontalScrollPercent() * 100), true
}

// Header renders a one-line banner summarizing the merged sources (or the
// isolated one) and the pane's key hints.
func (l *LogPage) Header(width int) string {
	p := styles.CatppuccinMocha()
	title := lipgloss.NewStyle().Foreground(p.Peach).Bold(true)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true)

	label := "Logs"
	switch {
	case l.IsolatedLabel() != "":
		label = fmt.Sprintf("Isolated: %s  [%d/%d]", l.IsolatedLabel(), l.isolatedIdx+1, len(l.order))
	case len(l.order) > 0:
		label = fmt.Sprintf("Merged: %d source(s)", len(l.order))
	}
	if l.wrap {
		label += "  [wrap]"
	}

	full := title.Render(fmt.Sprintf("v %s", label)) + "  " +
		hint.Render("(c: isolate/merge, w: wrap, up/down pgup/pgdn scroll, shift+left/right: pan, End: jump+follow, Esc back)")
	if width <= 0 {
		return full
	}
	return ansi.Truncate(full, width, "...")
}

func (l *LogPage) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "home", "g":
			l.viewport.GotoTop()
			return nil
		case "end", "G":
			l.viewport.GotoBottom()
			return nil
		case "shift+left":
			if !l.wrap {
				l.viewport.ScrollLeft(halfViewportStep(l.viewport.Width()))
			}
			return nil
		case "shift+right":
			if !l.wrap {
				l.viewport.ScrollRight(halfViewportStep(l.viewport.Width()))
			}
			return nil
		}
	}

	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return cmd
}

// halfViewportStep is the Shift+Left/Shift+Right scroll distance: half the
// viewport's current width, so it adapts to terminal size.
// SetSize resizes the viewport. Horizontal scroll resets to the left edge
// on an actual size change (there's nothing meaningful to preserve once the
// overflow that produced it has changed shape); wrapped content is re-flowed
// to the new width so it keeps fitting exactly.
func (l *LogPage) SetSize(w, h int) {
	if w < 10 || h < 1 {
		return
	}
	resized := w != l.viewport.Width() || h != l.viewport.Height()
	l.viewport.SetWidth(w)
	l.viewport.SetHeight(h)
	if resized {
		l.viewport.SetXOffset(0)
		if l.wrap {
			l.applyContent()
		}
	}
}

func (l *LogPage) SetFocused(f bool) {
	l.focused = f
}

func (l *LogPage) View() string {
	p := styles.CatppuccinMocha()
	if !l.HasContent() {
		return lipgloss.NewStyle().Foreground(p.Overlay1).Render("No logs loaded")
	}
	return l.viewport.View()
}
