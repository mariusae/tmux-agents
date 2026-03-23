package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	StatusInterpolation       = "#(tmux-agents status -d \" • \")"
	legacyStatusInterpolation = "#(tmux-agents status)"
	RecommendedInterval       = 1
)

type TmuxStatus struct {
	StatusRight    string
	StatusInterval int
	Available      bool
}

func ShowSetupText(ctx context.Context) (string, error) {
	status, err := ReadTmuxStatus(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("tmux.conf line:\n")
	b.WriteString(`run "tmux-agents setup"`)
	b.WriteString("\n\n")
	b.WriteString("status interpolation:\n")
	b.WriteString(StatusInterpolation)
	b.WriteString("\n\n")
	b.WriteString("runtime tmux commands:\n")
	b.WriteString(fmt.Sprintf("tmux set-option -g status-interval %d\n", RecommendedInterval))
	if status.Available {
		b.WriteString(fmt.Sprintf("tmux set-option -g status-right %s\n", strconv.Quote(desiredStatusRight(status.StatusRight))))
	} else {
		b.WriteString(fmt.Sprintf("tmux set-option -g status-right %s\n", strconv.Quote(StatusInterpolation+"<existing status-right>")))
	}

	if status.Available {
		b.WriteString("\ncurrent tmux status:\n")
		b.WriteString(fmt.Sprintf("status-right: %s\n", status.StatusRight))
		b.WriteString(fmt.Sprintf("status-interval: %d\n", status.StatusInterval))
		b.WriteString(fmt.Sprintf("recommended status-right: %s\n", desiredStatusRight(status.StatusRight)))
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

func ApplyTmuxSetup(ctx context.Context) ([]string, error) {
	status, err := ReadTmuxStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Available {
		return []string{"tmux not running; skipped runtime tmux setup"}, nil
	}

	messages := make([]string, 0, 2)
	if status.StatusInterval != RecommendedInterval {
		if err := runTmux(ctx, "set-option", "-g", "status-interval", strconv.Itoa(RecommendedInterval)); err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("set status-interval to %d", RecommendedInterval))
	}

	nextStatusRight := desiredStatusRight(status.StatusRight)
	if nextStatusRight != status.StatusRight {
		if err := runTmux(ctx, "set-option", "-g", "status-right", nextStatusRight); err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("prepended %s to status-right", StatusInterpolation))
	}

	if len(messages) == 0 {
		messages = append(messages, "tmux status setup already applied")
	}
	return messages, nil
}

func ReadTmuxStatus(ctx context.Context) (TmuxStatus, error) {
	right, err := readTmuxOption(ctx, "status-right")
	if err != nil {
		if errors.Is(err, errNoTmux) {
			return TmuxStatus{}, nil
		}
		return TmuxStatus{}, err
	}

	intervalText, err := readTmuxOption(ctx, "status-interval")
	if err != nil {
		if errors.Is(err, errNoTmux) {
			return TmuxStatus{}, nil
		}
		return TmuxStatus{}, err
	}

	interval, _ := strconv.Atoi(strings.TrimSpace(intervalText))
	return TmuxStatus{
		StatusRight:    strings.TrimSpace(right),
		StatusInterval: interval,
		Available:      true,
	}, nil
}

var errNoTmux = errors.New("tmux unavailable")

func readTmuxOption(ctx context.Context, option string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", "show-option", "-gqv", option)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errNoTmux
		}
		if strings.Contains(stderr.String(), "failed to connect") {
			return "", errNoTmux
		}
		return "", err
	}
	return stdout.String(), nil
}

func runTmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "failed to connect") {
			return errNoTmux
		}
		return err
	}
	return nil
}

func desiredStatusRight(current string) string {
	trimmed := strings.TrimSpace(current)
	for _, interpolation := range []string{StatusInterpolation, legacyStatusInterpolation} {
		trimmed = strings.TrimSpace(strings.ReplaceAll(trimmed, interpolation, ""))
	}
	if trimmed == "" {
		return StatusInterpolation
	}

	return StatusInterpolation + trimmed
}
