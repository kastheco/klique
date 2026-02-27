package harness

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// OpenCode implements Harness for the OpenCode CLI.
type OpenCode struct{}

const openCodeEnforceCLIToolsPlugin = `/**
 * CLI-tools enforcement plugin for OpenCode.
 * Installed by kasmos setup. Blocks banned CLI tools and suggests replacements.
 */
export const EnforceCLIToolsPlugin = async ({ client, directory }) => {
  const BANNED = [
    { pattern: /(^|[|;&` + "`" + `]\s*|\$\(\s*)\bgrep\b/, name: "grep", replacement: "rg", reason: "rg is faster, respects .gitignore, and has better defaults" },
    { pattern: /(^|[|;&` + "`" + `]\s*|\$\(\s*)\bsed\b/, name: "sed", replacement: "sd or comby", reason: "sd for simple replacements, comby for structural/multi-line rewrites" },
    { pattern: /(^|[|;&` + "`" + `]\s*|\$\(\s*)\bawk\b/, name: "awk", replacement: "yq/jq, sd, or comby", reason: "yq/jq for structured data, sd for text, comby for code patterns" },
    { pattern: /\bwc\s+(-\w*l|--lines)\b|\bwc\b.*\s-l\b/, name: "wc -l", replacement: "scc", reason: "scc provides language-aware line counts with complexity estimates" },
  ];

  // diff needs special handling (allow git diff)
  const DIFF_PATTERN = /(^|[|;&` + "`" + `]\s*|\$\(\s*)\bdiff\b/;
  const GIT_DIFF_PATTERN = /\bgit\s+diff\b/;

  const checkCommand = (cmd) => {
    for (const { pattern, name, replacement, reason } of BANNED) {
      if (pattern.test(cmd)) {
        return ` + "`" + `BLOCKED: '${name}' is banned. Use '${replacement}' instead. ${reason}.` + "`" + `;
      }
    }
    if (DIFF_PATTERN.test(cmd) && !GIT_DIFF_PATTERN.test(cmd)) {
      return "BLOCKED: standalone 'diff' is banned. Use 'difft' (difftastic) for syntax-aware structural diffs. 'git diff' is allowed.";
    }
    return null;
  };

  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      const cmd = output.args?.command;
      if (!cmd) return;
      const blocked = checkCommand(cmd);
      if (blocked) {
        output.args.command = ` + "`" + `echo '${blocked.replace(/'/g, "'\\''")}' >&2; exit 2` + "`" + `;
      }
    },
  };
};
`

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) Detect() (string, bool) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels shells out to `opencode models` and parses the output line-by-line.
// Caps at 10 seconds to avoid hanging the wizard if opencode is misconfigured.
func (o *OpenCode) ListModels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "models")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("opencode models: %w", err)
	}

	var models []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			models = append(models, line)
		}
	}
	return models, scanner.Err()
}

func (o *OpenCode) BuildFlags(agent AgentConfig) []string {
	// opencode uses project config (opencode.json), not CLI flags for model/temp/effort
	return agent.ExtraFlags
}

func (o *OpenCode) InstallEnforcement() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	pluginPath := filepath.Join(pluginDir, "enforce-cli-tools.js")
	if err := os.WriteFile(pluginPath, []byte(openCodeEnforceCLIToolsPlugin), 0o644); err != nil {
		return fmt.Errorf("write enforcement plugin: %w", err)
	}

	return nil
}

func (o *OpenCode) SupportsTemperature() bool { return true }
func (o *OpenCode) SupportsEffort() bool      { return true }

func (o *OpenCode) ListEffortLevels(model string) []string {
	switch {
	case strings.HasPrefix(model, "anthropic/"):
		return []string{"", "low", "medium", "high", "max"}
	case strings.Contains(model, "codex"):
		return []string{"", "low", "medium", "high", "xhigh"}
	default:
		return []string{"", "low", "medium", "high"}
	}
}
