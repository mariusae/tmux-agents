package tmux

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

type ContextInfo struct {
	Session string
	Window  string
	Pane    string
	PaneID  string
}

func DiscoverCurrentContext(ctx context.Context) (ContextInfo, error) {
	paneID := strings.TrimSpace(os.Getenv("TMUX_PANE"))
	if paneID == "" {
		return ContextInfo{}, nil
	}

	cmd := exec.CommandContext(
		ctx,
		"tmux",
		"display-message",
		"-p",
		"-t",
		paneID,
		"#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ContextInfo{}, nil
		}
		return ContextInfo{}, err
	}

	parts := strings.Split(strings.TrimSpace(stdout.String()), "\t")
	if len(parts) != 4 {
		return ContextInfo{}, nil
	}

	return ContextInfo{
		Session: parts[0],
		Window:  parts[1],
		Pane:    parts[2],
		PaneID:  parts[3],
	}, nil
}
