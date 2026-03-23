package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

type eventType int

const (
	eventNone eventType = iota
	eventKey
	eventResize
	eventFocusGained
	eventFocusLost
	eventColorQuery
)

type keyCode int

const (
	keyUnknown keyCode = iota
	keyUp
	keyDown
	keyEnter
	keyEscape
	keyTab
	keyBackspace
	keyLiteral
	keyOpen
	keyQuit
)

type event struct {
	typ    eventType
	key    keyCode
	text   string
	width  int
	height int
	kind   string
	color  rgbColor
}

type terminal struct {
	tty      *os.File
	state    *term.State
	events   chan event
	rawBytes chan byte
	done     chan struct{}

	mu     sync.RWMutex
	width  int
	height int

	resizeCh chan os.Signal
	wg       sync.WaitGroup
}

type paletteProbe struct {
	Foreground   *rgbColor
	Background   *rgbColor
	RawResponses []string
	QueryWrapped bool
	TTYOpened    bool
	InputSource  string
	OutputSource string
	Capability   colorCapability
	PaletteKnown bool
	ProbeError   string
}

func openTerminal() (*terminal, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	state, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		_ = tty.Close()
		return nil, err
	}

	width, height, err := term.GetSize(int(tty.Fd()))
	if err != nil {
		width = 80
		height = 24
	}

	t := &terminal{
		tty:      tty,
		state:    state,
		events:   make(chan event, 128),
		rawBytes: make(chan byte, 1024),
		done:     make(chan struct{}),
		width:    width,
		height:   height,
		resizeCh: make(chan os.Signal, 8),
	}

	if err := t.write("\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H\x1b[?1004h"); err != nil {
		_ = term.Restore(int(tty.Fd()), state)
		_ = tty.Close()
		return nil, err
	}

	signal.Notify(t.resizeCh, syscall.SIGWINCH)

	t.wg.Add(3)
	go t.readLoop()
	go t.parseLoop()
	go t.resizeLoop()
	return t, nil
}

func (t *terminal) close() error {
	if t == nil {
		return nil
	}

	close(t.done)
	signal.Stop(t.resizeCh)
	_ = t.write("\x1b[?1004l\x1b[0m\x1b[?25h\x1b[?1049l")
	_ = term.Restore(int(t.tty.Fd()), t.state)
	_ = t.tty.Close()
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(25 * time.Millisecond):
	}
	return nil
}

func (t *terminal) Events() <-chan event {
	return t.events
}

func (t *terminal) Size() (int, int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.width, t.height
}

func (t *terminal) redraw(frame string) error {
	lines := strings.Split(frame, "\n")

	var b strings.Builder
	b.WriteString("\x1b[H")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString(";1H")
		}
		b.WriteString(line)
		b.WriteString("\x1b[K")
	}
	return t.write(b.String())
}

func (t *terminal) requestPalette() error {
	if err := t.writeQuery(colorQuerySequence("10")); err != nil {
		return err
	}
	return t.writeQuery(colorQuerySequence("11"))
}

func (t *terminal) write(text string) error {
	_, err := io.WriteString(t.tty, text)
	return err
}

func (t *terminal) writeQuery(text string) error {
	if err := t.write(text); err == nil {
		return nil
	}
	_, err := io.WriteString(os.Stdout, text)
	return err
}

func (t *terminal) readLoop() {
	defer t.wg.Done()

	buf := make([]byte, 1)
	for {
		n, err := t.tty.Read(buf)
		if err != nil {
			select {
			case <-t.done:
				return
			default:
				return
			}
		}
		if n == 0 {
			continue
		}
		select {
		case <-t.done:
			return
		case t.rawBytes <- buf[0]:
		}
	}
}

func (t *terminal) resizeLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.done:
			return
		case <-t.resizeCh:
			width, height, err := term.GetSize(int(t.tty.Fd()))
			if err != nil {
				continue
			}
			t.mu.Lock()
			t.width = width
			t.height = height
			t.mu.Unlock()

			select {
			case <-t.done:
				return
			case t.events <- event{typ: eventResize, width: width, height: height}:
			}
		}
	}
}

func (t *terminal) parseLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.done:
			return
		case b := <-t.rawBytes:
			switch b {
			case 0x03:
				t.emit(event{typ: eventKey, key: keyQuit})
			case 0x0f:
				t.emit(event{typ: eventKey, key: keyOpen})
			case '\r', '\n':
				t.emit(event{typ: eventKey, key: keyEnter})
			case '\t':
				t.emit(event{typ: eventKey, key: keyTab})
			case 0x08, 0x7f:
				t.emit(event{typ: eventKey, key: keyBackspace})
			case 0x1b:
				t.parseEscape()
			default:
				if b >= 0x20 && b <= 0x7e {
					t.emit(event{typ: eventKey, key: keyLiteral, text: string([]byte{b})})
				}
			}
		}
	}
}

func (t *terminal) parseEscape() {
	next, ok := t.nextByte(25 * time.Millisecond)
	if !ok {
		t.emit(event{typ: eventKey, key: keyEscape})
		return
	}

	switch next {
	case '[':
		t.parseCSI()
	case ']':
		t.parseOSC()
	case 'P':
		t.parseDCS()
	default:
		t.emit(event{typ: eventKey, key: keyEscape})
	}
}

func (t *terminal) parseCSI() {
	sequence := make([]byte, 0, 8)
	for {
		b, ok := t.nextByte(50 * time.Millisecond)
		if !ok {
			return
		}
		sequence = append(sequence, b)
		if b >= 0x40 && b <= 0x7e {
			break
		}
	}

	if len(sequence) == 0 {
		return
	}

	final := sequence[len(sequence)-1]
	switch final {
	case 'A':
		t.emit(event{typ: eventKey, key: keyUp})
	case 'B':
		t.emit(event{typ: eventKey, key: keyDown})
	case 'I':
		t.emit(event{typ: eventFocusGained})
	case 'O':
		t.emit(event{typ: eventFocusLost})
	}
}

func (t *terminal) parseOSC() {
	buffer, ok := readOSCBuffer(t.nextByte)
	if !ok {
		return
	}
	if kind, color, ok := parseQueryResponse(string(buffer)); ok {
		t.emit(event{typ: eventColorQuery, kind: kind, color: color})
	}
}

func (t *terminal) parseDCS() {
	buffer, ok := readDCSBuffer(t.nextByte)
	if !ok {
		return
	}
	if kind, color, ok := parseWrappedColorResponse(string(buffer)); ok {
		t.emit(event{typ: eventColorQuery, kind: kind, color: color})
	}
}

func readOSCBuffer(next func(time.Duration) (byte, bool)) ([]byte, bool) {
	buffer := make([]byte, 0, 32)
	for {
		b, ok := next(100 * time.Millisecond)
		if !ok {
			return nil, false
		}
		switch b {
		case 0x07:
			return buffer, true
		case 0x1b:
			next, ok := next(10 * time.Millisecond)
			if !ok {
				return nil, false
			}
			if next == '\\' {
				return buffer, true
			}
			buffer = append(buffer, b, next)
		default:
			buffer = append(buffer, b)
		}
	}
}

func readDCSBuffer(next func(time.Duration) (byte, bool)) ([]byte, bool) {
	buffer := make([]byte, 0, 64)
	for {
		b, ok := next(100 * time.Millisecond)
		if !ok {
			return nil, false
		}
		if b == 0x1b {
			next, ok := next(10 * time.Millisecond)
			if !ok {
				return nil, false
			}
			if next == '\\' {
				return buffer, true
			}
			buffer = append(buffer, b, next)
			continue
		}
		buffer = append(buffer, b)
	}
}

func (t *terminal) nextByte(timeout time.Duration) (byte, bool) {
	select {
	case <-t.done:
		return 0, false
	case b := <-t.rawBytes:
		return b, true
	case <-time.After(timeout):
		return 0, false
	}
}

func (t *terminal) emit(ev event) {
	select {
	case <-t.done:
	case t.events <- ev:
	default:
	}
}

func colorQuerySequence(slot string) string {
	return "\x1b]" + slot + ";?\x1b\\"
}

func ignoreTerminalClose(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

func probePalette(ctx context.Context) paletteProbe {
	probe := paletteProbe{
		QueryWrapped: false,
		Capability:   detectCapabilityFromEnv(),
	}

	input, inputSource, inputErr := openQueryInput()
	if inputErr != nil {
		probe.ProbeError = inputErr.Error()
		return probe
	}
	probe.InputSource = inputSource
	probe.TTYOpened = inputSource == "/dev/tty"
	if input != os.Stdin {
		defer input.Close()
	}

	output, outputSource, outputErr := openQueryOutput()
	if outputErr != nil {
		probe.ProbeError = outputErr.Error()
		return probe
	}
	probe.OutputSource = outputSource
	if output != os.Stdout {
		defer output.Close()
	}

	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		probe.ProbeError = err.Error()
		return probe
	}
	defer term.Restore(int(input.Fd()), state)

	rawBytes := make(chan byte, 1024)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := input.Read(buf)
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			select {
			case rawBytes <- buf[0]:
			case <-ctx.Done():
				return
			}
		}
	}()

	if _, err := io.WriteString(output, colorQuerySequence("10")); err != nil {
		if output != os.Stdout {
			if _, stdoutErr := io.WriteString(os.Stdout, colorQuerySequence("10")); stdoutErr == nil {
				probe.OutputSource = "stdout (fallback)"
			} else {
				probe.ProbeError = err.Error()
				return probe
			}
		} else {
			probe.ProbeError = err.Error()
			return probe
		}
	}
	if _, err := io.WriteString(output, colorQuerySequence("11")); err != nil {
		if output != os.Stdout {
			if _, stdoutErr := io.WriteString(os.Stdout, colorQuerySequence("11")); stdoutErr == nil {
				probe.OutputSource = "stdout (fallback)"
			} else {
				probe.ProbeError = err.Error()
				return probe
			}
		} else {
			probe.ProbeError = err.Error()
			return probe
		}
	}

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		if probe.Foreground != nil && probe.Background != nil {
			break
		}
		select {
		case <-ctx.Done():
			probe.ProbeError = ctx.Err().Error()
			return probe
		case <-timeout.C:
			if probe.Foreground == nil || probe.Background == nil {
				probe.ProbeError = "timed out waiting for terminal color query response"
			}
			return finalizeProbe(probe)
		case b := <-rawBytes:
			responses, fg, bg := consumeProbeByte(rawBytes, b)
			if len(responses) == 0 {
				continue
			}
			probe.RawResponses = append(probe.RawResponses, responses...)
			if fg != nil {
				probe.Foreground = fg
			}
			if bg != nil {
				probe.Background = bg
			}
		}
	}

	return finalizeProbe(probe)
}

func openQueryInput() (*os.File, string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err == nil {
		return tty, "/dev/tty", nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return os.Stdin, "stdin", nil
	}
	return nil, "", err
}

func openQueryOutput() (*os.File, string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err == nil {
		return tty, "/dev/tty", nil
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, "stdout", nil
	}
	return nil, "", err
}

func finalizeProbe(probe paletteProbe) paletteProbe {
	probe.PaletteKnown = probe.Background != nil
	return probe
}

func consumeProbeByte(rawBytes <-chan byte, first byte) ([]string, *rgbColor, *rgbColor) {
	if first != 0x1b {
		return nil, nil, nil
	}

	next, ok := nextProbeByte(rawBytes, 25*time.Millisecond)
	if !ok {
		return nil, nil, nil
	}
	switch next {
	case ']':
		buffer, ok := readOSCBuffer(func(timeout time.Duration) (byte, bool) {
			return nextProbeByte(rawBytes, timeout)
		})
		if !ok {
			return nil, nil, nil
		}
		raw := string(buffer)
		kind, color, ok := parseQueryResponse(raw)
		if !ok {
			return []string{raw}, nil, nil
		}
		return classifyProbeColor(raw, kind, color)
	case 'P':
		buffer, ok := readDCSBuffer(func(timeout time.Duration) (byte, bool) {
			return nextProbeByte(rawBytes, timeout)
		})
		if !ok {
			return nil, nil, nil
		}
		raw := string(buffer)
		kind, color, ok := parseWrappedColorResponse(raw)
		if !ok {
			return []string{raw}, nil, nil
		}
		return classifyProbeColor(raw, kind, color)
	default:
		return nil, nil, nil
	}
}

func classifyProbeColor(raw, kind string, color rgbColor) ([]string, *rgbColor, *rgbColor) {
	switch kind {
	case "10":
		return []string{raw}, &color, nil
	case "11":
		return []string{raw}, nil, &color
	default:
		return []string{raw}, nil, nil
	}
}

func parseWrappedColorResponse(raw string) (string, rgbColor, bool) {
	text := raw
	if strings.HasPrefix(text, "tmux;") {
		text = strings.TrimPrefix(text, "tmux;")
		text = strings.ReplaceAll(text, "\x1b\x1b", "\x1b")
	}
	if strings.HasPrefix(text, "\x1b]") {
		text = strings.TrimPrefix(text, "\x1b]")
		if strings.HasSuffix(text, "\x07") {
			text = strings.TrimSuffix(text, "\x07")
		}
		if strings.HasSuffix(text, "\x1b\\") {
			text = strings.TrimSuffix(text, "\x1b\\")
		}
	}
	return parseQueryResponse(text)
}

func nextProbeByte(rawBytes <-chan byte, timeout time.Duration) (byte, bool) {
	select {
	case b := <-rawBytes:
		return b, true
	case <-time.After(timeout):
		return 0, false
	}
}
