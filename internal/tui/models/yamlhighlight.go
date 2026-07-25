package models

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ktails/ktails/internal/tui/styles"
)

// reYAMLKeyValue matches a "key: value" line (or the tail of a list item
// after its dash), splitting into indent, key, the whitespace after the
// colon, and the value. The key character class excludes ':', so the match
// stops at the first colon even when the value itself contains one (e.g. an
// RFC3339 timestamp).
var reYAMLKeyValue = regexp.MustCompile(`^(\s*)([A-Za-z0-9_.\-/]+):(\s*)(.*)$`)

// reYAMLListItem matches a "- value" or "- key: value" list item line.
var reYAMLListItem = regexp.MustCompile(`^(\s*)-(\s+)(.*)$`)

var (
	// RE2 (Go's regexp) has no backreferences, so quoted strings need one
	// alternative per quote style rather than `^(['"]).*\1$`.
	reYAMLQuoted = regexp.MustCompile(`^(".*"|'.*')$`)
	reYAMLBool   = regexp.MustCompile(`^(true|false|null|~)$`)
	reYAMLNumber = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
)

// highlightYAML colorizes rendered YAML for the Detail Pane: mapping keys,
// list dashes, comments, and value literals (strings/numbers/booleans) each
// get their own color, on a best-effort line-by-line basis rather than a
// full YAML parse — good enough for a read-only terminal view, and cheap
// enough to run on every SetDetail.
func highlightYAML(raw string) string {
	if raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = highlightYAMLLine(line)
	}
	return strings.Join(lines, "\n")
}

func highlightYAMLLine(line string) string {
	p := styles.CatppuccinMocha()
	trimmed := strings.TrimSpace(line)

	if trimmed == "" {
		return line
	}
	if strings.HasPrefix(trimmed, "#") {
		return lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render(line)
	}
	if trimmed == "---" || trimmed == "..." {
		return lipgloss.NewStyle().Foreground(p.Overlay0).Render(line)
	}

	if m := reYAMLListItem.FindStringSubmatch(line); m != nil {
		indent, gap, rest := m[1], m[2], m[3]
		dash := lipgloss.NewStyle().Foreground(p.Overlay0).Render("-")
		return indent + dash + gap + highlightYAMLKeyValueOrScalar(rest, p)
	}

	if m := reYAMLKeyValue.FindStringSubmatch(line); m != nil {
		indent, key, gap, value := m[1], m[2], m[3], m[4]
		keyStyled := lipgloss.NewStyle().Foreground(p.Sapphire).Render(key)
		return indent + keyStyled + lipgloss.NewStyle().Foreground(p.Overlay0).Render(":") + gap + highlightYAMLValue(value, p)
	}

	// Plain continuation line — e.g. a multi-line block scalar's body.
	return line
}

// highlightYAMLKeyValueOrScalar highlights the text after a list item's
// dash, which is either a nested "key: value" pair or a bare scalar.
func highlightYAMLKeyValueOrScalar(rest string, p styles.Palette) string {
	if m := reYAMLKeyValue.FindStringSubmatch(rest); m != nil {
		key, gap, value := m[2], m[3], m[4]
		keyStyled := lipgloss.NewStyle().Foreground(p.Sapphire).Render(key)
		return keyStyled + lipgloss.NewStyle().Foreground(p.Overlay0).Render(":") + gap + highlightYAMLValue(value, p)
	}
	return highlightYAMLValue(rest, p)
}

// highlightYAMLValue colors a scalar value by its apparent type: quoted
// strings, booleans/null, numbers, or plain text. An empty value (the key
// introduces a nested map/list on following lines) is returned unchanged.
func highlightYAMLValue(value string, p styles.Palette) string {
	switch {
	case value == "":
		return value
	case reYAMLQuoted.MatchString(value):
		return lipgloss.NewStyle().Foreground(p.Green).Render(value)
	case reYAMLBool.MatchString(value):
		return lipgloss.NewStyle().Foreground(p.Mauve).Render(value)
	case reYAMLNumber.MatchString(value):
		return lipgloss.NewStyle().Foreground(p.Peach).Render(value)
	default:
		return lipgloss.NewStyle().Foreground(p.Teal).Render(value)
	}
}
