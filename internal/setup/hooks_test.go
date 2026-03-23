package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUninstallHooks(t *testing.T) {
	t.Setenv("TMUX_AGENTS_CLAUDE_SETTINGS_PATH", filepath.Join(t.TempDir(), "claude", "settings.json"))
	t.Setenv("TMUX_AGENTS_CODEX_CONFIG_PATH", filepath.Join(t.TempDir(), "codex", "config.toml"))

	report, err := InstallHooks(context.Background())
	if err != nil {
		t.Fatalf("InstallHooks returned error: %v", err)
	}
	if !report.AnyChanged() {
		t.Fatal("expected install to change config files")
	}

	claudeData, err := os.ReadFile(report.Changes[0].Path)
	if err != nil {
		t.Fatalf("read claude settings: %v", err)
	}
	if !strings.Contains(string(claudeData), "tmux-agents") {
		t.Fatal("expected claude settings to contain tmux-agents hook command")
	}

	codexData, err := os.ReadFile(report.Changes[1].Path)
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	if !strings.Contains(string(codexData), "tmux-agents") {
		t.Fatal("expected codex config to contain tmux-agents notify command")
	}

	uninstallReport, err := UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks returned error: %v", err)
	}
	if !uninstallReport.AnyChanged() {
		t.Fatal("expected uninstall to change config files")
	}

	claudeData, err = os.ReadFile(report.Changes[0].Path)
	if err != nil {
		t.Fatalf("read claude settings after uninstall: %v", err)
	}
	if strings.Contains(string(claudeData), "tmux-agents") {
		t.Fatal("expected claude settings to remove tmux-agents hook command")
	}

	codexData, err = os.ReadFile(report.Changes[1].Path)
	if err != nil {
		t.Fatalf("read codex config after uninstall: %v", err)
	}
	if strings.Contains(string(codexData), "tmux-agents") {
		t.Fatal("expected codex config to remove tmux-agents notify command")
	}
}
