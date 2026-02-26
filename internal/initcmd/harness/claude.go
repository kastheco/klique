package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const claudeEnforceCLIToolsScript = `#!/bin/bash
# PreToolUse hook: block legacy CLI tools, enforce modern replacements.
# Installed by kasmos init. Source of truth: cli-tools skill.
# Reads Bash tool_input.command from stdin JSON and rejects banned commands.

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

[ -z "$COMMAND" ] && exit 0

# grep -> rg (ripgrep)
# Word-boundary match avoids false positives (e.g. ast-grep)
if echo "$COMMAND" | grep -qP '(^|[|;&\x60]\s*|\$\(\s*)\bgrep\b'; then
  echo "BLOCKED: 'grep' is banned. Use 'rg' (ripgrep) instead. rg is faster, respects .gitignore, and has better defaults." >&2
  exit 2
fi

# sed -> sd or comby
if echo "$COMMAND" | grep -qP '(^|[|;&\x60]\s*|\$\(\s*)\bsed\b'; then
  echo "BLOCKED: 'sed' is banned. Use 'sd' for simple replacements or 'comby' for structural/multi-line rewrites." >&2
  exit 2
fi

# awk -> yq/jq, sd, or comby
if echo "$COMMAND" | grep -qP '(^|[|;&\x60]\s*|\$\(\s*)\bawk\b'; then
  echo "BLOCKED: 'awk' is banned. Use 'yq'/'jq' for structured data, 'sd' for text, or 'comby' for code patterns." >&2
  exit 2
fi

# standalone diff (not git diff) -> difft
if echo "$COMMAND" | grep -qP '(^|[|;&\x60]\s*|\$\(\s*)\bdiff\b' && \
   ! echo "$COMMAND" | grep -qP '\bgit\s+diff\b'; then
  echo "BLOCKED: standalone 'diff' is banned. Use 'difft' (difftastic) for syntax-aware structural diffs. 'git diff' is allowed." >&2
  exit 2
fi

# wc -l -> scc
if echo "$COMMAND" | grep -qP '\bwc\s+(-\w*l|--lines)\b|\bwc\b.*\s-l\b'; then
  echo "BLOCKED: 'wc -l' is banned. Use 'scc' for language-aware line counts with complexity estimates." >&2
  exit 2
fi

exit 0
`

// Claude implements Harness for the Claude Code CLI.
type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) Detect() (string, bool) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels returns the static set of Claude models.
func (c *Claude) ListModels() ([]string, error) {
	return []string{
		"claude-sonnet-4-6",
		"claude-opus-4-6",
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
	}, nil
}

func (c *Claude) BuildFlags(agent AgentConfig) []string {
	var flags []string
	if agent.Model != "" {
		flags = append(flags, "--model", agent.Model)
	}
	if agent.Effort != "" {
		flags = append(flags, "--effort", agent.Effort)
	}
	flags = append(flags, agent.ExtraFlags...)
	return flags
}

func (c *Claude) InstallSuperpowers() error {
	// Check if already installed
	out, err := exec.Command("claude", "plugin", "list").Output()
	if err == nil && strings.Contains(string(out), "superpowers") {
		return nil // already installed
	}

	// Add marketplace
	cmd := exec.Command("claude", "plugin", "marketplace", "add", "obra/superpowers-marketplace")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add marketplace: %s: %w", string(out), err)
	}

	// Install plugin
	cmd = exec.Command("claude", "plugin", "install", "superpowers@superpowers-marketplace")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install plugin: %s: %w", string(out), err)
	}

	return nil
}

func (c *Claude) InstallEnforcement() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	hooksDir := filepath.Join(home, ".claude", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create claude hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "enforce-cli-tools.sh")
	if err := os.WriteFile(hookPath, []byte(claudeEnforceCLIToolsScript), 0o755); err != nil {
		return fmt.Errorf("write claude enforcement hook: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	settingsRaw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read claude settings: %w", err)
		}
		settingsRaw = []byte(`{"hooks":{}}`)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsRaw, &settings); err != nil {
		return fmt.Errorf("parse claude settings: %w", err)
	}

	hooksVal, ok := settings["hooks"]
	if !ok {
		hooksVal = map[string]any{}
	}

	hooks, ok := hooksVal.(map[string]any)
	if !ok {
		return fmt.Errorf("claude settings hooks has unexpected type %T", hooksVal)
	}

	preToolUseVal, ok := hooks["PreToolUse"]
	if !ok {
		preToolUseVal = []any{}
	}

	preToolUse, ok := preToolUseVal.([]any)
	if !ok {
		return fmt.Errorf("claude settings hooks.PreToolUse has unexpected type %T", preToolUseVal)
	}

	if !hasClaudeEnforcementHook(preToolUse) {
		preToolUse = append(preToolUse, map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": hookPath,
				},
			},
		})
	}

	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks

	merged, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, merged, 0o644); err != nil {
		return fmt.Errorf("write claude settings: %w", err)
	}

	return nil
}

func hasClaudeEnforcementHook(preToolUse []any) bool {
	for _, entry := range preToolUse {
		group, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		hooks, ok := group["hooks"].([]any)
		if !ok {
			continue
		}

		for _, hook := range hooks {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				continue
			}

			command, _ := hookMap["command"].(string)
			if strings.Contains(command, "enforce-cli-tools.sh") {
				return true
			}
		}
	}

	return false
}

func (c *Claude) SupportsTemperature() bool { return false }
func (c *Claude) SupportsEffort() bool      { return true }

func (c *Claude) ListEffortLevels(_ string) []string {
	return []string{"", "low", "medium", "high", "max"}
}
