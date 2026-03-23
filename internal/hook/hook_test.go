package hook

import "testing"

func TestResolveClaudeStop(t *testing.T) {
	t.Parallel()

	resolved, err := Resolve("claude", "Stop", "claude-session", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Kind != "notification" {
		t.Fatalf("expected notification kind, got %q", resolved.Kind)
	}
	if resolved.Provider != "claude" {
		t.Fatalf("expected claude provider, got %q", resolved.Provider)
	}
}

func TestResolveCodexNotifyUsesEnvSessionID(t *testing.T) {
	t.Setenv("CODEX_SESSION_ID", "codex-session")

	resolved, err := Resolve("codex", "notify", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.ProviderSessionID != "codex-session" {
		t.Fatalf("expected env-derived session id, got %q", resolved.ProviderSessionID)
	}
	if resolved.Kind != "notification" {
		t.Fatalf("expected notification kind, got %q", resolved.Kind)
	}
}

func TestResolveUnknownEventFails(t *testing.T) {
	t.Parallel()

	if _, err := Resolve("codex", "bogus", "", ""); err == nil {
		t.Fatal("expected unknown hook event to fail")
	}
}
