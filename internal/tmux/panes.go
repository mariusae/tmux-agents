package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Pane struct {
	Session        string
	Window         string
	WindowName     string
	Pane           string
	PaneID         string
	PanePID        int
	CurrentCommand string
}

func ListPanes(ctx context.Context) ([]Pane, error) {
	cmd := exec.CommandContext(
		ctx,
		"tmux",
		"list-panes",
		"-a",
		"-F",
		"#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil
		}
		if strings.Contains(stderr.String(), "failed to connect") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 7)
		if len(parts) != 7 {
			continue
		}

		panePID, err := strconv.Atoi(parts[5])
		if err != nil {
			panePID = 0
		}

		panes = append(panes, Pane{
			Session:        parts[0],
			Window:         parts[1],
			WindowName:     parts[2],
			Pane:           parts[3],
			PaneID:         parts[4],
			PanePID:        panePID,
			CurrentCommand: parts[6],
		})
	}

	return panes, nil
}

func CapturePaneTail(ctx context.Context, paneID string, lines int) (string, error) {
	return CapturePane(ctx, paneID, lines)
}

func CapturePane(ctx context.Context, target string, lines int) (string, error) {
	return capturePane(ctx, target, lines, false)
}

func CapturePaneStyled(ctx context.Context, target string, lines int) (string, error) {
	return capturePane(ctx, target, lines, true)
}

func capturePane(ctx context.Context, target string, lines int, styled bool) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", nil
	}

	start := fmt.Sprintf("-%d", lines)
	args := []string{"capture-pane", "-p", "-J", "-S", start, "-t", target}
	if styled {
		args = []string{"capture-pane", "-p", "-e", "-N", "-J", "-S", start, "-t", target}
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", nil
		}
		if strings.Contains(stderr.String(), "can't find pane") || strings.Contains(stderr.String(), "failed to connect") {
			return "", nil
		}
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}

	return stdout.String(), nil
}

func SelectTarget(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}

	session, windowTarget := splitSessionTarget(target)
	if session != "" {
		if err := runTmux(ctx, "switch-client", "-t", session); err != nil && !isNoCurrentClient(err) {
			return err
		}
	}
	if windowTarget != "" {
		if err := runTmux(ctx, "select-window", "-t", windowTarget); err != nil {
			return err
		}
	}
	return runTmux(ctx, "select-pane", "-t", target)
}

func SendKeysLiteral(ctx context.Context, target, text string) error {
	target = strings.TrimSpace(target)
	if target == "" || text == "" {
		return nil
	}
	return runTmux(ctx, "send-keys", "-t", target, "-l", text)
}

func SendKeyName(ctx context.Context, target, keyName string) error {
	target = strings.TrimSpace(target)
	keyName = strings.TrimSpace(keyName)
	if target == "" || keyName == "" {
		return nil
	}
	return runTmux(ctx, "send-keys", "-t", target, keyName)
}

func splitSessionTarget(target string) (string, string) {
	session, rest, found := strings.Cut(target, ":")
	if !found {
		return "", ""
	}
	window, _, _ := strings.Cut(rest, ".")
	if session == "" || window == "" {
		return "", ""
	}
	return session, session + ":" + window
}

func runTmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil
		}
		if strings.Contains(stderr.String(), "failed to connect") {
			return nil
		}
		return fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func isNoCurrentClient(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no current client")
}
