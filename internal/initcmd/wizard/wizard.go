package wizard

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

// State holds all wizard-collected values across stages.
type State struct {
	// Stage 1 outputs
	Registry        *harness.Registry
	DetectResults   []harness.DetectResult
	SelectedHarness []string // names of harnesses user selected

	// Stage 2 outputs
	Agents []AgentState

	// Stage 3 outputs
	PhaseMapping map[string]string

	// Stage 4 outputs
	SelectedTools []string // binary names of CLI tools to include in scaffolded agent files
}

// AgentState holds the wizard form values for one agent role.
type AgentState struct {
	Role        string
	Harness     string
	Model       string
	Temperature string // "" means default; parsed to *float64 on save
	Effort      string // "" means default
	Enabled     bool
}

// DefaultAgentRoles returns the built-in agent role names.
func DefaultAgentRoles() []string {
	return []string{"coder", "reviewer", "planner", "chat"}
}

// RoleDefaults returns sensible per-role defaults for fresh inits.
// Harness is left empty; the caller fills it from selected harnesses.
func RoleDefaults() map[string]AgentState {
	return map[string]AgentState{
		"coder": {
			Role:        "coder",
			Model:       "anthropic/claude-sonnet-4-6",
			Effort:      "medium",
			Temperature: "0.1",
			Enabled:     true,
		},
		"planner": {
			Role:        "planner",
			Model:       "anthropic/claude-opus-4-6",
			Effort:      "max",
			Temperature: "0.5",
			Enabled:     true,
		},
		"reviewer": {
			Role:        "reviewer",
			Model:       "openai/gpt-5.3-codex",
			Effort:      "xhigh",
			Temperature: "0.2",
			Enabled:     true,
		},
		"chat": {
			Role:        "chat",
			Model:       "anthropic/claude-sonnet-4-6",
			Effort:      "high",
			Temperature: "0.3",
			Enabled:     true,
		},
	}
}

// IsCustomized returns true if the agent's settings differ from factory RoleDefaults.
// defaultHarness is the harness that would be assigned if the user didn't customize.
func IsCustomized(a AgentState, defaultHarness string) bool {
	defaults, ok := RoleDefaults()[a.Role]
	if !ok {
		return false // unknown role, can't compare
	}
	defaults.Harness = defaultHarness
	return a.Harness != defaults.Harness ||
		a.Model != defaults.Model ||
		a.Effort != defaults.Effort ||
		a.Temperature != defaults.Temperature ||
		a.Enabled != defaults.Enabled
}

// Run executes all wizard stages in sequence.
// If existing is non-nil, pre-populates forms from existing config.
func Run(registry *harness.Registry, existing *config.TOMLConfigResult) (*State, error) {
	m := newRootModel(registry, existing)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	rm, ok := finalModel.(rootModel)
	if !ok {
		return nil, fmt.Errorf("unexpected wizard model type %T", finalModel)
	}
	if rm.cancelled {
		return nil, fmt.Errorf("wizard cancelled")
	}
	return rm.state, nil
}

// parseTemperature converts a temperature string to *float64.
// Returns nil for empty string or unparsable values.
func parseTemperature(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// ToTOMLConfig converts wizard state to the TOML config structure.
// Disabled agents are included so their configuration is preserved across re-runs.
func (s *State) ToTOMLConfig() *config.TOMLConfig {
	tc := &config.TOMLConfig{
		Phases: s.PhaseMapping,
		Agents: make(map[string]config.TOMLAgent),
	}

	for _, a := range s.Agents {
		tc.Agents[a.Role] = config.TOMLAgent{
			Enabled:     a.Enabled,
			Program:     a.Harness,
			Model:       a.Model,
			Effort:      a.Effort,
			Temperature: parseTemperature(a.Temperature),
			Flags:       []string{},
		}
	}

	return tc
}

// ToAgentConfigs converts wizard state to harness.AgentConfig slice
// for use by scaffold.
//
// The chat role is special: it is not user-configurable per harness, so a
// single AgentState entry is stored with the first selected harness. To ensure
// chat.md is scaffolded for every selected harness, we fan it out here.
func (s *State) ToAgentConfigs() []harness.AgentConfig {
	var configs []harness.AgentConfig
	for _, a := range s.Agents {
		if !a.Enabled {
			continue
		}
		if a.Role == "chat" {
			// Emit one entry per selected harness so chat.md is written everywhere.
			for _, h := range s.SelectedHarness {
				configs = append(configs, harness.AgentConfig{
					Role:        a.Role,
					Harness:     h,
					Model:       a.Model,
					Effort:      a.Effort,
					Enabled:     a.Enabled,
					Temperature: parseTemperature(a.Temperature),
				})
			}
			continue
		}
		configs = append(configs, harness.AgentConfig{
			Role:        a.Role,
			Harness:     a.Harness,
			Model:       a.Model,
			Effort:      a.Effort,
			Enabled:     a.Enabled,
			Temperature: parseTemperature(a.Temperature),
		})
	}
	return configs
}
