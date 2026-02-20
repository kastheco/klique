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

	// Clone or pull
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return fmt.Errorf("create codex dir: %w", err)
		}
		cmd := exec.Command("git", "clone",
			"https://github.com/obra/superpowers.git", repoDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone superpowers: %s: %w", string(out), err)
		}
	} else {
		cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
		_ = cmd.Run() // best-effort update
	}

	// Symlink skills
	skillsDir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	skillsLink := filepath.Join(skillsDir, "superpowers")
	skillsSrc := filepath.Join(repoDir, "skills")
	os.Remove(skillsLink)
	if err := os.Symlink(skillsSrc, skillsLink); err != nil {
		return fmt.Errorf("symlink skills: %w", err)
	}

	return nil
}

func (c *Codex) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Scaffolding is orchestrated by internal/initcmd/scaffold package
	return nil
}

func (c *Codex) SupportsTemperature() bool { return true }
func (c *Codex) SupportsEffort() bool      { return true }
