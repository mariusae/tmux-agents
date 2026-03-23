package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/store"
)

func TestStatusLineShowsWaitingAndRecentlyIdleAgents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := store.OpenBolt(filepath.Join(t.TempDir(), "tmux-agents.db"))
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	for _, event := range []model.Event{
		{
			Time:              now.Add(-2 * time.Minute),
			Provider:          "codex",
			ProviderSessionID: "session-1",
			TmuxSession:       "ion",
			TmuxWindow:        "3",
			TmuxPane:          "0",
			Kind:              model.EventKindStateAwaitingInput,
			Source:            model.EventSourceHook,
		},
		{
			Time:              now.Add(-1 * time.Minute),
			Provider:          "claude",
			ProviderSessionID: "session-2",
			TmuxSession:       "proj",
			TmuxWindow:        "1",
			TmuxPane:          "2",
			Kind:              model.EventKindStateAwaitingInput,
			Source:            model.EventSourceHook,
		},
		// Idle agent that transitioned recently — should appear.
		{
			Time:              now.Add(-5 * time.Second),
			Provider:          "codex",
			ProviderSessionID: "session-3",
			TmuxSession:       "misc",
			TmuxWindow:        "7",
			TmuxPane:          "1",
			Kind:              model.EventKindStateRunning,
			Source:            model.EventSourceHook,
		},
		{
			Time:              now,
			Provider:          "codex",
			ProviderSessionID: "session-3",
			TmuxSession:       "misc",
			TmuxWindow:        "7",
			TmuxPane:          "1",
			Kind:              model.EventKindStateIdle,
			Source:            model.EventSourceHook,
		},
		// Idle agent that transitioned long ago — should NOT appear.
		{
			Time:              now.Add(-5 * time.Minute),
			Provider:          "codex",
			ProviderSessionID: "session-4",
			TmuxSession:       "old",
			TmuxWindow:        "0",
			TmuxPane:          "0",
			Kind:              model.EventKindStateIdle,
			Source:            model.EventSourceHook,
		},
	} {
		if _, _, err := st.RecordEvent(ctx, event); err != nil {
			t.Fatalf("RecordEvent returned error: %v", err)
		}
	}

	if err := st.SetMeta(ctx, "last_reconcile_at", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMeta returned error: %v", err)
	}

	application := &App{store: st}
	line, err := application.StatusLine(ctx)
	if err != nil {
		t.Fatalf("StatusLine returned error: %v", err)
	}

	want := "❯ion:3.0 ❯proj:1.2 ○misc:7.1 █ "
	if line != want {
		t.Fatalf("StatusLine() = %q, want %q", line, want)
	}
}

func TestStatusLineEmptyWhenNothingWaiting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := store.OpenBolt(filepath.Join(t.TempDir(), "tmux-agents.db"))
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if _, _, err := st.RecordEvent(ctx, model.Event{
		Time:              now,
		Provider:          "codex",
		ProviderSessionID: "session-1",
		TmuxSession:       "ion",
		TmuxWindow:        "3",
		TmuxPane:          "0",
		Kind:              model.EventKindStateRunning,
		Source:            model.EventSourceHook,
	}); err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}
	if err := st.SetMeta(ctx, "last_reconcile_at", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMeta returned error: %v", err)
	}

	application := &App{store: st}
	line, err := application.StatusLine(ctx)
	if err != nil {
		t.Fatalf("StatusLine returned error: %v", err)
	}

	if line != "" {
		t.Fatalf("StatusLine() = %q, want empty string", line)
	}
}
