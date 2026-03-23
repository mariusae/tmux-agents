package tui

import "testing"

func TestColorQuerySequenceIsPlainOSC(t *testing.T) {
	got := colorQuerySequence("11")
	want := "\x1b]11;?\x1b\\"
	if got != want {
		t.Fatalf("colorQuerySequence() = %q, want %q", got, want)
	}
}

func TestParseTmuxStyle(t *testing.T) {
	t.Parallel()

	fg, bg := parseTmuxStyle("bg=#262a33,fg=#ffffff")
	if fg == nil || *fg != (rgbColor{R: 255, G: 255, B: 255}) {
		t.Fatalf("fg = %#v, want white", fg)
	}
	if bg == nil || *bg != (rgbColor{R: 0x26, G: 0x2a, B: 0x33}) {
		t.Fatalf("bg = %#v, want %#v", bg, rgbColor{R: 0x26, G: 0x2a, B: 0x33})
	}
}

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
