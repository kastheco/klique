package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Codex implements Harness for the Codex CLI.
type Codex struct{}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) Detect() (string, bool) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels returns a default suggestion. Codex accepts free-text model names.
func (c *Codex) ListModels() ([]string, error) {
	return []string{"gpt-5.3-codex"}, nil
}

func (c *Codex) BuildFlags(agent AgentConfig) []string {
	var flags []string
	if agent.Model != "" {
		flags = append(flags, "-m", agent.Model)
	}
	if agent.Effort != "" {
		flags = append(flags, "-c", fmt.Sprintf("reasoning.effort=%s", agent.Effort))
	}
	if agent.Temperature != nil {
		flags = append(flags, "-c", fmt.Sprintf("temperature=%g", *agent.Temperature))
	}
	flags = append(flags, agent.ExtraFlags...)
	return flags
}

func (c *Codex) InstallSuperpowers() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	repoDir := filepath.Join(home, ".codex", "superpowers")

	if err := cloneOrPull(repoDir, "https://github.com/obra/superpowers.git"); err != nil {
		return err
	}

	// Symlink skills
	skillsDir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	skillsLink := filepath.Join(skillsDir, "superpowers")
	skillsSrc := filepath.Join(repoDir, "skills")
	if err := os.Remove(skillsLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing skills link: %w", err)
	}
	if err := os.Symlink(skillsSrc, skillsLink); err != nil {
		return fmt.Errorf("symlink skills: %w", err)
	}

	return nil
}

func (c *Codex) InstallEnforcement() error { return nil }

func (c *Codex) SupportsTemperature() bool { return true }
func (c *Codex) SupportsEffort() bool      { return true }

func (c *Codex) ListEffortLevels(_ string) []string {
	return []string{"", "low", "medium", "high", "xhigh"}
}
