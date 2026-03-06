package config

import "strings"

// AgentProfile defines the program and flags for an agent in a specific role.
type AgentProfile struct {
	Program       string   `json:"program"     toml:"program"`
	Flags         []string `json:"flags,omitempty" toml:"flags,omitempty"`
	Model         string   `json:"model,omitempty" toml:"model,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty" toml:"temperature,omitempty"`
	Effort        string   `json:"effort,omitempty" toml:"effort,omitempty"`
	ExecutionMode string   `json:"execution_mode,omitempty" toml:"execution_mode,omitempty"`
	Enabled       bool     `json:"enabled,omitempty" toml:"enabled,omitempty"`
}

const (
	ExecutionModeTmux     = "tmux"
	ExecutionModeHeadless = "headless"
)

func NormalizeExecutionMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", ExecutionModeTmux:
		return ExecutionModeTmux
	case ExecutionModeHeadless:
		return ExecutionModeHeadless
	default:
		return ExecutionModeTmux
	}
}

// ResolveProfile looks up the agent profile for a given lifecycle phase.
// Falls back to defaultProgram if any link is missing, empty, or disabled.
func (c *Config) ResolveProfile(phase string, defaultProgram string) AgentProfile {
	if c.PhaseRoles == nil || c.Profiles == nil {
		return AgentProfile{Program: defaultProgram, ExecutionMode: ExecutionModeTmux}
	}
	roleName, ok := c.PhaseRoles[phase]
	if !ok {
		return AgentProfile{Program: defaultProgram, ExecutionMode: ExecutionModeTmux}
	}
	profile, ok := c.Profiles[roleName]
	if !ok {
		return AgentProfile{Program: defaultProgram, ExecutionMode: ExecutionModeTmux}
	}
	if profile.Program == "" || !profile.Enabled {
		return AgentProfile{Program: defaultProgram, ExecutionMode: ExecutionModeTmux}
	}
	profile.ExecutionMode = NormalizeExecutionMode(profile.ExecutionMode)
	return profile
}

// BuildCommand returns the full command string (program + flags) for this profile.
func (p AgentProfile) BuildCommand() string {
	return strings.Join(append([]string{p.Program}, p.Flags...), " ")
}
