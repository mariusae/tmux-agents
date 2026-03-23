package tui

import (
	"testing"
	"time"
)

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

func TestParseCSIArrowDown(t *testing.T) {
	t.Parallel()

	term := newTestTerminal()
	term.rawBytes <- 'B'

	term.parseCSI()

	assertNextKeyEvent(t, term.events, keyDown)
}

func TestParseCSIArrowDownWithDoubleBracketPrefix(t *testing.T) {
	t.Parallel()

	term := newTestTerminal()
	term.rawBytes <- '['
	term.rawBytes <- 'B'

	term.parseCSI()

	assertNextKeyEvent(t, term.events, keyDown)
}

func TestParseSS3ArrowDown(t *testing.T) {
	t.Parallel()

	term := newTestTerminal()
	term.rawBytes <- 'B'

	term.parseSS3()

	assertNextKeyEvent(t, term.events, keyDown)
}

func newTestTerminal() *terminal {
	return &terminal{
		events:   make(chan event, 4),
		rawBytes: make(chan byte, 8),
		done:     make(chan struct{}),
	}
}

func assertNextKeyEvent(t *testing.T, events <-chan event, want keyCode) {
	t.Helper()

	select {
	case ev := <-events:
		if ev.typ != eventKey || ev.key != want {
			t.Fatalf("event = %#v, want key %v", ev, want)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for key %v", want)
	}
}
