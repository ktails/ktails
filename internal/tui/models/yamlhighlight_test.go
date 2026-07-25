package models

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightYAML_PreservesTextStripsToOriginal(t *testing.T) {
	raw := `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  labels:
    app: web
spec:
  replicas: 3
  paused: false
  containers:
    - name: app
      image: nginx:1.25
# a comment
---
`
	highlighted := highlightYAML(raw)

	stripped := ansi.Strip(highlighted)
	if stripped != raw {
		t.Fatalf("highlighting changed the text content:\nwant:\n%s\ngot:\n%s", raw, stripped)
	}

	if !strings.Contains(highlighted, "\x1b") {
		t.Fatal("expected ANSI color codes in highlighted YAML, found none")
	}
}

func TestHighlightYAML_EmptyInput(t *testing.T) {
	if got := highlightYAML(""); got != "" {
		t.Fatalf("expected empty string passthrough, got %q", got)
	}
}
