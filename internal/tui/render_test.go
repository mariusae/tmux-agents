package tui

import (
	"strings"
	"testing"
)

func TestRenderANSILinePreservesSGRAndPadsWidth(t *testing.T) {
	t.Parallel()

	line := "\x1b[31mred\x1b[0m text"
	rendered := renderANSILine(line, 10)

	if !strings.Contains(rendered, "\x1b[31m") {
		t.Fatal("expected SGR styling to be preserved")
	}
	if !strings.HasSuffix(rendered, "\x1b[0m") {
		t.Fatal("expected rendered line to end with reset")
	}

	visible := stripANSIVisible(rendered)
	if visible != "red text  " {
		t.Fatalf("visible output = %q, want %q", visible, "red text  ")
	}
}

func TestRenderANSILineExpandsTabsAndClips(t *testing.T) {
	t.Parallel()

	line := "a\tb"
	rendered := renderANSILine(line, 6)
	visible := stripANSIVisible(rendered)

	if visible != "a     " {
		t.Fatalf("visible output = %q, want %q", visible, "a     ")
	}
}

func stripANSIVisible(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\x1b' {
			_, next, ok := readANSISequence(text, i)
			if !ok {
				i++
				continue
			}
			i = next
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}
