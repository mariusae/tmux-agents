package tui

import "testing"

func TestParseQueryResponse(t *testing.T) {
	t.Parallel()

	kind, color, ok := parseQueryResponse("11;rgb:1a1a/2b2b/3c3c")
	if !ok {
		t.Fatal("expected query response to parse")
	}
	if kind != "11" {
		t.Fatalf("kind = %q, want %q", kind, "11")
	}
	if color != (rgbColor{R: 0x1a, G: 0x2b, B: 0x3c}) {
		t.Fatalf("color = %#v, want %#v", color, rgbColor{R: 0x1a, G: 0x2b, B: 0x3c})
	}
}

func TestParseQueryResponseRGBAAnd16BitChannels(t *testing.T) {
	t.Parallel()

	kind, color, ok := parseQueryResponse("11;rgba:ffff/8000/0000/ffff")
	if !ok {
		t.Fatal("expected rgba query response to parse")
	}
	if kind != "11" {
		t.Fatalf("kind = %q, want %q", kind, "11")
	}
	if color != (rgbColor{R: 0xff, G: 0x80, B: 0x00}) {
		t.Fatalf("color = %#v, want %#v", color, rgbColor{R: 0xff, G: 0x80, B: 0x00})
	}
}

func TestBuildThemeUsesTintedBackgroundsWhenPaletteKnown(t *testing.T) {
	t.Parallel()

	fg := rgbColor{R: 240, G: 240, B: 240}
	bg := rgbColor{R: 12, G: 12, B: 12}
	theme := buildTheme(capabilityTrueColor, &fg, &bg)

	if theme.previewBG == nil || theme.selectedBG == nil {
		t.Fatal("expected tinted preview and selection backgrounds when palette is known")
	}
	if theme.mutedFG == nil || theme.accentFG == nil {
		t.Fatal("expected derived foreground colors when palette is known")
	}
}
