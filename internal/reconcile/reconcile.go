package reconcile

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/process"
	"github.com/mariusae/tmux-agents/internal/store"
	"github.com/mariusae/tmux-agents/internal/tmux"
)

type Result struct {
	Seen    int
	Updated int
	Missing int
}

func Run(ctx context.Context, st store.Store) (Result, error) {
	now := time.Now().UTC()

	panes, err := tmux.ListPanes(ctx)
	if err != nil {
		return Result{}, err
	}

	agents, err := st.ListAgents(ctx)
	if err != nil {
		return Result{}, err
	}

	agentByKey := make(map[string]model.Agent, len(agents))
	for _, agent := range agents {
		agentByKey[agent.Key] = agent
	}

	result := Result{}
	liveKeys := make(map[string]struct{})

	for _, pane := range panes {
		liveAgent, ok := detectLiveAgent(ctx, pane)
		if !ok {
			continue
		}

		result.Seen++
		liveKeys[liveAgent.AgentKey()] = struct{}{}

		existing, exists := agentByKey[liveAgent.AgentKey()]
		if !needsLiveUpdate(existing, liveAgent, exists) {
			continue
		}

		if _, _, err := st.RecordEvent(ctx, liveAgent); err != nil {
			return result, err
		}
		result.Updated++
	}

	for _, agent := range agents {
		if agent.ReconcileSource == "" || agent.State == model.AgentStateGone {
			continue
		}
		if _, ok := liveKeys[agent.Key]; ok {
			continue
		}

		event := model.Event{
			Time:              now,
			Provider:          agent.Provider,
			ProviderSessionID: agent.ProviderSessionID,
			TmuxSession:       agent.TmuxSession,
			TmuxWindow:        agent.TmuxWindow,
			TmuxPane:          agent.TmuxPane,
			Kind:              model.EventKindLiveMissing,
			Message:           "agent no longer detected in live tmux scan",
			Source:            model.EventSourceReconcile,
		}
		if _, _, err := st.RecordEvent(ctx, event); err != nil {
			return result, err
		}
		result.Missing++
	}

	if err := st.SetMeta(ctx, "last_reconcile_at", now.Format(time.RFC3339Nano)); err != nil {
		return result, err
	}

	return result, nil
}

func detectLiveAgent(ctx context.Context, pane tmux.Pane) (model.Event, bool) {
	commands, err := process.DescendantCommands(ctx, pane.PanePID)
	if err != nil {
		return model.Event{}, false
	}

	provider := detectProvider(pane.CurrentCommand, commands)
	if provider == "" {
		return model.Event{}, false
	}

	tail, err := tmux.CapturePaneTail(ctx, pane.PaneID, 40)
	if err != nil {
		return model.Event{}, false
	}

	kind, message := classifyLiveState(provider, tail)

	return model.Event{
		Time:              time.Now().UTC(),
		Provider:          provider,
		ProviderSessionID: syntheticSessionID(pane),
		TmuxSession:       pane.Session,
		TmuxWindow:        pane.Window,
		TmuxPane:          pane.Pane,
		Kind:              kind,
		Message:           message,
		Source:            model.EventSourceReconcile,
	}, true
}

func detectProvider(currentCommand string, commands []string) string {
	candidates := make([]string, 0, len(commands)+1)
	if currentCommand != "" {
		candidates = append(candidates, currentCommand)
	}
	candidates = append(candidates, commands...)

	for _, candidate := range candidates {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(candidate)))
		switch {
		case strings.Contains(base, "codex"):
			return "codex"
		case strings.Contains(base, "claude"):
			return "claude"
		}
	}

	return ""
}

func syntheticSessionID(pane tmux.Pane) string {
	return "pane:" + pane.PaneID
}

func needsLiveUpdate(existing model.Agent, liveEvent model.Event, exists bool) bool {
	if !exists {
		return true
	}
	if existing.State != expectedStateForEvent(liveEvent.Kind) || !existing.Live {
		return true
	}
	if existing.TmuxSession != liveEvent.TmuxSession || existing.TmuxWindow != liveEvent.TmuxWindow || existing.TmuxPane != liveEvent.TmuxPane {
		return true
	}
	return false
}

func classifyLiveState(provider, tail string) (model.EventKind, string) {
	switch provider {
	case "codex":
		if codexLooksAwaitingInput(tail) {
			return model.EventKindStateAwaitingInput, "codex detected and awaiting user input"
		}
		if codexLooksRunning(tail) {
			return model.EventKindStateRunning, "codex detected and actively working"
		}
		return model.EventKindStateIdle, "codex detected without an active or waiting prompt"
	default:
		return model.EventKindLiveDetected, "agent detected in live tmux scan"
	}
}

func codexLooksAwaitingInput(tail string) bool {
	const waitingPrompt = "press enter to confirm or esc to cancel"

	for _, line := range lastNonEmptyLines(tail, 3) {
		normalized := strings.ToLower(strings.TrimSpace(line))
		if normalized == waitingPrompt {
			return true
		}
	}
	return false
}

func codexLooksRunning(tail string) bool {
	for _, line := range lastNonEmptyLines(tail, 8) {
		normalized := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(normalized, "esc to interrupt") {
			return true
		}
		if strings.HasPrefix(normalized, "• working") {
			return true
		}
	}
	return false
}

func lastNonEmptyLines(text string, limit int) []string {
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, limit)
	for i := len(rawLines) - 1; i >= 0 && len(lines) < limit; i-- {
		line := strings.TrimSpace(rawLines[i])
		if line == "" {
			continue
		}
		lines = append(lines, rawLines[i])
	}

	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}
	return lines
}

func expectedStateForEvent(kind model.EventKind) model.AgentState {
	switch kind {
	case model.EventKindStateAwaitingInput:
		return model.AgentStateAwaitingInput
	case model.EventKindStateIdle:
		return model.AgentStateIdle
	case model.EventKindLiveMissing, model.EventKindPaneClosed, model.EventKindStateGone:
		return model.AgentStateGone
	default:
		return model.AgentStateRunning
	}
}
