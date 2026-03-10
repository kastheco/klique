package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const TOMLConfigFileName = "config.toml"

// TOMLAgent is the TOML-level representation of an agent.
// Maps directly to [agents.*] tables in config.toml.
type TOMLAgent struct {
	Enabled       bool     `toml:"enabled"`
	Program       string   `toml:"program"`
	Model         string   `toml:"model,omitempty"`
	Temperature   *float64 `toml:"temperature,omitempty"`
	Effort        string   `toml:"effort,omitempty"`
	ExecutionMode string   `toml:"execution_mode,omitempty"`
	Flags         []string `toml:"flags,omitempty"`
}

func (a TOMLAgent) toProfile() AgentProfile {
	return AgentProfile{
		Program:       a.Program,
		Model:         a.Model,
		Temperature:   a.Temperature,
		Effort:        a.Effort,
		ExecutionMode: NormalizeExecutionMode(a.ExecutionMode),
		Enabled:       a.Enabled,
		Flags:         a.Flags,
	}
}

// TOMLUIConfig holds UI-specific settings from the [ui] TOML table.
type TOMLUIConfig struct {
	AnimateBanner      bool  `toml:"animate_banner"`
	AutoAdvanceWaves   bool  `toml:"auto_advance_waves"`
	AutoReviewFix      *bool `toml:"auto_review_fix"`
	MaxReviewFixCycles *int  `toml:"max_review_fix_cycles"`
}

// TOMLTelemetryConfig holds telemetry settings from the [telemetry] TOML table.
type TOMLTelemetryConfig struct {
	Enabled *bool `toml:"enabled,omitempty"`
}

// TOMLHook is the TOML/JSON representation of a single FSM transition hook.
// Maps directly to [[hooks]] entries in config.toml or the "hooks" JSON array.
type TOMLHook struct {
	Type    string            `json:"type"              toml:"type"`
	URL     string            `json:"url,omitempty"     toml:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
	Command string            `json:"command,omitempty" toml:"command,omitempty"`
	Events  []string          `json:"events,omitempty"  toml:"events,omitempty"`
}

// TOMLOrchestrationConfig holds orchestration settings from the [orchestration] TOML table.
type TOMLOrchestrationConfig struct {
	// BlueprintSkipThreshold is the maximum task count for single-agent mode.
	// When <= this value, elaboration and wave orchestration are skipped.
	BlueprintSkipThreshold *int `toml:"blueprint_skip_threshold,omitempty"`
}

// TOMLConfig is the top-level TOML file structure.
type TOMLConfig struct {
	Phases        map[string]string       `toml:"phases"`
	Agents        map[string]TOMLAgent    `toml:"agents"`
	UI            TOMLUIConfig            `toml:"ui"`
	Telemetry     TOMLTelemetryConfig     `toml:"telemetry"`
	Orchestration TOMLOrchestrationConfig `toml:"orchestration"`
	DatabaseURL   string                  `toml:"database_url,omitempty"`
	Hooks         []TOMLHook              `toml:"hooks"`
}

// TOMLConfigResult holds the parsed config in terms of internal types.
type TOMLConfigResult struct {
	Profiles               map[string]AgentProfile
	PhaseRoles             map[string]string
	AnimateBanner          bool
	AutoAdvanceWaves       bool
	AutoReviewFix          *bool
	MaxReviewFixCycles     *int
	TelemetryEnabled       *bool
	DatabaseURL            string
	BlueprintSkipThreshold *int
	Hooks                  []TOMLHook
}

// LoadTOMLConfigFrom reads and parses a TOML config file,
// returning the result mapped to internal types.
func LoadTOMLConfigFrom(path string) (*TOMLConfigResult, error) {
	var tc TOMLConfig
	if _, err := toml.DecodeFile(path, &tc); err != nil {
		return nil, fmt.Errorf("decode TOML config: %w", err)
	}

	result := &TOMLConfigResult{
		Profiles:               make(map[string]AgentProfile),
		PhaseRoles:             tc.Phases,
		AnimateBanner:          tc.UI.AnimateBanner,
		AutoAdvanceWaves:       tc.UI.AutoAdvanceWaves,
		AutoReviewFix:          tc.UI.AutoReviewFix,
		MaxReviewFixCycles:     tc.UI.MaxReviewFixCycles,
		TelemetryEnabled:       tc.Telemetry.Enabled,
		DatabaseURL:            tc.DatabaseURL,
		BlueprintSkipThreshold: tc.Orchestration.BlueprintSkipThreshold,
		Hooks:                  tc.Hooks,
	}

	for name, agent := range tc.Agents {
		result.Profiles[name] = agent.toProfile()
	}

	return result, nil
}

// LoadTOMLConfig loads the TOML config from the project-local config directory
// (<repo-root>/.kasmos/config.toml when in a git repo/worktree).
// Returns nil, nil if the file does not exist.
func LoadTOMLConfig() (*TOMLConfigResult, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, TOMLConfigFileName)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no TOML config is valid
		}
		return nil, fmt.Errorf("stat TOML config: %w", err)
	}

	return LoadTOMLConfigFrom(path)
}

// LoadHooksForRepo reads the [[hooks]] entries from <repoPath>/.kasmos/config.toml
// without any side effects (no config creation, no writes). Returns nil when the
// file does not exist or contains no hooks. Errors are returned to the caller.
func LoadHooksForRepo(repoPath string) ([]TOMLHook, error) {
	path := filepath.Join(repoPath, ".kasmos", TOMLConfigFileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat TOML config for repo %s: %w", repoPath, err)
	}
	result, err := LoadTOMLConfigFrom(path)
	if err != nil {
		return nil, err
	}
	return result.Hooks, nil
}

// SaveTOMLConfigTo writes a TOMLConfig to the given path.
func SaveTOMLConfigTo(tc *TOMLConfig, path string) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("close config file: %w", cerr)
		}
	}()

	if _, err := fmt.Fprintln(f, "# Generated by kas setup"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f); err != nil {
		return err
	}

	enc := toml.NewEncoder(f)
	if err := enc.Encode(tc); err != nil {
		return fmt.Errorf("encode TOML: %w", err)
	}
	return nil
}

// SaveTOMLConfig writes to the project-local config directory
// (<repo-root>/.kasmos/config.toml when in a git repo/worktree).
func SaveTOMLConfig(tc *TOMLConfig) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return SaveTOMLConfigTo(tc, filepath.Join(configDir, TOMLConfigFileName))
}

// GetTOMLConfigPath returns the path to the TOML config file.
func GetTOMLConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, TOMLConfigFileName), nil
}
