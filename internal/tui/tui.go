package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mariusae/tmux-agents/internal/app"
	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/tmux"
)

type uiState struct {
	application *app.App
	terminal    *terminal

	agents       []model.Agent
	selected     int
	previewLines []string
	previewAt    time.Time
	lastError    string
	focused      bool

	fg    *rgbColor
	bg    *rgbColor
	theme theme
}

func Run(ctx context.Context, application *app.App) error {
	term, err := openTerminal()
	if err != nil {
		return err
	}

	closed := false
	defer func() {
		if !closed {
			_ = term.close()
		}
	}()

	ui := &uiState{
		application: application,
		terminal:    term,
		focused:     true,
		theme:       buildTheme(detectCapabilityFromEnv(), nil, nil),
	}

	_ = term.requestPalette()
	ui.refreshAgents(ctx)
	ui.refreshPreview(ctx)
	_ = term.redraw(ui.render(time.Now()))

	agentsTicker := time.NewTicker(250 * time.Millisecond)
	previewTicker := time.NewTicker(150 * time.Millisecond)
	defer agentsTicker.Stop()
	defer previewTicker.Stop()

	reconcileDone := startBackgroundReconcile(ctx, 750*time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-term.Events():
			switch ev.typ {
			case eventKey:
				switch ev.key {
				case keyUp:
					if ui.selected > 0 {
						ui.selected--
						ui.refreshPreview(ctx)
					}
				case keyDown:
					if ui.selected+1 < len(ui.agents) {
						ui.selected++
						ui.refreshPreview(ctx)
					}
				case keyOpen:
					target := ui.selectedTarget()
					if target == "" {
						break
					}
					closed = true
					_ = term.close()
					return tmux.SelectTarget(ctx, target)
				case keyEnter:
					ui.forwardNamedKey(ctx, "Enter")
				case keyEscape:
					return nil
				case keyLiteral:
					ui.forwardLiteral(ctx, ev.text)
				}
			case eventResize:
			case eventFocusGained:
				ui.focused = true
				_ = term.requestPalette()
			case eventFocusLost:
				ui.focused = false
			case eventColorQuery:
				switch ev.kind {
				case "10":
					color := ev.color
					ui.fg = &color
				case "11":
					color := ev.color
					ui.bg = &color
				}
				ui.theme = buildTheme(detectCapabilityFromEnv(), ui.fg, ui.bg)
			}
			_ = term.redraw(ui.render(time.Now()))
		case <-agentsTicker.C:
			ui.refreshAgents(ctx)
			_ = term.redraw(ui.render(time.Now()))
		case <-previewTicker.C:
			ui.refreshPreview(ctx)
			_ = term.redraw(ui.render(time.Now()))
		case err, ok := <-reconcileDone:
			if !ok {
				reconcileDone = nil
				continue
			}
			if err != nil {
				ui.lastError = err.Error()
			} else {
				ui.refreshAgents(ctx)
				ui.refreshPreview(ctx)
			}
			_ = term.redraw(ui.render(time.Now()))
		}
	}
}

func (ui *uiState) refreshAgents(ctx context.Context) {
	selectedKey := ui.selectedKey()

	agents, err := ui.application.AgentsSnapshot(ctx)
	if err != nil {
		ui.lastError = err.Error()
		return
	}

	ui.agents = agents
	ui.lastError = ""

	if len(agents) == 0 {
		ui.selected = 0
		return
	}

	for i, agent := range agents {
		if agent.Key == selectedKey && selectedKey != "" {
			ui.selected = i
			return
		}
	}

	if ui.selected >= len(agents) {
		ui.selected = len(agents) - 1
	}
	if ui.selected < 0 {
		ui.selected = 0
	}
}

func (ui *uiState) refreshPreview(ctx context.Context) {
	target := ui.selectedTarget()
	if target == "" {
		ui.previewLines = nil
		return
	}

	_, height := ui.terminal.Size()
	captureLines := maxInt(20, height-3)

	text, err := tmux.CapturePaneStyled(ctx, target, captureLines)
	if err != nil {
		ui.lastError = err.Error()
		return
	}

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	ui.previewLines = trimEmptyEdgeLines(lines)
	ui.previewAt = time.Now()
}

func (ui *uiState) render(now time.Time) string {
	width, height := ui.terminal.Size()
	if width < 48 || height < 8 {
		return fillStyledLine("tmux-agents: terminal too small", width, textStyle{bold: true}) +
			strings.Repeat("\n"+fillStyledLine("", width, textStyle{}), maxInt(0, height-1))
	}

	leftWidth := clampInt(width/3, 28, 42)
	if width-leftWidth-3 < 24 {
		leftWidth = maxInt(20, width-27)
	}
	rightWidth := maxInt(20, width-leftWidth-3)

	lines := make([]string, 0, height)
	lines = append(lines, ui.renderTopBorder(leftWidth, rightWidth))

	contentHeight := height - 3
	sidebarLines := ui.renderSidebar(leftWidth, contentHeight, now)
	previewLines := ui.renderPreviewPane(rightWidth, contentHeight, now)
	borderStyle := textStyle{fg: ui.theme.mutedFG}
	vertical := styleSequence(borderStyle) + "│" + resetSequence()

	for i := 0; i < contentHeight; i++ {
		lines = append(lines, vertical+sidebarLines[i]+vertical+previewLines[i]+vertical)
	}

	lines = append(lines, ui.renderBottomBorder(leftWidth, rightWidth))
	lines = append(lines, ui.renderFooter(width))
	return strings.Join(lines, "\n")
}

func (ui *uiState) renderFooter(width int) string {
	message := " Esc quit  Ctrl-O open pane  ↑/↓ select  typing + Enter send to pane "
	if ui.lastError != "" {
		message = " " + ui.lastError + " "
	}
	style := textStyle{fg: ui.theme.mutedFG, dim: ui.lastError == ""}
	return fillStyledLine(message, width, style)
}

func (ui *uiState) renderSidebar(width, height int, now time.Time) []string {
	lines := make([]string, height)
	base := textStyle{}

	if len(ui.agents) == 0 {
		if height > 0 {
			lines[0] = fillStyledLine(" no agents detected", width, textStyle{dim: true})
		}
		for i := 1; i < height; i++ {
			lines[i] = fillStyledLine("", width, base)
		}
		return lines
	}

	visibleItems := maxInt(1, height)
	start := scrollStart(ui.selected, visibleItems, len(ui.agents))

	row := 0
	for index := start; index < len(ui.agents) && row < height; index++ {
		agent := ui.agents[index]
		selected := index == ui.selected
		rowStyle := base
		if selected {
			rowStyle.bg = ui.theme.selectedBG
			if rowStyle.bg == nil {
				rowStyle.reverse = true
			}
		}

		active := formatActiveTime(now, agent.LastActivityAt())
		indicator := " "
		if !agent.LastActiveAt.IsZero() && now.Sub(agent.LastActiveAt) < 30*time.Second {
			indicator = "↯"
		}
		lines[row] = renderEdgeLine(width, rowStyle,
			segment{text: " " + indicator + agent.Label(), style: textStyle{bold: agent.AwaitingInput}},
			segment{text: " " + active + " ", style: textStyle{fg: ui.theme.mutedFG, dim: true}},
		)
		row++
	}

	for row < height {
		lines[row] = fillStyledLine("", width, base)
		row++
	}
	return lines
}

func (ui *uiState) renderPreviewPane(width, height int, now time.Time) []string {
	lines := make([]string, height)
	if height == 0 {
		return lines
	}

	selected := ui.selectedAgent()
	base := textStyle{bg: ui.theme.previewBG}

	if selected.Key == "" {
		lines[0] = fillStyledLine(" no selected agent", width, textStyle{bg: ui.theme.previewBG, dim: true})
		for i := 1; i < height; i++ {
			lines[i] = fillStyledLine("", width, base)
		}
		return lines
	}

	if len(ui.previewLines) == 0 {
		lines[0] = fillStyledLine(" preview unavailable", width, textStyle{bg: ui.theme.previewBG, dim: true})
		for i := 1; i < height; i++ {
			lines[i] = fillStyledLine("", width, base)
		}
		return lines
	}

	start := 0
	if len(ui.previewLines) > height {
		start = len(ui.previewLines) - height
	}

	row := 0
	for _, line := range ui.previewLines[start:] {
		if row >= height {
			break
		}
		lines[row] = renderANSILine(line, width)
		row++
	}
	for row < height {
		lines[row] = fillStyledLine("", width, base)
		row++
	}
	return lines
}

func (ui *uiState) renderTopBorder(leftWidth, rightWidth int) string {
	borderStyle := textStyle{fg: ui.theme.mutedFG}
	left := titledBorderSection(" INBOX ", leftWidth)

	rightTitle := " PREVIEW "
	if target := ui.selectedTarget(); target != "" {
		rightTitle = " " + target + " "
	}
	right := titledBorderSection(rightTitle, rightWidth)
	return styleSequence(borderStyle) + "┌" + left + "┬" + right + "┐" + resetSequence()
}

func (ui *uiState) renderBottomBorder(leftWidth, rightWidth int) string {
	borderStyle := textStyle{fg: ui.theme.mutedFG}
	return styleSequence(borderStyle) + "└" + strings.Repeat("─", leftWidth) + "┴" + strings.Repeat("─", rightWidth) + "┘" + resetSequence()
}

func titledBorderSection(title string, width int) string {
	titleWidth := len([]rune(title))
	if titleWidth >= width {
		return truncateText(title, width)
	}
	return title + strings.Repeat("─", width-titleWidth)
}

func trimEmptyEdgeLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(stripANSI(lines[start])) == "" {
		start++
	}

	end := len(lines)
	for end > start && strings.TrimSpace(stripANSI(lines[end-1])) == "" {
		end--
	}

	if start >= end {
		return nil
	}
	return lines[start:end]
}

func stripANSI(text string) string {
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

func (ui *uiState) selectedAgent() model.Agent {
	if ui.selected >= 0 && ui.selected < len(ui.agents) {
		return ui.agents[ui.selected]
	}
	return model.Agent{}
}

func (ui *uiState) selectedTarget() string {
	return ui.selectedAgent().TargetLabel()
}

func (ui *uiState) selectedKey() string {
	return ui.selectedAgent().Key
}

func (ui *uiState) forwardNamedKey(ctx context.Context, keyName string) {
	target := ui.selectedTarget()
	if target == "" {
		return
	}
	if err := tmux.SendKeyName(ctx, target, keyName); err != nil {
		ui.lastError = err.Error()
		return
	}
	ui.lastError = ""
}

func (ui *uiState) forwardLiteral(ctx context.Context, text string) {
	target := ui.selectedTarget()
	if target == "" || text == "" {
		return
	}
	if err := tmux.SendKeysLiteral(ctx, target, text); err != nil {
		ui.lastError = err.Error()
		return
	}
	ui.lastError = ""
}

func startBackgroundReconcile(ctx context.Context, interval time.Duration) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)

		timer := time.NewTimer(0)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			_, err := app.ReconcileDefault(ctx)
			select {
			case <-ctx.Done():
				return
			case done <- err:
			default:
			}

			timer.Reset(interval)
		}
	}()
	return done
}

type segment struct {
	text  string
	style textStyle
}

func renderEdgeLine(width int, base textStyle, left, right segment) string {
	leftText := truncateText(left.text, width)
	rightText := truncateText(right.text, maxInt(0, width-len([]rune(leftText))))
	gap := maxInt(0, width-len([]rune(leftText))-len([]rune(rightText)))
	segments := []segment{
		{text: leftText, style: left.style},
		{text: strings.Repeat(" ", gap), style: textStyle{}},
		{text: rightText, style: right.style},
	}
	return renderLineSegments(width, base, segments)
}

func renderLineSegments(width int, base textStyle, segments []segment) string {
	var b strings.Builder
	remaining := width
	for _, seg := range segments {
		if remaining <= 0 {
			break
		}
		text := truncateText(seg.text, remaining)
		remaining -= len([]rune(text))
		style := mergeStyle(base, seg.style)
		b.WriteString(styleSequence(style))
		b.WriteString(text)
	}
	if remaining > 0 {
		b.WriteString(styleSequence(base))
		b.WriteString(strings.Repeat(" ", remaining))
	}
	b.WriteString(resetSequence())
	return b.String()
}

func fillStyledLine(text string, width int, style textStyle) string {
	text = truncateText(text, width)
	padding := maxInt(0, width-len([]rune(text)))
	return styleSequence(style) + text + strings.Repeat(" ", padding) + resetSequence()
}

func mergeStyle(base, extra textStyle) textStyle {
	merged := base
	if extra.fg != nil {
		merged.fg = extra.fg
	}
	if extra.bg != nil {
		merged.bg = extra.bg
	}
	merged.bold = merged.bold || extra.bold
	merged.dim = merged.dim || extra.dim
	merged.reverse = merged.reverse || extra.reverse
	return merged
}

func truncateText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func renderANSILine(text string, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	column := 0

	for i := 0; i < len(text) && column < width; {
		switch text[i] {
		case '\x1b':
			sequence, next, ok := readANSISequence(text, i)
			if !ok {
				i++
				continue
			}
			if isRenderableANSI(sequence) {
				b.WriteString(sequence)
			}
			i = next
		case '\t':
			spaces := 8 - (column % 8)
			for j := 0; j < spaces && column < width; j++ {
				b.WriteByte(' ')
				column++
			}
			i++
		case '\r', '\n':
			i++
		default:
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError && size == 1 {
				i++
				continue
			}
			if r < 0x20 || r == 0x7f {
				i += size
				continue
			}
			if column+1 > width {
				break
			}
			b.WriteRune(r)
			column++
			i += size
		}
	}

	if column < width {
		b.WriteString(strings.Repeat(" ", width-column))
	}
	b.WriteString(resetSequence())
	return b.String()
}

func readANSISequence(text string, start int) (string, int, bool) {
	if start+1 >= len(text) {
		return "", start + 1, false
	}

	switch text[start+1] {
	case '[':
		i := start + 2
		for i < len(text) {
			c := text[i]
			if c >= 0x40 && c <= 0x7e {
				return text[start : i+1], i + 1, true
			}
			i++
		}
	case ']':
		i := start + 2
		for i < len(text) {
			switch text[i] {
			case '\a':
				return text[start : i+1], i + 1, true
			case '\x1b':
				if i+1 < len(text) && text[i+1] == '\\' {
					return text[start : i+2], i + 2, true
				}
			}
			i++
		}
	default:
		return text[start : start+2], start + 2, true
	}

	return "", len(text), false
}

func isRenderableANSI(sequence string) bool {
	if sequence == "" || !strings.HasPrefix(sequence, "\x1b[") {
		return false
	}
	final := sequence[len(sequence)-1]
	switch final {
	case 'm', 'K':
		return true
	default:
		return false
	}
}

func scrollStart(selected, window, total int) int {
	if total <= window {
		return 0
	}
	start := selected - window/2
	if start < 0 {
		start = 0
	}
	if start+window > total {
		start = total - window
	}
	return start
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatActiveTime(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	now = now.Local()
	t = t.Local()
	if t.After(now) {
		t = now
	}

	age := now.Sub(t)
	switch {
	case age < 30*time.Second:
		return "just now"
	case age < 90*time.Second:
		return "last minute"
	case age < time.Hour:
		return fmt.Sprintf("%dmin", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age/time.Hour))
	}

	nowY, nowM, nowD := now.Date()
	tY, tM, tD := t.Date()
	nowDate := time.Date(nowY, nowM, nowD, 0, 0, 0, 0, now.Location())
	tDate := time.Date(tY, tM, tD, 0, 0, 0, 0, t.Location())
	if nowDate.Sub(tDate) == 24*time.Hour {
		return "yesterday"
	}
	if now.Year() == t.Year() {
		return t.Format("Mon02")
	}
	return t.Format("02Jan06")
}
