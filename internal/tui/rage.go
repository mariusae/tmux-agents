package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

func Rage(ctx context.Context, w io.Writer) error {
	probe := probePalette(ctx)
	theme := buildTheme(probe.Capability, probe.Foreground, probe.Background)

	_, _ = fmt.Fprintf(w, "tmux-agents rage\n\n")
	_, _ = fmt.Fprintf(w, "environment:\n")
	_, _ = fmt.Fprintf(w, "  TERM=%s\n", envOrUnset("TERM"))
	_, _ = fmt.Fprintf(w, "  COLORTERM=%s\n", envOrUnset("COLORTERM"))
	_, _ = fmt.Fprintf(w, "  TMUX=%s\n", envOrUnset("TMUX"))
	_, _ = fmt.Fprintf(w, "\n")

	_, _ = fmt.Fprintf(w, "terminal:\n")
	_, _ = fmt.Fprintf(w, "  capability=%s\n", capabilityLabel(probe.Capability))
	_, _ = fmt.Fprintf(w, "  tty_opened=%t\n", probe.TTYOpened)
	_, _ = fmt.Fprintf(w, "  input_source=%s\n", emptyAsUnknown(probe.InputSource))
	_, _ = fmt.Fprintf(w, "  output_source=%s\n", emptyAsUnknown(probe.OutputSource))
	_, _ = fmt.Fprintf(w, "  query_wrapped_for_tmux=%t\n", probe.QueryWrapped)
	if probe.ProbeError != "" {
		_, _ = fmt.Fprintf(w, "  probe_error=%s\n", probe.ProbeError)
	}
	_, _ = fmt.Fprintf(w, "\n")

	_, _ = fmt.Fprintf(w, "palette:\n")
	_, _ = fmt.Fprintf(w, "  foreground=%s\n", formatRGB(probe.Foreground))
	_, _ = fmt.Fprintf(w, "  background=%s\n", formatRGB(probe.Background))
	if probe.Background != nil {
		_, _ = fmt.Fprintf(w, "  background_luminance=%.2f\n", luminance(*probe.Background))
		_, _ = fmt.Fprintf(w, "  background_classification=%s\n", classifyBackground(*probe.Background))
		_, _ = fmt.Fprintf(w, "  selected_tint_alpha=%.2f\n", selectedTintAlpha(theme.palette.light))
	} else {
		_, _ = fmt.Fprintf(w, "  background_classification=unknown\n")
		_, _ = fmt.Fprintf(w, "  selected_tint_alpha=unknown\n")
	}
	if len(probe.RawResponses) == 0 {
		_, _ = fmt.Fprintf(w, "  raw_responses=none\n")
	} else {
		_, _ = fmt.Fprintf(w, "  raw_responses:\n")
		for _, response := range probe.RawResponses {
			_, _ = fmt.Fprintf(w, "    %s\n", strings.TrimSpace(response))
		}
	}
	_, _ = fmt.Fprintf(w, "\n")

	_, _ = fmt.Fprintf(w, "computed_tints:\n")
	_, _ = fmt.Fprintf(w, "  selected_row_bg=%s\n", formatColorRef(theme.selectedBG))
	_, _ = fmt.Fprintf(w, "  preview_bg=%s\n", formatColorRef(theme.previewBG))
	_, _ = fmt.Fprintf(w, "  muted_fg=%s\n", formatColorRef(theme.mutedFG))
	_, _ = fmt.Fprintf(w, "  accent_fg=%s\n", formatColorRef(theme.accentFG))
	_, _ = fmt.Fprintf(w, "\n")

	_, _ = fmt.Fprintf(w, "notes:\n")
	switch {
	case probe.Background == nil:
		_, _ = fmt.Fprintf(w, "  background color query failed, so tint backgrounds fall back to terminal default\n")
	case probe.Capability < capabilityANSI256:
		_, _ = fmt.Fprintf(w, "  terminal capability is below ansi256, so tint backgrounds fall back to terminal default\n")
	case theme.selectedBG == nil:
		_, _ = fmt.Fprintf(w, "  selected tint was not computed\n")
	default:
		_, _ = fmt.Fprintf(w, "  selected rows should use a subtle background tint derived from the terminal background\n")
	}
	return nil
}

func envOrUnset(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "<unset>"
	}
	return value
}

func emptyAsUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func classifyBackground(color rgbColor) string {
	if luminance(color) > 128.0 {
		return "light"
	}
	return "dark"
}
