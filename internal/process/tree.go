package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Info struct {
	PID     int
	PPID    int
	Command string
}

func DescendantCommands(ctx context.Context, rootPID int) ([]string, error) {
	processes, err := listProcesses(ctx)
	if err != nil {
		return nil, err
	}

	byPID := make(map[int]Info, len(processes))
	children := make(map[int][]Info, len(processes))
	for _, proc := range processes {
		byPID[proc.PID] = proc
		children[proc.PPID] = append(children[proc.PPID], proc)
	}

	root, ok := byPID[rootPID]
	if !ok {
		return nil, nil
	}

	commands := make([]string, 0, 8)
	queue := []Info{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		commands = append(commands, current.Command)
		queue = append(queue, children[current.PID]...)
	}

	return commands, nil
}

func listProcesses(ctx context.Context) ([]Info, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,comm=")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("ps: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	processes := make([]Info, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		processes = append(processes, Info{
			PID:     pid,
			PPID:    ppid,
			Command: fields[2],
		})
	}

	return processes, nil
}
