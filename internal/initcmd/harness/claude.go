package harness

import (
	"fmt"
	"os/exec"
	"strings"
)

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

func (c *Claude) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Scaffolding is orchestrated by internal/initcmd/scaffold package
	return nil
}

func (c *Claude) SupportsTemperature() bool { return false }
func (c *Claude) SupportsEffort() bool      { return true }
