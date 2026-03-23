package model

import "testing"

func TestAgentLabelUsesTmuxTargetSyntax(t *testing.T) {
	t.Parallel()

	agent := Agent{
		Provider:    "codex",
		TmuxSession: "ion",
		TmuxWindow:  "3",
		TmuxPane:    "0",
	}

	if got := agent.Label(); got != "codex@ion:3.0" {
		t.Fatalf("expected tmux target label, got %q", got)
	}
}

func TestAgentLabelFallsBackWithoutTarget(t *testing.T) {
	t.Parallel()

	agent := Agent{
		Provider:          "codex",
		ProviderSessionID: "session-1",
		TmuxSession:       "ion",
		TmuxWindow:        "3",
		TmuxPane:          "%173",
	}

	if got := agent.Label(); got != "codex/session-1" {
		t.Fatalf("expected provider-session fallback, got %q", got)
	}
}
