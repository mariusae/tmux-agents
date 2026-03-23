package hook

import (
	"fmt"
	"os"
	"strings"
)

type Resolved struct {
	Provider          string
	ProviderSessionID string
	Kind              string
	Message           string
}

func Resolve(provider, rawEvent, providerSessionID, message string) (Resolved, error) {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return Resolved{}, err
	}

	kind, err := resolveKind(normalizedProvider, rawEvent)
	if err != nil {
		return Resolved{}, err
	}

	sessionID := strings.TrimSpace(providerSessionID)
	if sessionID == "" {
		sessionID = defaultSessionID(normalizedProvider)
	}

	resolvedMessage := strings.TrimSpace(message)
	if resolvedMessage == "" {
		resolvedMessage = fmt.Sprintf("%s hook %s", normalizedProvider, strings.TrimSpace(rawEvent))
	}

	return Resolved{
		Provider:          normalizedProvider,
		ProviderSessionID: sessionID,
		Kind:              kind,
		Message:           resolvedMessage,
	}, nil
}

func normalizeProvider(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "claude":
		return "claude", nil
	case "codex":
		return "codex", nil
	default:
		return "", fmt.Errorf("unknown hook provider %q", raw)
	}
}

func resolveKind(provider, rawEvent string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(rawEvent))

	switch provider {
	case "claude":
		switch normalized {
		case "userpromptsubmit", "prompt", "submit":
			return "prompt_submitted", nil
		case "pretooluse", "tool_start", "tool_started":
			return "tool_started", nil
		case "posttooluse", "tool_finish", "tool_finished":
			return "tool_finished", nil
		case "stop", "done", "complete", "completed":
			return "state_idle", nil
		case "notification":
			return "notification", nil
		default:
			return "", fmt.Errorf("unknown claude hook event %q", rawEvent)
		}
	case "codex":
		switch normalized {
		case "notify", "notification", "done", "complete", "completed":
			return "notification", nil
		case "prompt", "submit":
			return "prompt_submitted", nil
		case "start", "resume", "running":
			return "state_running", nil
		default:
			return "", fmt.Errorf("unknown codex hook event %q", rawEvent)
		}
	default:
		return "", fmt.Errorf("unknown hook provider %q", provider)
	}
}

func defaultSessionID(provider string) string {
	if value := strings.TrimSpace(os.Getenv("TMUX_AGENTS_SESSION_ID")); value != "" {
		return value
	}

	for _, key := range sessionEnvKeys(provider) {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

func sessionEnvKeys(provider string) []string {
	switch provider {
	case "claude":
		return []string{
			"CLAUDE_SESSION_ID",
			"CLAUDE_CONVERSATION_ID",
			"ANTHROPIC_SESSION_ID",
		}
	case "codex":
		return []string{
			"CODEX_SESSION_ID",
			"OPENAI_CODEX_SESSION_ID",
			"OPENAI_SESSION_ID",
		}
	default:
		return nil
	}
}
