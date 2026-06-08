package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/store"
)

func disableLiveTmux(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
}

func TestFormatShowTime(t *testing.T) {
	loc := time.FixedZone("test", -5*60*60)
	now := time.Date(2026, time.March, 23, 12, 0, 0, 0, loc)

	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{name: "zero", when: time.Time{}, want: "-"},
		{name: "just now", when: now.Add(-20 * time.Second), want: "just now"},
		{name: "last minute", when: now.Add(-70 * time.Second), want: "last minute"},
		{name: "minutes", when: now.Add(-2 * time.Minute), want: "2min"},
		{name: "hours", when: now.Add(-1 * time.Hour), want: "1h"},
		{name: "yesterday", when: now.Add(-24 * time.Hour), want: "yesterday"},
		{name: "same year", when: time.Date(2026, time.March, 3, 9, 0, 0, 0, loc), want: "Tue03"},
		{name: "older year", when: time.Date(2025, time.February, 19, 9, 0, 0, 0, loc), want: "19Feb25"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatShowTime(now, tc.when); got != tc.want {
				t.Fatalf("formatShowTime() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunListOutputsToolTargetsInInboxOrder(t *testing.T) {
	ctx := context.Background()
	disableLiveTmux(t)

	dbPath := filepath.Join(t.TempDir(), "tmux-agents.db")
	st, err := store.OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}

	now := time.Now().UTC()
	events := []model.Event{
		{
			Time:              now.Add(-10 * time.Minute),
			Provider:          "codex",
			ProviderSessionID: "older",
			TmuxSession:       "work",
			TmuxWindow:        "2",
			TmuxWindowName:    "api",
			TmuxPane:          "0",
			TmuxPaneID:        "%20",
			Kind:              model.EventKindStateRunning,
			Source:            model.EventSourceHook,
		},
		{
			Time:              now.Add(-2 * time.Minute),
			Provider:          "claude",
			ProviderSessionID: "newer",
			TmuxSession:       "work",
			TmuxWindow:        "1",
			TmuxWindowName:    "app",
			TmuxPane:          "1",
			TmuxPaneID:        "%12",
			Kind:              model.EventKindStateRunning,
			Source:            model.EventSourceHook,
		},
		{
			Time:              now.Add(-1 * time.Minute),
			Provider:          "claude",
			ProviderSessionID: "newer",
			TmuxSession:       "work",
			TmuxWindow:        "1",
			TmuxWindowName:    "app",
			TmuxPane:          "1",
			TmuxPaneID:        "%12",
			Kind:              model.EventKindStateAwaitingInput,
			Source:            model.EventSourceHook,
		},
	}
	for _, event := range events {
		if _, _, err := st.RecordEvent(ctx, event); err != nil {
			t.Fatalf("RecordEvent returned error: %v", err)
		}
	}
	if err := st.SetMeta(ctx, "last_reconcile_at", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMeta returned error: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	t.Setenv("TMUX_AGENTS_DB_PATH", dbPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(ctx, []string{"list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(list) code = %d, stderr = %q", code, stderr.String())
	}

	var got []struct {
		Title             string `json:"title"`
		Target            string `json:"target"`
		TmuxPaneID        string `json:"tmux_pane_id"`
		TmuxSession       string `json:"tmux_session"`
		TmuxWindow        string `json:"tmux_window"`
		TmuxWindowName    string `json:"tmux_window_name"`
		TmuxPane          string `json:"tmux_pane"`
		Provider          string `json:"provider"`
		ProviderSessionID string `json:"provider_session_id"`
		State             string `json:"state"`
		AwaitingInput     bool   `json:"awaiting_input"`
		Live              bool   `json:"live"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("list output is not JSON: %v\n%s", err, stdout.String())
	}
	if len(got) != 2 {
		t.Fatalf("list returned %d agents, want 2: %#v", len(got), got)
	}

	first := got[0]
	if first.Title != "claude@work:app.1" {
		t.Fatalf("first title = %q, want inbox label %q", first.Title, "claude@work:app.1")
	}
	if first.Target != "%12" || first.TmuxPaneID != "%12" {
		t.Fatalf("first tmux target = %q/%q, want %%12", first.Target, first.TmuxPaneID)
	}
	if first.TmuxSession != "work" || first.TmuxWindow != "1" || first.TmuxWindowName != "app" || first.TmuxPane != "1" {
		t.Fatalf("first tmux location = %#v, want work:1(app).1", first)
	}
	if first.Provider != "claude" || first.ProviderSessionID != "newer" {
		t.Fatalf("first provider fields = %q/%q, want claude/newer", first.Provider, first.ProviderSessionID)
	}
	if first.State != string(model.AgentStateAwaitingInput) || !first.AwaitingInput || !first.Live {
		t.Fatalf("first state fields = %#v, want awaiting live agent", first)
	}

	if got[1].Title != "codex@work:api.0" {
		t.Fatalf("second title = %q, want older codex agent", got[1].Title)
	}
}

func TestRunStatusOutput(t *testing.T) {
	ctx := context.Background()
	disableLiveTmux(t)

	dbPath := filepath.Join(t.TempDir(), "tmux-agents.db")
	st, err := store.OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}

	now := time.Now().UTC()
	if _, _, err := st.RecordEvent(ctx, model.Event{
		Time:              now,
		Provider:          "codex",
		ProviderSessionID: "session-1",
		TmuxSession:       "ion",
		TmuxWindow:        "3",
		TmuxPane:          "0",
		Kind:              model.EventKindStateAwaitingInput,
		Source:            model.EventSourceHook,
	}); err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}
	if err := st.SetMeta(ctx, "last_reconcile_at", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMeta returned error: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	t.Setenv("TMUX_AGENTS_DB_PATH", dbPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(ctx, []string{"status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runStatus() code = %d, stderr = %q", code, stderr.String())
	}

	want := "❯ion:3.0 █ "
	if got := stdout.String(); got != want {
		t.Fatalf("runStatus() output = %q, want %q", got, want)
	}
}

func TestRunStatusEmptyWhenNoNotableAgents(t *testing.T) {
	ctx := context.Background()
	disableLiveTmux(t)

	dbPath := filepath.Join(t.TempDir(), "tmux-agents.db")
	st, err := store.OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt returned error: %v", err)
	}

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
	if err := st.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	t.Setenv("TMUX_AGENTS_DB_PATH", dbPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(ctx, []string{"status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runStatus() code = %d, stderr = %q", code, stderr.String())
	}

	if got := stdout.String(); got != "" {
		t.Fatalf("runStatus() output = %q, want empty string", got)
	}
}
