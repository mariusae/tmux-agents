package cli

import (
	"bytes"
	"context"
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

func TestRunStatusWithDelimiter(t *testing.T) {
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
	if code := Run(ctx, []string{"status", "-d", " • "}, &stdout, &stderr); code != 0 {
		t.Fatalf("runStatus() code = %d, stderr = %q", code, stderr.String())
	}

	if got := stdout.String(); got != "❯ion:3.0 • \n" {
		t.Fatalf("runStatus() output = %q, want %q", got, "❯ion:3.0 • \n")
	}
}

func TestRunStatusWithoutWaitingAgentsPrintsEmptyLine(t *testing.T) {
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
	if code := Run(ctx, []string{"status", "-d", " • "}, &stdout, &stderr); code != 0 {
		t.Fatalf("runStatus() code = %d, stderr = %q", code, stderr.String())
	}

	if got := stdout.String(); got != "\n" {
		t.Fatalf("runStatus() output = %q, want empty line", got)
	}
}
