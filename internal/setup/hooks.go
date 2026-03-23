package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type FileChange struct {
	Path    string
	Diff    string
	Changed bool
}

type HookReport struct {
	Changes []FileChange
}

func (r HookReport) AnyChanged() bool {
	for _, change := range r.Changes {
		if change.Changed {
			return true
		}
	}
	return false
}

func InstallHooks(ctx context.Context) (HookReport, error) {
	_ = ctx
	return updateHooks(true)
}

func UninstallHooks(ctx context.Context) (HookReport, error) {
	_ = ctx
	return updateHooks(false)
}

func updateHooks(install bool) (HookReport, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return HookReport{}, err
	}

	exe := commandName()

	claudeChange, err := updateClaudeHooks(paths.ClaudeSettings, exe, install)
	if err != nil {
		return HookReport{}, err
	}
	codexChange, err := updateCodexHooks(paths.CodexConfig, exe, install)
	if err != nil {
		return HookReport{}, err
	}

	return HookReport{Changes: []FileChange{claudeChange, codexChange}}, nil
}

func commandName() string {
	if value := strings.TrimSpace(os.Getenv("TMUX_AGENTS_COMMAND")); value != "" {
		return value
	}
	return "tmux-agents"
}

func updateClaudeHooks(path, executable string, install bool) (FileChange, error) {
	before, mode, err := readFileOrEmpty(path)
	if err != nil {
		return FileChange{}, err
	}

	root := map[string]any{}
	if len(bytes.TrimSpace(before)) > 0 {
		if err := json.Unmarshal(before, &root); err != nil {
			return FileChange{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	rawHooks, ok := root["hooks"].(map[string]any)
	if !ok {
		rawHooks = map[string]any{}
	}

	changed := false
	for event, command := range claudeHookCommands(executable) {
		current, _ := rawHooks[event].([]any)
		next, itemChanged := updateClaudeEventHooks(current, command, install)
		if itemChanged {
			changed = true
		}
		if len(next) == 0 {
			delete(rawHooks, event)
		} else {
			rawHooks[event] = next
		}
	}

	if len(rawHooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = rawHooks
	}

	after, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return FileChange{}, err
	}
	if len(after) > 0 {
		after = append(after, '\n')
	}

	diff, err := unifiedDiff(path, before, after)
	if err != nil {
		return FileChange{}, err
	}

	if changed && diff != "" {
		if err := writeFileAtomic(path, after, mode); err != nil {
			return FileChange{}, err
		}
	}

	return FileChange{Path: path, Diff: diff, Changed: changed && diff != ""}, nil
}

func claudeHookCommands(executable string) map[string]string {
	return map[string]string{
		"UserPromptSubmit": shellJoin(executable, "hook", "claude", "UserPromptSubmit"),
		"PreToolUse":       shellJoin(executable, "hook", "claude", "PreToolUse"),
		"Stop":             shellJoin(executable, "hook", "claude", "Stop"),
		"Notification":     shellJoin(executable, "hook", "claude", "Notification"),
	}
}

func updateClaudeEventHooks(entries []any, command string, install bool) ([]any, bool) {
	if len(entries) == 0 && !install {
		return nil, false
	}

	changed := false
	out := make([]any, 0, len(entries)+1)
	foundExact := false

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}

		rawNested, ok := entryMap["hooks"].([]any)
		if !ok {
			out = append(out, entry)
			continue
		}

		nextNested := make([]any, 0, len(rawNested))
		matchedManaged := false
		for _, rawHook := range rawNested {
			hookMap, ok := rawHook.(map[string]any)
			if !ok {
				nextNested = append(nextNested, rawHook)
				continue
			}

			commandValue, _ := hookMap["command"].(string)
			if isManagedClaudeCommand(commandValue) {
				matchedManaged = true
				if install && commandValue == command && !foundExact {
					foundExact = true
					nextNested = append(nextNested, rawHook)
				} else {
					changed = true
				}
				continue
			}

			nextNested = append(nextNested, rawHook)
		}

		if matchedManaged && len(nextNested) == 0 {
			changed = true
			continue
		}

		if matchedManaged {
			entryMap["hooks"] = nextNested
			out = append(out, entryMap)
			continue
		}

		out = append(out, entry)
	}

	if install && !foundExact {
		out = append(out, map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": command,
				},
			},
		})
		changed = true
	}

	return out, changed
}

func isManagedClaudeCommand(command string) bool {
	return strings.Contains(normalizeCommandText(command), "tmux-agents hook claude ")
}

func updateCodexHooks(path, executable string, install bool) (FileChange, error) {
	before, mode, err := readFileOrEmpty(path)
	if err != nil {
		return FileChange{}, err
	}

	root := map[string]any{}
	if len(bytes.TrimSpace(before)) > 0 {
		if err := toml.Unmarshal(before, &root); err != nil {
			return FileChange{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	command := shellJoin(executable, "hook", "codex", "notify")
	notify := coerceStringSlice(root["notify"])
	filtered := notify[:0]
	foundExact := false
	changed := false
	for _, existing := range notify {
		if strings.Contains(normalizeCommandText(existing), "tmux-agents hook codex notify") {
			if install && existing == command && !foundExact {
				filtered = append(filtered, existing)
				foundExact = true
			} else {
				changed = true
			}
			continue
		}
		filtered = append(filtered, existing)
	}

	if len(filtered) != len(notify) {
		changed = true
	}
	notify = slices.Clone(filtered)
	if install && !foundExact {
		notify = append(notify, command)
		changed = true
	}
	if !install && len(notify) != len(filtered) {
		changed = true
	}

	if len(notify) == 0 {
		delete(root, "notify")
	} else {
		root["notify"] = notify
	}

	after, err := toml.Marshal(root)
	if err != nil {
		return FileChange{}, err
	}

	diff, err := unifiedDiff(path, before, after)
	if err != nil {
		return FileChange{}, err
	}

	if changed && diff != "" {
		if err := writeFileAtomic(path, after, mode); err != nil {
			return FileChange{}, err
		}
	}

	return FileChange{Path: path, Diff: diff, Changed: changed && diff != ""}, nil
}

func coerceStringSlice(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return slices.Clone(value)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func shellJoin(args ...string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if isShellSafe(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func normalizeCommandText(command string) string {
	replacer := strings.NewReplacer("'", "", "\"", "")
	return strings.Join(strings.Fields(replacer.Replace(command)), " ")
}

func isShellSafe(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("._:/-@", r):
		default:
			return false
		}
	}
	return true
}
