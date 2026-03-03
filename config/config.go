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

// GetConfigDir returns the XDG-compliant config directory (~/.config/kasmos/).
// On first call without that directory, it attempts to migrate legacy directories
// (.klique, then .hivemind) via rename. If migration fails it returns the old path.
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}

	target := filepath.Join(home, ".config", "kasmos")

	// Fast path: target already exists.
	if _, statErr := os.Stat(target); statErr == nil {
		return target, nil
	}

	// Try each legacy directory in preference order.
	for _, legacy := range []string{
		filepath.Join(home, ".klique"),
		filepath.Join(home, ".hivemind"),
	} {
		if _, statErr := os.Stat(legacy); statErr != nil {
			continue
		}
		// Ensure ~/.config/ parent exists before rename.
		if mkErr := os.MkdirAll(filepath.Dir(target), 0755); mkErr != nil {
			log.ErrorLog.Printf("failed to create %s: %v", filepath.Dir(target), mkErr)
			return legacy, nil
		}
		if mvErr := os.Rename(legacy, target); mvErr != nil {
			log.ErrorLog.Printf("failed to migrate %s to %s: %v", legacy, target, mvErr)
			return legacy, nil
		}
		return target, nil
	}

	// No legacy dir found — return target path; caller creates it on first write.
	return target, nil
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
	// TelemetryEnabled controls Sentry crash reporting; defaults to true when nil.
	TelemetryEnabled *bool `json:"telemetry_enabled,omitempty"`
	// DatabaseURL is the remote kasmos store URL; uses local file when empty.
	DatabaseURL string `json:"database_url,omitempty"`
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
		if tomlCfg.TelemetryEnabled != nil {
			cfg.TelemetryEnabled = tomlCfg.TelemetryEnabled
		}
		if tomlCfg.DatabaseURL != "" {
			cfg.DatabaseURL = tomlCfg.DatabaseURL
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
