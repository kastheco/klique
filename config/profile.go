package config

// AgentProfile defines the program and flags for an agent in a specific role.
type AgentProfile struct {
	Program     string   `json:"program"     toml:"program"`
	Flags       []string `json:"flags,omitempty" toml:"flags,omitempty"`
	Model       string   `json:"model,omitempty" toml:"model,omitempty"`
	Temperature *float64 `json:"temperature,omitempty" toml:"temperature,omitempty"`
	Effort      string   `json:"effort,omitempty" toml:"effort,omitempty"`
	Enabled     bool     `json:"enabled,omitempty" toml:"enabled,omitempty"`
}

// ResolveProfile looks up the agent profile for a given lifecycle phase.
// Falls back to defaultProgram if any link is missing, empty, or disabled.
func (c *Config) ResolveProfile(phase string, defaultProgram string) AgentProfile {
	if c.PhaseRoles == nil || c.Profiles == nil {
		return AgentProfile{Program: defaultProgram}
	}
	roleName, ok := c.PhaseRoles[phase]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	profile, ok := c.Profiles[roleName]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	if profile.Program == "" || !profile.Enabled {
		return AgentProfile{Program: defaultProgram}
	}
	return profile
}

// BuildCommand returns the full command string (program + flags) for this profile.
func (p AgentProfile) BuildCommand() string {
	if len(p.Flags) == 0 {
		return p.Program
	}
	result := p.Program
	for _, f := range p.Flags {
		result += " " + f
	}
	return result
}
