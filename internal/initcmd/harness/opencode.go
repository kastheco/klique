package harness

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenCode implements Harness for the OpenCode CLI.
type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) Detect() (string, bool) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels shells out to `opencode models` and parses the output line-by-line.
func (o *OpenCode) ListModels() ([]string, error) {
	cmd := exec.Command("opencode", "models")
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

func (o *OpenCode) InstallSuperpowers() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")

	// Clone or pull
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		cmd := exec.Command("git", "clone",
			"https://github.com/obra/superpowers.git", repoDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone superpowers: %s: %w", string(out), err)
		}
	} else {
		cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
		_ = cmd.Run() // best-effort update
	}

	// Symlink plugin
	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}
	pluginLink := filepath.Join(pluginDir, "superpowers.js")
	pluginSrc := filepath.Join(repoDir, ".opencode", "plugins", "superpowers.js")
	os.Remove(pluginLink)
	if err := os.Symlink(pluginSrc, pluginLink); err != nil {
		return fmt.Errorf("symlink plugin: %w", err)
	}

	// Symlink skills
	skillsDir := filepath.Join(home, ".config", "opencode", "skills")
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

func (o *OpenCode) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Scaffolding is orchestrated by internal/initcmd/scaffold package
	return nil
}

func (o *OpenCode) SupportsTemperature() bool { return true }
func (o *OpenCode) SupportsEffort() bool      { return true }
