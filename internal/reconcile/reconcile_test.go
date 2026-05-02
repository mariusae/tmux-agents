package reconcile

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/store"
)

func TestCodexLooksAwaitingInput(t *testing.T) {
	t.Parallel()

	tail := "\n› 1. Yes, proceed (y)\n  2. Yes, and don't ask again for these files (a)\n  3. No, and tell Codex what to do differently (esc)\n\nPress enter to confirm or esc to cancel\n"
	if !codexLooksAwaitingInput(tail) {
		t.Fatal("expected exact waiting prompt near the bottom to be detected")
	}
}

func TestCodexLooksAwaitingInputFalseWhenNotNearBottom(t *testing.T) {
	t.Parallel()

	tail := "\nPress enter to confirm or esc to cancel\n\nsome later output\nstill later output\nbottom line\n"
	if codexLooksAwaitingInput(tail) {
		t.Fatal("expected waiting prompt away from the bottom to be ignored")
	}
}

func TestCodexLooksRunning(t *testing.T) {
	t.Parallel()

	tail := "\n• Working (47s • esc to interrupt)\n\n› Explain this codebase\n\n  gpt-5.4 high · 82% left · ~/src/project\n"
	if !codexLooksRunning(tail) {
		t.Fatal("expected running codex pane to be detected as running")
	}
}

func TestCodexLooksRunningFalseForWaitingPrompt(t *testing.T) {
	t.Parallel()

	tail := "\n› 1. Yes, proceed (y)\n  2. Yes, and don't ask again for these files (a)\n  3. No, and tell Codex what to do differently (esc)\n\nPress enter to confirm or esc to cancel\n"
	if codexLooksRunning(tail) {
		t.Fatal("expected waiting codex pane to be detected as awaiting input")
	}
}

func TestClassifyLiveStateForCodexIdleFallback(t *testing.T) {
	t.Parallel()

	kind, _ := classifyLiveState("codex", "\nsome transcript text\nno footer match here\n")
	if kind != model.EventKindStateIdle {
		t.Fatalf("expected idle fallback, got %q", kind)
	}
}

func TestApplyMarksHookAgentMissingWhenPaneGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	recordTestEvent(t, ctx, st, model.Event{
		Time:              now.Add(-time.Minute),
		Provider:          "codex",
		ProviderSessionID: "session-1",
		TmuxSession:       "dead",
		TmuxWindow:        "0",
		TmuxPane:          "0",
		TmuxPaneID:        "%42",
		Kind:              model.EventKindStateAwaitingInput,
		Source:            model.EventSourceHook,
	})

	result, err := Apply(ctx, st, Snapshot{
		CapturedAt: now,
		LivePanes:  map[string]bool{},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Missing != 1 {
		t.Fatalf("Apply marked %d missing agents, want 1", result.Missing)
	}

	agent := onlyAgent(t, ctx, st, "codex:session-1")
	if agent.Live {
		t.Fatal("expected hook agent to be marked not live")
	}
	if agent.State != model.AgentStateGone {
		t.Fatalf("agent state = %q, want %q", agent.State, model.AgentStateGone)
	}
}

func TestApplyMarksHookAgentMissingWhenSessionGoneWithoutPaneID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	recordTestEvent(t, ctx, st, model.Event{
		Time:              now.Add(-time.Minute),
		Provider:          "codex",
		ProviderSessionID: "session-1",
		TmuxSession:       "dead",
		TmuxWindow:        "0",
		TmuxPane:          "0",
		Kind:              model.EventKindStateAwaitingInput,
		Source:            model.EventSourceHook,
	})

	result, err := Apply(ctx, st, Snapshot{
		CapturedAt:   now,
		LivePanes:    map[string]bool{"%1": true},
		LiveSessions: map[string]bool{"other": true},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Missing != 1 {
		t.Fatalf("Apply marked %d missing agents, want 1", result.Missing)
	}

	agent := onlyAgent(t, ctx, st, "codex:session-1")
	if agent.Live {
		t.Fatal("expected hook agent to be marked not live")
	}
}

func TestApplyMarksHookAgentMissingWhenCheckedPaneHasNoAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	recordTestEvent(t, ctx, st, model.Event{
		Time:              now.Add(-time.Minute),
		Provider:          "claude",
		ProviderSessionID: "session-1",
		TmuxSession:       "work",
		TmuxWindow:        "1",
		TmuxPane:          "0",
		TmuxPaneID:        "%7",
		Kind:              model.EventKindStateRunning,
		Source:            model.EventSourceHook,
	})

	result, err := Apply(ctx, st, Snapshot{
		CapturedAt:   now,
		LivePanes:    map[string]bool{"%7": true},
		CheckedPanes: map[string]bool{"%7": true},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Missing != 1 {
		t.Fatalf("Apply marked %d missing agents, want 1", result.Missing)
	}

	agent := onlyAgent(t, ctx, st, "claude:session-1")
	if agent.Live {
		t.Fatal("expected hook agent to be marked not live")
	}
}

func TestApplyKeepsHookAgentWhenSameProviderIsLiveInSamePane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	recordTestEvent(t, ctx, st, model.Event{
		Time:              now.Add(-time.Minute),
		Provider:          "codex",
		ProviderSessionID: "real-session",
		TmuxSession:       "work",
		TmuxWindow:        "1",
		TmuxPane:          "0",
		TmuxPaneID:        "%9",
		Kind:              model.EventKindStateAwaitingInput,
		Source:            model.EventSourceHook,
	})

	result, err := Apply(ctx, st, Snapshot{
		CapturedAt: now,
		LivePanes:  map[string]bool{"%9": true},
		LiveEvents: []model.Event{
			{
				Time:              now,
				Provider:          "codex",
				ProviderSessionID: "pane:%9",
				TmuxSession:       "work",
				TmuxWindow:        "1",
				TmuxPane:          "0",
				TmuxPaneID:        "%9",
				Kind:              model.EventKindStateIdle,
				Source:            model.EventSourceReconcile,
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Missing != 0 {
		t.Fatalf("Apply marked %d missing agents, want 0", result.Missing)
	}

	agent := onlyAgent(t, ctx, st, "codex:real-session")
	if !agent.Live {
		t.Fatal("expected hook agent to remain live")
	}
}

func openTestStore(t *testing.T) *store.BoltStore {
	t.Helper()

	st, err := store.OpenBolt(filepath.Join(t.TempDir(), "tmux-agents.db"))
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}
	return st
}

func recordTestEvent(t *testing.T, ctx context.Context, st store.Store, event model.Event) {
	t.Helper()

	if _, _, err := st.RecordEvent(ctx, event); err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}
}

func onlyAgent(t *testing.T, ctx context.Context, st store.Store, key string) model.Agent {
	t.Helper()

	agents, err := st.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	for _, agent := range agents {
		if agent.Key == key {
			return agent
		}
	}
	t.Fatalf("agent %q not found", key)
	return model.Agent{}
}
