package reconcile

import (
	"testing"

	"github.com/mariusae/tmux-agents/internal/model"
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
