package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kastheco/kasmos/log"
)

const (
	// ConfigFileName is the name of the JSON config file within the config dir.
	ConfigFileName = "config.json"

	// defaultProgram is the fallback program name when command detection fails.
	defaultProgram = "opencode"
)

// aliasRegex matches shell alias output to extract the real command path.
// Handles formats: "aliased to <path>", "-> <path>", "= <path>".
var aliasRegex = regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)

// GetConfigDir returns the project-local config directory (<repo-root>/.kasmos/).
//
// If the current directory is inside a git repository (including a worktree),
// the directory is anchored to the main repository root so all orchestrator
// state uses a single shared location.
// On first call without existing config files in the target, it attempts a one-time
// migration by copying files from legacy XDG directories. The migration is a copy
// (not a move) so legacy locations are preserved. Any migration error is silently
// ignored — the new target is always returned.
func GetConfigDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	baseDir := cwd
	if repoRoot, repoErr := ResolveRepoRoot(cwd); repoErr == nil {
		baseDir = repoRoot
	}

	target := filepath.Join(baseDir, ".kasmos")

	// Fast path: config already exists in target — skip migration entirely.
	for _, marker := range []string{"config.toml", "config.json"} {
		if _, statErr := os.Stat(filepath.Join(target, marker)); statErr == nil {
			return target, nil
		}
	}

	// Attempt one-time migration from legacy directories (copy, not move).
	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		for _, legacy := range []string{
			filepath.Join(home, ".config", "kasmos"),
			filepath.Join(home, ".klique"),
			filepath.Join(home, ".hivemind"),
		} {
			if _, statErr := os.Stat(legacy); statErr != nil {
				continue
			}
			// Legacy dir found — copy known files to target.
			if mkErr := os.MkdirAll(target, 0755); mkErr != nil {
				log.ErrorLog.Printf("failed to create %s: %v", target, mkErr)
				break
			}
			for _, fname := range []string{"config.json", "config.toml", "state.json", "taskstore.db"} {
				copyIfMissing(filepath.Join(legacy, fname), filepath.Join(target, fname))
			}
			break
		}
	}

	return target, nil
}

// ResolveRepoRoot returns the main repository root for dir.
// It supports both normal repos and git worktrees.
func ResolveRepoRoot(dir string) (string, error) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		// .git not found — try git CLI fallback.
		return resolveRepoRootViaGit(dir)
	}

	if info.IsDir() {
		// Regular repo: .git is a directory, so dir is the repo root.
		return dir, nil
	}

	// Worktree: .git is a file with content "gitdir: <path>".
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return resolveRepoRootViaGit(dir)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return resolveRepoRootViaGit(dir)
	}

	worktreeGitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(dir, worktreeGitDir)
	}
	worktreeGitDir = filepath.Clean(worktreeGitDir)

	commondirPath := filepath.Join(worktreeGitDir, "commondir")
	commondirData, err := os.ReadFile(commondirPath)
	if err != nil {
		return resolveRepoRootViaGit(dir)
	}
	commondir := strings.TrimSpace(string(commondirData))

	var mainGitDir string
	if filepath.IsAbs(commondir) {
		mainGitDir = commondir
	} else {
		mainGitDir = filepath.Clean(filepath.Join(worktreeGitDir, commondir))
	}

	return filepath.Dir(mainGitDir), nil
}

// resolveRepoRootViaGit shells out to git to find the main repository root.
func resolveRepoRootViaGit(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		out, err = exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
		if err != nil {
			return "", fmt.Errorf("resolve repo root for %s: %w", dir, err)
		}
	}

	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return "", fmt.Errorf("resolve repo root for %s: empty git-common-dir output", dir)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	return filepath.Dir(filepath.Clean(gitDir)), nil
}

// copyIfMissing copies src to dst only when dst does not already exist.
// Any error (missing src, unreadable, write failure) is silently ignored — migration
// is best-effort and must never block normal startup.
func copyIfMissing(src, dst string) {
	if _, err := os.Stat(dst); err == nil {
		// dst already exists — do not overwrite.
		return
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}
	_ = os.WriteFile(dst, data, 0644)
}

// Config holds all persistent application configuration.
type Config struct {
	// DefaultProgram is the command launched for new instances.
	DefaultProgram string `json:"default_program"`
	// AutoYes makes the daemon automatically accept all agent prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is how often (ms) the daemon checks sessions.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is prepended to git branch names created by the app.
	BranchPrefix string `json:"branch_prefix"`
	// NotificationsEnabled controls desktop notifications; defaults to true when nil.
	NotificationsEnabled *bool `json:"notifications_enabled,omitempty"`
	// Profiles maps role names to agent program configurations.
	Profiles map[string]AgentProfile `json:"profiles,omitempty"`
	// PhaseRoles maps lifecycle phase names to role names.
	PhaseRoles map[string]string `json:"phase_roles,omitempty"`
	// AnimateBanner enables the idle banner animation (off by default).
	AnimateBanner bool `json:"animate_banner,omitempty"`
	// AutoAdvanceWaves skips the confirmation dialog after a clean wave.
	AutoAdvanceWaves bool `json:"auto_advance_waves,omitempty"`
	// AutoReviewFix enables the automatic review→fix→re-review loop.
	AutoReviewFix bool `json:"auto_review_fix,omitempty"`
	// MaxReviewFixCycles caps the review-fix loop iterations (0 = unlimited).
	MaxReviewFixCycles int `json:"max_review_fix_cycles,omitempty"`
	// TelemetryEnabled controls Sentry crash reporting; defaults to true when nil.
	TelemetryEnabled *bool `json:"telemetry_enabled,omitempty"`
	// DatabaseURL is the remote kasmos store URL; uses local file when empty.
	DatabaseURL string `json:"database_url,omitempty"`
	// BlueprintSkipThresholdValue is the maximum task count below which single-agent
	// blueprint-skip mode is used instead of wave orchestration.
	// When nil, the default threshold of 2 applies.
	BlueprintSkipThresholdValue *int `json:"blueprint_skip_threshold,omitempty"`
}

// BlueprintSkipThreshold returns the configured threshold for single-agent mode.
// Plans with <= threshold tasks skip elaboration and wave orchestration.
// Defaults to 2 when not configured.
func (c *Config) BlueprintSkipThreshold() int {
	if c.BlueprintSkipThresholdValue == nil {
		return 2
	}
	return *c.BlueprintSkipThresholdValue
}

// DefaultConfig builds a Config populated with sensible out-of-the-box values.
func DefaultConfig() *Config {
	program, err := GetDefaultCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get default command: %v", err)
		program = defaultProgram
	}

	trueVal := true
	prefix := branchPrefix()

	return &Config{
		DefaultProgram:       program,
		AutoYes:              false,
		DaemonPollInterval:   1000,
		BranchPrefix:         prefix,
		NotificationsEnabled: &trueVal,
	}
}

// branchPrefix derives the git branch prefix from the current OS user.
// Falls back to "session/" when the username is unavailable.
func branchPrefix() string {
	u, err := user.Current()
	if err != nil || u == nil || u.Username == "" {
		log.ErrorLog.Printf("failed to get current user: %v", err)
		return "session/"
	}
	return fmt.Sprintf("%s/", strings.ToLower(u.Username))
}

// AreNotificationsEnabled reports whether desktop notifications are active.
// Returns true when NotificationsEnabled is nil (opt-out semantics).
func (c *Config) AreNotificationsEnabled() bool {
	if c.NotificationsEnabled == nil {
		return true
	}
	return *c.NotificationsEnabled
}

// IsTelemetryEnabled reports whether Sentry telemetry is active.
// Returns true when TelemetryEnabled is nil (opt-out semantics).
func (c *Config) IsTelemetryEnabled() bool {
	if c.TelemetryEnabled == nil {
		return true
	}
	return *c.TelemetryEnabled
}

// GetDefaultCommand returns the preferred agent command path.
// It tries opencode first, then falls back to claude.
func GetDefaultCommand() (string, error) {
	if p, err := findCommand("opencode"); err == nil {
		return p, nil
	}
	if p, err := findCommand("claude"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("neither opencode nor claude command found in aliases or PATH")
}

// findCommand locates name via the user's login shell and PATH.
// It sources the appropriate rc file so aliases are visible, then falls
// back to exec.LookPath when the shell subprocess fails.
func findCommand(name string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	var inline string
	switch {
	case strings.Contains(shell, "zsh"):
		inline = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; which %s", name)
	case strings.Contains(shell, "bash"):
		inline = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; which %s", name)
	default:
		inline = fmt.Sprintf("which %s", name)
	}

	out, err := exec.Command(shell, "-c", inline).Output()
	if err == nil && len(out) > 0 {
		if p := parseCommandOutput(string(out)); p != "" {
			return p, nil
		}
	}

	// Fallback: consult PATH directly.
	if p, lookErr := exec.LookPath(name); lookErr == nil {
		return p, nil
	}

	return "", fmt.Errorf("%s command not found in aliases or PATH", name)
}

// parseCommandOutput extracts a command path from raw shell output.
// It resolves alias declarations (e.g. "opencode: aliased to /usr/local/bin/opencode")
// and returns an empty string for blank output.
func parseCommandOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	if m := aliasRegex.FindStringSubmatch(trimmed); len(m) > 1 {
		return m[1]
	}
	return trimmed
}

// LoadConfig reads config.json from the config directory. When the file is
// absent it creates and persists a default. On parse errors it returns a default.
// TOML config is overlaid after JSON load for Profiles, PhaseRoles, and flags.
func LoadConfig() *Config {
	dir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	data, readErr := os.ReadFile(filepath.Join(dir, ConfigFileName))
	if readErr != nil {
		if os.IsNotExist(readErr) {
			def := DefaultConfig()
			if saveErr := saveConfig(def); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return def
		}
		log.WarningLog.Printf("failed to get config file: %v", readErr)
		return DefaultConfig()
	}

	var cfg Config
	if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", unmarshalErr)
		return DefaultConfig()
	}

	// Overlay TOML config values where present — TOML is authoritative for these fields.
	tomlCfg, tomlErr := LoadTOMLConfig()
	if tomlErr != nil {
		log.WarningLog.Printf("failed to load TOML config: %v", tomlErr)
	} else if tomlCfg != nil {
		if len(tomlCfg.Profiles) > 0 {
			cfg.Profiles = tomlCfg.Profiles
		}
		if len(tomlCfg.PhaseRoles) > 0 {
			cfg.PhaseRoles = tomlCfg.PhaseRoles
		}
		if tomlCfg.AnimateBanner {
			cfg.AnimateBanner = true
		}
		if tomlCfg.AutoAdvanceWaves {
			cfg.AutoAdvanceWaves = true
		}
		if tomlCfg.AutoReviewFix != nil {
			cfg.AutoReviewFix = *tomlCfg.AutoReviewFix
		}
		if tomlCfg.MaxReviewFixCycles != nil {
			cfg.MaxReviewFixCycles = *tomlCfg.MaxReviewFixCycles
		}
		if tomlCfg.TelemetryEnabled != nil {
			cfg.TelemetryEnabled = tomlCfg.TelemetryEnabled
		}
		if tomlCfg.DatabaseURL != "" {
			cfg.DatabaseURL = tomlCfg.DatabaseURL
		}
		if tomlCfg.BlueprintSkipThreshold != nil {
			cfg.BlueprintSkipThresholdValue = tomlCfg.BlueprintSkipThreshold
		}
	}

	return &cfg
}

// saveConfig serialises cfg as indented JSON and writes it to the config directory.
func saveConfig(cfg *Config) error {
	dir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return fmt.Errorf("failed to create config directory: %w", mkErr)
	}
	data, marshalErr := json.MarshalIndent(cfg, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal config: %w", marshalErr)
	}
	return os.WriteFile(filepath.Join(dir, ConfigFileName), data, 0644)
}

// SaveConfig is the exported wrapper around saveConfig.
func SaveConfig(cfg *Config) error {
	return saveConfig(cfg)
}
