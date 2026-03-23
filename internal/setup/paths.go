package setup

import (
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	ClaudeSettings string
	CodexConfig    string
}

func ResolvePaths() (Paths, error) {
	if claude := strings.TrimSpace(os.Getenv("TMUX_AGENTS_CLAUDE_SETTINGS_PATH")); claude != "" {
		return Paths{
			ClaudeSettings: claude,
			CodexConfig:    codexConfigPath(),
		}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		ClaudeSettings: filepath.Join(home, ".claude", "settings.json"),
		CodexConfig:    codexConfigPath(),
	}, nil
}

func codexConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("TMUX_AGENTS_CODEX_CONFIG_PATH")); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex/config.toml"
	}
	return filepath.Join(home, ".codex", "config.toml")
}
