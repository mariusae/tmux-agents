package tui

import "testing"

func TestParseWrappedColorResponseFromTmuxDCS(t *testing.T) {
	t.Parallel()

	kind, color, ok := parseWrappedColorResponse("tmux;\x1b\x1b]11;rgb:0000/0000/ffff\x1b\x1b\\")
	if !ok {
		t.Fatal("expected tmux-wrapped color response to parse")
	}
	if kind != "11" {
		t.Fatalf("kind = %q, want %q", kind, "11")
	}
	if color != (rgbColor{R: 0, G: 0, B: 255}) {
		t.Fatalf("color = %#v, want %#v", color, rgbColor{R: 0, G: 0, B: 255})
	}
}
