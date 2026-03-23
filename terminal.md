# Terminal UI Design Notes

This document describes a practical approach for terminal UI styling that adapts to the user's terminal rather than assuming a dark or light theme.

## Background Detection

Always prefer querying the terminal's default background color directly.

1. Ask the terminal for its default background RGB `bg = (r, g, b)`.
2. Classify it as light or dark with luminance:

```text
Y = 0.299*r + 0.587*g + 0.114*b
light iff Y > 128.0
```

If the background query fails, treat the theme as unknown and avoid applying custom tint backgrounds. Falling back to the terminal default is better than guessing wrong.

### How This `crossterm` Branch Performs Detection

In the `nornagon/color-query` branch of `crossterm`, background detection is done by actively querying the terminal's dynamic color slots. It does not rely on `TERM`, theme names, or environment heuristics.

For background color, `crossterm` queries OSC slot `11`; for foreground color it uses slot `10`.

The background query sequence is:

```text
ESC ] 11 ; ? ESC \
```

The implementation behavior is:

1. If raw mode is not already enabled, `crossterm` temporarily enables it for the query.
2. It tries to write the OSC query directly to `/dev/tty`.
3. If writing to `/dev/tty` fails, it falls back to writing the query to `stdout`.
4. It waits up to 2 seconds for a matching OSC color response.
5. While waiting, it uses the normal internal event reader, but filters for an `OSC color` reply for the requested slot only.
6. Unrelated events are skipped and preserved rather than consumed.

On Unix, the parser accepts replies terminated by either BEL or ST:

```text
ESC ] 11 ; rgb:0000/0000/ffff BEL
ESC ] 11 ; rgb:0000/0000/ffff ESC \
```

The payload parser accepts `rgb:` and `rgba:` forms:

- `rgb:rr/gg/bb`
- `rgb:rrrr/gggg/bbbb`
- `rgba:...` is also accepted, but the alpha component is validated and ignored

Component handling:

- 2 hex digits are used directly as 8-bit values
- 4 hex digits are downscaled to 8-bit by dividing by `257`

Examples:

- `rgb:ffff/8000/0000` becomes `(255, 128, 0)`
- `rgb:0000/0000/ffff` becomes `(0, 0, 255)`

Failure behavior matters:

- If the terminal returns an unrecognized OSC payload, `crossterm` returns `Ok(None)`
- If no reply arrives within 2 seconds, it returns an I/O error
- On non-Unix platforms, or without the `events` feature, this API is unsupported

This means support depends on all of the following:

- the terminal emulator implements OSC 10/11 queries
- the transport path allows the reply through
- if a multiplexer such as `tmux` is in use, it forwards the OSC response correctly

In practice, this is a true request/response protocol over terminal escape sequences, parsed back through the event system.

## Tinting

Tinting should be derived from the terminal background, not hard-coded colors.

### Base Algorithm

Choose an overlay color and alpha, then blend it over the terminal background:

- Light background:
  - overlay = `(0, 0, 0)`
- Dark background:
  - overlay = `(255, 255, 255)`

Blend per channel:

```text
out = floor(overlay * alpha + bg * (1 - alpha))
```

Then map the blended RGB to terminal capabilities:

- TrueColor: use exact RGB
- ANSI 256: choose nearest xterm-256 color by perceptual distance
- ANSI 16 or unknown: use the terminal default color instead of forcing a bad approximation

### Recommended Purpose-Specific Alphas

A single alpha is usually not enough. Different surfaces need different strength.

- Subtle persistent surfaces:
  - Light: `0.04`
  - Dark: `0.12`
  - Example: chat composer background, low-emphasis panels

- Diff changed-line backgrounds in `mdiff`:
  - Light: `0.04`
  - Dark: `0.04`
  - Goal: visible but restrained

- Search, finder, and transient HUD overlays in `mdiff`:
  - Light: `0.10`
  - Dark: `0.20`
  - Goal: stronger visual anchoring for active interaction

### Examples

With a white terminal background `(255,255,255)`:

- `alpha = 0.04` with black overlay gives `(244,244,244)`
- `alpha = 0.10` with black overlay gives `(229,229,229)`

With a black terminal background `(0,0,0)`:

- `alpha = 0.12` with white overlay gives `(30,30,30)`
- `alpha = 0.20` with white overlay gives `(51,51,51)`

## Shimmer Effect

For animated status text, use a moving cosine band rather than color cycling.

### Timing Model

- Anchor animation to a process-global start time so redraws stay stable.
- Repeat every `2.0s`.
- For text of length `N`, sweep over `N + 20` positions so the band can enter and exit offscreen.

Position:

```text
pos = floor(((elapsed_seconds mod 2.0) / 2.0) * (N + 20))
```

For character `i`:

```text
i_pos = i + 10
dist = abs(i_pos - pos)
```

Use a cosine falloff with half-width `5`:

```text
if dist <= 5:
  t = 0.5 * (1 + cos(pi * dist / 5))
else:
  t = 0
```

### Rendering

- TrueColor terminals:
  - Blend between default foreground and default background with intensity `t * 0.9`
  - Render the shimmer text in bold

- Lower-color terminals:
  - `t < 0.2`: dim
  - `0.2 <= t < 0.6`: normal
  - `t >= 0.6`: bold

Redraw around every `32ms` to keep the shimmer smooth.

## Focus Regain and Background Refresh

Terminal theme may change while the app is unfocused. If tinting depends on the terminal background, refresh it on focus regain.

### Protocol

Enable terminal focus reporting:

```text
enable:  CSI ? 1004 h
disable: CSI ? 1004 l
focus in:  CSI I
focus out: CSI O
```

With `crossterm`, this corresponds to `EnableFocusChange` and `DisableFocusChange`, and incoming events become `Event::FocusGained` and `Event::FocusLost`.

### Practical Requirements

- The terminal emulator must support focus reporting.
- If running under `tmux`, it must forward focus events.
- The application must listen for focus events and rerender on `FocusGained`.

### Refresh Policy

On `FocusGained`:

1. Re-query the terminal default background color.
2. Recompute light/dark classification.
3. Recompute all tint colors derived from the background.
4. Rerender the UI if any tint value changed.

This is separate from focus reporting itself: even if focus events work, background refresh still depends on the terminal supporting default-color queries.

## Summary

Use queried terminal colors, classify by luminance, tint by blending toward black or white, and vary alpha by purpose. Use shimmer sparingly for live status surfaces. Refresh derived colors on focus regain so the UI tracks terminal theme changes without restart.
