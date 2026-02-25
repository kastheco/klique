package config

import (
	"encoding/json"
	"fmt"
	"github.com/kastheco/kasmos/log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ConfigFileName = "config.json"
	defaultProgram = "claude"
)

// GetConfigDir returns the path to the application's configuration directory.
// Uses XDG-compliant ~/.config/kasmos/. On first run, migrates legacy
// directories: ~/.klique or ~/.hivemind -> ~/.config/kasmos/.
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	newDir := filepath.Join(homeDir, ".config", "kasmos")

	// Already exists — fast path
	if _, err := os.Stat(newDir); err == nil {
		return newDir, nil
	}

	// Try migrating from legacy directories (most recent first)
	legacyDirs := []string{
		filepath.Join(homeDir, ".klique"),
		filepath.Join(homeDir, ".hivemind"),
	}

	for _, oldDir := range legacyDirs {
		if _, err := os.Stat(oldDir); err == nil {
			// Ensure parent ~/.config/ exists
			if mkErr := os.MkdirAll(filepath.Dir(newDir), 0755); mkErr != nil {
				log.ErrorLog.Printf("failed to create %s: %v", filepath.Dir(newDir), mkErr)
				return oldDir, nil
			}
			if renameErr := os.Rename(oldDir, newDir); renameErr != nil {
				log.ErrorLog.Printf("failed to migrate %s to %s: %v", oldDir, newDir, renameErr)
				return oldDir, nil
			}
			return newDir, nil
		}
	}

	return newDir, nil
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// NotificationsEnabled controls whether macOS/Linux desktop notifications
	// are sent when an agent finishes (Running -> Ready).
	NotificationsEnabled *bool `json:"notifications_enabled,omitempty"`
	// Profiles maps agent role names to their program and flags configuration.
	Profiles map[string]AgentProfile `json:"profiles,omitempty"`
	// PhaseRoles maps lifecycle phase names to agent role names.
	PhaseRoles map[string]string `json:"phase_roles,omitempty"`
	// AnimateBanner controls the idle banner animation (disabled by default).
	AnimateBanner bool `json:"animate_banner,omitempty"`
	// TelemetryEnabled controls whether crash reporting via Sentry is active.
	// Defaults to true when not set.
	TelemetryEnabled *bool `json:"telemetry_enabled,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	trueVal := true
	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix: func() string {
			user, err := user.Current()
			if err != nil || user == nil || user.Username == "" {
				log.ErrorLog.Printf("failed to get current user: %v", err)
				return "session/"
			}
			return fmt.Sprintf("%s/", strings.ToLower(user.Username))
		}(),
		NotificationsEnabled: &trueVal,
	}
}

// AreNotificationsEnabled returns whether desktop notifications are enabled.
// Defaults to true when the field is not set.
func (c *Config) AreNotificationsEnabled() bool {
	if c.NotificationsEnabled == nil {
		return true
	}
	return *c.NotificationsEnabled
}

// IsTelemetryEnabled returns whether Sentry telemetry is enabled.
// Defaults to true when the field is not set.
func (c *Config) IsTelemetryEnabled() bool {
	if c.TelemetryEnabled == nil {
		return true
	}
	return *c.TelemetryEnabled
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution: using "which" command
// 2. PATH lookup
//
// If both fail, it returns an error.
func GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Force the shell to load the user's profile and then run the command
	// For zsh, source .zshrc; for bash, source .bashrc
	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = "source ~/.zshrc &>/dev/null || true; which claude"
	} else if strings.Contains(shell, "bash") {
		shellCmd = "source ~/.bashrc &>/dev/null || true; which claude"
	} else {
		shellCmd = "which claude"
	}

	cmd := exec.Command(shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Check if the output is an alias definition and extract the actual path
			// Handle formats like "claude: aliased to /path/to/claude" or other shell-specific formats
			aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	// Otherwise, try to find in PATH directly
	claudePath, err := exec.LookPath("claude")
	if err == nil {
		return claudePath, nil
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	// Overlay TOML config if it exists (TOML is authority for Profiles and PhaseRoles)
	tomlResult, tomlErr := LoadTOMLConfig()
	if tomlErr != nil {
		log.WarningLog.Printf("failed to load TOML config: %v", tomlErr)
	} else if tomlResult != nil {
		if len(tomlResult.Profiles) > 0 {
			config.Profiles = tomlResult.Profiles
		}
		if len(tomlResult.PhaseRoles) > 0 {
			config.PhaseRoles = tomlResult.PhaseRoles
		}
		if tomlResult.AnimateBanner {
			config.AnimateBanner = true
		}
		if tomlResult.TelemetryEnabled != nil {
			config.TelemetryEnabled = tomlResult.TelemetryEnabled
		}
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}
