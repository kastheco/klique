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
	// Hooks configures FSM transition hooks loaded from config.toml.
	Hooks []TOMLHook `json:"hooks,omitempty"`
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

// applyConfigDefaults fills in zero-value fields of cfg with sensible defaults.
// It is nil-safe and centralises the default logic shared by DefaultConfig and configFromTOML.
func applyConfigDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.DefaultProgram == "" {
		program, err := GetDefaultCommand()
		if err != nil {
			log.ErrorLog.Printf("failed to get default command: %v", err)
			cfg.DefaultProgram = defaultProgram
		} else {
			cfg.DefaultProgram = program
		}
	}
	if cfg.DaemonPollInterval == 0 {
		cfg.DaemonPollInterval = 1000
	}
	if cfg.BranchPrefix == "" {
		cfg.BranchPrefix = branchPrefix()
	}
}

// DefaultConfig builds a Config populated with sensible out-of-the-box values.
func DefaultConfig() *Config {
	trueVal := true
	cfg := &Config{
		AutoYes:              false,
		AutoAdvanceWaves:     true,
		AutoReviewFix:        true,
		NotificationsEnabled: &trueVal,
	}
	applyConfigDefaults(cfg)
	return cfg
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

// configFromTOML converts a TOMLConfigResult into a Config.
// It builds a fresh Config so explicit false and 0 values from TOML survive without
// being silently dropped by an overlay-only approach.
// applyConfigDefaults fills in any omitted fields afterwards.
func configFromTOML(result *TOMLConfigResult) *Config {
	cfg := DefaultConfig()
	if result != nil {
		cfg.DefaultProgram = result.DefaultProgram
		cfg.AutoYes = result.AutoYes
		cfg.DaemonPollInterval = result.DaemonPollInterval
		cfg.BranchPrefix = result.BranchPrefix
		cfg.NotificationsEnabled = result.NotificationsEnabled
		cfg.Profiles = result.Profiles
		cfg.PhaseRoles = result.PhaseRoles
		cfg.AnimateBanner = result.AnimateBanner
		cfg.TelemetryEnabled = result.TelemetryEnabled
		cfg.DatabaseURL = result.DatabaseURL
		cfg.Hooks = result.Hooks
		cfg.BlueprintSkipThresholdValue = result.BlueprintSkipThreshold
		if result.AutoAdvanceWaves != nil {
			cfg.AutoAdvanceWaves = *result.AutoAdvanceWaves
		}
		if result.AutoReviewFix != nil {
			cfg.AutoReviewFix = *result.AutoReviewFix
		}
		if result.MaxReviewFixCycles != nil {
			cfg.MaxReviewFixCycles = *result.MaxReviewFixCycles
		}
	}
	applyConfigDefaults(cfg)
	return cfg
}

// configToTOML converts a Config back into a TOMLConfig for serialisation.
// It is used by migrateJSONToTOML and the default-config persistence path.
// AutoReviewFix and MaxReviewFixCycles are always written as pointers so that
// explicit false/0 values survive round-trips.
func configToTOML(cfg *Config) *TOMLConfig {
	if cfg == nil {
		return &TOMLConfig{}
	}
	phases := make(map[string]string, len(cfg.PhaseRoles))
	for phase, role := range cfg.PhaseRoles {
		phases[phase] = role
	}
	agents := make(map[string]TOMLAgent, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		agents[name] = TOMLAgent{
			Enabled:       p.Enabled,
			Program:       p.Program,
			Model:         p.Model,
			Temperature:   p.Temperature,
			Effort:        p.Effort,
			ExecutionMode: NormalizeExecutionMode(p.ExecutionMode),
			Flags:         p.Flags,
		}
	}
	out := &TOMLConfig{
		Phases: phases,
		Agents: agents,
		UI: TOMLUIConfig{
			AnimateBanner: cfg.AnimateBanner,
		},
		Telemetry:            TOMLTelemetryConfig{Enabled: cfg.TelemetryEnabled},
		Orchestration:        TOMLOrchestrationConfig{BlueprintSkipThreshold: cfg.BlueprintSkipThresholdValue},
		DatabaseURL:          cfg.DatabaseURL,
		DefaultProgram:       cfg.DefaultProgram,
		AutoYes:              cfg.AutoYes,
		DaemonPollInterval:   cfg.DaemonPollInterval,
		BranchPrefix:         cfg.BranchPrefix,
		NotificationsEnabled: cfg.NotificationsEnabled,
		Hooks:                cfg.Hooks,
	}
	autoReviewFix := cfg.AutoReviewFix
	autoAdvanceWaves := cfg.AutoAdvanceWaves
	out.UI.AutoAdvanceWaves = &autoAdvanceWaves
	out.UI.AutoReviewFix = &autoReviewFix
	maxReviewFixCycles := cfg.MaxReviewFixCycles
	out.UI.MaxReviewFixCycles = &maxReviewFixCycles
	return out
}

// migrateJSONToTOML reads config.json from configDir, writes its contents to
// config.toml, and renames config.json to config.json.migrated.
// Returns the parsed Config and true when a JSON config was found and parsed.
// All migration side effects (TOML write, rename) are best-effort: failures are
// logged but do not prevent startup — the parsed Config is still returned.
func migrateJSONToTOML(configDir string) (*Config, bool) {
	jsonPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		log.WarningLog.Printf("failed to read JSON config: %v", err)
		return nil, false
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return nil, false
	}
	applyConfigDefaults(&cfg)

	tomlPath := filepath.Join(configDir, TOMLConfigFileName)
	if err := SaveTOMLConfigTo(configToTOML(&cfg), tomlPath); err != nil {
		log.WarningLog.Printf("failed to migrate JSON config to TOML: %v", err)
		return &cfg, true
	}
	if err := os.Rename(jsonPath, filepath.Join(configDir, "config.json.migrated")); err != nil {
		log.WarningLog.Printf("failed to rename JSON config: %v", err)
	}
	return &cfg, true
}

// LoadConfig reads config.toml from the config directory as the authoritative source.
// When config.toml is absent but config.json exists, it performs a one-time migration:
// the JSON values are written to config.toml and config.json is renamed to
// config.json.migrated. When neither file exists, a default config is created and
// persisted as config.toml. On parse errors, defaults are returned without writing.
func LoadConfig() *Config {
	dir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	tomlResult, tomlErr := LoadTOMLConfig()
	if tomlErr != nil {
		log.WarningLog.Printf("failed to load TOML config: %v", tomlErr)
		return DefaultConfig()
	}
	if tomlResult != nil {
		return configFromTOML(tomlResult)
	}

	if cfg, ok := migrateJSONToTOML(dir); ok {
		return cfg
	}

	def := DefaultConfig()
	if saveErr := SaveTOMLConfigTo(configToTOML(def), filepath.Join(dir, TOMLConfigFileName)); saveErr != nil {
		log.WarningLog.Printf("failed to save default config: %v", saveErr)
	}
	return def
}
