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

// YAML highlight styles, built once rather than per line (highlightYAML runs
// on every SetDetail, over every line of the rendered YAML). p is always
// styles.CatppuccinMocha() in practice — the p parameters threaded through
// highlightYAMLLine/highlightYAMLKeyValueOrScalar/highlightYAMLValue stay as
// documentation of what these are derived from and to keep the functions'
// signatures stable.
var (
	yamlCommentStyle = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay1).Faint(true)
	yamlDocSepStyle  = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay0)
	yamlDashStyle    = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay0)
	yamlKeyStyle     = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Sapphire)
	yamlColonStyle   = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Overlay0)
	yamlQuotedStyle  = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Green)
	yamlBoolStyle    = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Mauve)
	yamlNumberStyle  = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Peach)
	yamlPlainStyle   = lipgloss.NewStyle().Foreground(styles.CatppuccinMocha().Teal)
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
		return yamlCommentStyle.Render(line)
	}
	if trimmed == "---" || trimmed == "..." {
		return yamlDocSepStyle.Render(line)
	}

	if m := reYAMLListItem.FindStringSubmatch(line); m != nil {
		indent, gap, rest := m[1], m[2], m[3]
		dash := yamlDashStyle.Render("-")
		return indent + dash + gap + highlightYAMLKeyValueOrScalar(rest, p)
	}

	if m := reYAMLKeyValue.FindStringSubmatch(line); m != nil {
		indent, key, gap, value := m[1], m[2], m[3], m[4]
		keyStyled := yamlKeyStyle.Render(key)
		return indent + keyStyled + yamlColonStyle.Render(":") + gap + highlightYAMLValue(value, p)
	}

	// Plain continuation line — e.g. a multi-line block scalar's body.
	return line
}

// highlightYAMLKeyValueOrScalar highlights the text after a list item's
// dash, which is either a nested "key: value" pair or a bare scalar.
func highlightYAMLKeyValueOrScalar(rest string, p styles.Palette) string {
	if m := reYAMLKeyValue.FindStringSubmatch(rest); m != nil {
		key, gap, value := m[2], m[3], m[4]
		keyStyled := yamlKeyStyle.Render(key)
		return keyStyled + yamlColonStyle.Render(":") + gap + highlightYAMLValue(value, p)
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
		return yamlQuotedStyle.Render(value)
	case reYAMLBool.MatchString(value):
		return yamlBoolStyle.Render(value)
	case reYAMLNumber.MatchString(value):
		return yamlNumberStyle.Render(value)
	default:
		return yamlPlainStyle.Render(value)
	}
}
