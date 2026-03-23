package tui

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type colorCapability int

const (
	capabilityUnknown colorCapability = iota
	capabilityANSI16
	capabilityANSI256
	capabilityTrueColor
)

type rgbColor struct {
	R uint8
	G uint8
	B uint8
}

type colorRef struct {
	rgb   rgbColor
	index int
	mode  colorCapability
}

type terminalPalette struct {
	capability colorCapability
	fg         *rgbColor
	bg         *rgbColor
	light      bool
}

type theme struct {
	palette    terminalPalette
	previewBG  *colorRef
	selectedBG *colorRef
	mutedFG    *colorRef
	accentFG   *colorRef
}

type textStyle struct {
	fg      *colorRef
	bg      *colorRef
	bold    bool
	dim     bool
	reverse bool
}

var animationStart = time.Now()

func detectCapabilityFromEnv() colorCapability {
	colorterm := strings.ToLower(os.Getenv("COLORTERM"))
	term := strings.ToLower(os.Getenv("TERM"))

	switch {
	case strings.Contains(colorterm, "truecolor"), strings.Contains(colorterm, "24bit"), strings.Contains(term, "direct"):
		return capabilityTrueColor
	case strings.Contains(term, "256color"):
		return capabilityANSI256
	case term != "":
		return capabilityANSI16
	default:
		return capabilityUnknown
	}
}

func selectedTintAlpha(light bool) float64 {
	if light {
		return 0.04
	}
	return 0.12
}

func buildTheme(capability colorCapability, fg, bg *rgbColor) theme {
	palette := terminalPalette{
		capability: capability,
		fg:         fg,
		bg:         bg,
	}
	if bg != nil {
		palette.light = luminance(*bg) > 128.0
	}

	t := theme{palette: palette}
	if bg != nil && capability >= capabilityANSI256 {
		persistentAlpha := selectedTintAlpha(palette.light)

		t.previewBG = colorForCapability(capability, blendTint(*bg, palette.light, persistentAlpha))
		t.selectedBG = colorForCapability(capability, blendTint(*bg, palette.light, persistentAlpha))
	}
	if fg != nil && bg != nil && capability >= capabilityANSI256 {
		muted := blendColors(*fg, *bg, 0.45)
		accent := blendColors(*fg, *bg, 0.85)
		t.mutedFG = colorForCapability(capability, muted)
		t.accentFG = colorForCapability(capability, accent)
	}
	return t
}

func luminance(c rgbColor) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func blendTint(bg rgbColor, light bool, alpha float64) rgbColor {
	overlay := rgbColor{R: 255, G: 255, B: 255}
	if light {
		overlay = rgbColor{}
	}
	return blendColors(overlay, bg, alpha)
}

func blendColors(overlay, bg rgbColor, alpha float64) rgbColor {
	return rgbColor{
		R: uint8(math.Floor(float64(overlay.R)*alpha + float64(bg.R)*(1.0-alpha))),
		G: uint8(math.Floor(float64(overlay.G)*alpha + float64(bg.G)*(1.0-alpha))),
		B: uint8(math.Floor(float64(overlay.B)*alpha + float64(bg.B)*(1.0-alpha))),
	}
}

func colorForCapability(capability colorCapability, rgb rgbColor) *colorRef {
	switch capability {
	case capabilityTrueColor:
		return &colorRef{rgb: rgb, mode: capabilityTrueColor}
	case capabilityANSI256:
		return &colorRef{rgb: rgb, index: nearestXterm256(rgb), mode: capabilityANSI256}
	default:
		return nil
	}
}

func styleSequence(style textStyle) string {
	codes := make([]string, 0, 6)
	if style.bold {
		codes = append(codes, "1")
	}
	if style.dim {
		codes = append(codes, "2")
	}
	if style.reverse {
		codes = append(codes, "7")
	}
	if style.fg != nil {
		switch style.fg.mode {
		case capabilityTrueColor:
			codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", style.fg.rgb.R, style.fg.rgb.G, style.fg.rgb.B))
		case capabilityANSI256:
			codes = append(codes, fmt.Sprintf("38;5;%d", style.fg.index))
		}
	}
	if style.bg != nil {
		switch style.bg.mode {
		case capabilityTrueColor:
			codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", style.bg.rgb.R, style.bg.rgb.G, style.bg.rgb.B))
		case capabilityANSI256:
			codes = append(codes, fmt.Sprintf("48;5;%d", style.bg.index))
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func resetSequence() string {
	return "\x1b[0m"
}

func capabilityLabel(capability colorCapability) string {
	switch capability {
	case capabilityTrueColor:
		return "truecolor"
	case capabilityANSI256:
		return "ansi256"
	case capabilityANSI16:
		return "ansi16"
	default:
		return "unknown"
	}
}

func formatRGB(color *rgbColor) string {
	if color == nil {
		return "unknown"
	}
	return fmt.Sprintf("#%02x%02x%02x (%d,%d,%d)", color.R, color.G, color.B, color.R, color.G, color.B)
}

func formatColorRef(color *colorRef) string {
	if color == nil {
		return "default"
	}
	switch color.mode {
	case capabilityTrueColor:
		return fmt.Sprintf("#%02x%02x%02x", color.rgb.R, color.rgb.G, color.rgb.B)
	case capabilityANSI256:
		return fmt.Sprintf("#%02x%02x%02x (xterm-256:%d)", color.rgb.R, color.rgb.G, color.rgb.B, color.index)
	default:
		return "default"
	}
}

func shimmerStyle(base theme, text string, now time.Time) textStyle {
	if base.palette.capability != capabilityTrueColor || base.palette.fg == nil || base.palette.bg == nil || text == "" {
		return textStyle{bold: true}
	}

	n := len([]rune(text))
	if n == 0 {
		return textStyle{bold: true}
	}

	elapsed := now.Sub(animationStart).Seconds()
	pos := math.Floor(math.Mod(elapsed, 2.0) / 2.0 * float64(n+20))

	maxT := 0.0
	for i := 0; i < n; i++ {
		iPos := float64(i + 10)
		dist := math.Abs(iPos - pos)
		if dist > 5 {
			continue
		}
		t := 0.5 * (1.0 + math.Cos(math.Pi*dist/5.0))
		if t > maxT {
			maxT = t
		}
	}

	baseFG := blendColors(*base.palette.fg, *base.palette.bg, 0.55)
	shimmerFG := blendColors(baseFG, *base.palette.fg, maxT*0.9)
	return textStyle{
		fg:   colorForCapability(capabilityTrueColor, shimmerFG),
		bold: true,
	}
}

func parseQueryResponse(raw string) (string, rgbColor, bool) {
	text := strings.TrimSpace(raw)
	parts := strings.SplitN(text, ";", 2)
	if len(parts) != 2 {
		return "", rgbColor{}, false
	}
	kind := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	var channels []string
	switch {
	case strings.HasPrefix(strings.ToLower(value), "rgb:"):
		channels = strings.Split(value[4:], "/")
		if len(channels) != 3 {
			return "", rgbColor{}, false
		}
	case strings.HasPrefix(strings.ToLower(value), "rgba:"):
		channels = strings.Split(value[5:], "/")
		if len(channels) != 4 {
			return "", rgbColor{}, false
		}
	default:
		return "", rgbColor{}, false
	}

	parseChannel := func(channel string) (uint8, bool) {
		channel = strings.TrimSpace(channel)
		if len(channel) != 2 && len(channel) != 4 {
			return 0, false
		}

		value, err := strconv.ParseUint(channel, 16, 16)
		if err != nil {
			return 0, false
		}
		if len(channel) == 2 {
			return uint8(value), true
		}
		return uint8((value + 128) / 257), true
	}

	r, ok := parseChannel(channels[0])
	if !ok {
		return "", rgbColor{}, false
	}
	g, ok := parseChannel(channels[1])
	if !ok {
		return "", rgbColor{}, false
	}
	b, ok := parseChannel(channels[2])
	if !ok {
		return "", rgbColor{}, false
	}
	if len(channels) == 4 {
		if _, ok := parseChannel(channels[3]); !ok {
			return "", rgbColor{}, false
		}
	}
	return kind, rgbColor{R: r, G: g, B: b}, true
}

func nearestXterm256(color rgbColor) int {
	bestIndex := 0
	bestDistance := math.MaxFloat64
	for index, candidate := range xterm256Palette() {
		distance := colorDistance(color, candidate)
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	return bestIndex
}

func colorDistance(a, b rgbColor) float64 {
	dr := float64(int(a.R) - int(b.R))
	dg := float64(int(a.G) - int(b.G))
	db := float64(int(a.B) - int(b.B))
	return 0.299*dr*dr + 0.587*dg*dg + 0.114*db*db
}

var cachedXterm256Palette []rgbColor

func xterm256Palette() []rgbColor {
	if cachedXterm256Palette != nil {
		return cachedXterm256Palette
	}

	palette := make([]rgbColor, 0, 256)
	base := []rgbColor{
		{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
		{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
		{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
		{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	palette = append(palette, base...)

	levels := []uint8{0, 95, 135, 175, 215, 255}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				palette = append(palette, rgbColor{R: levels[r], G: levels[g], B: levels[b]})
			}
		}
	}
	for gray := 0; gray < 24; gray++ {
		value := uint8(8 + gray*10)
		palette = append(palette, rgbColor{R: value, G: value, B: value})
	}

	cachedXterm256Palette = palette
	return cachedXterm256Palette
}
