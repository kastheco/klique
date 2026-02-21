package wizard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/internal/initcmd/harness"
)

// BuildProgressNote renders a summary of agent configuration progress.
// Used as the description for huh.NewNote() at the top of each agent form.
func BuildProgressNote(agents []AgentState, currentIdx int) string {
	var lines []string
	for i, a := range agents {
		switch {
		case i < currentIdx && !a.Enabled:
			lines = append(lines, fmt.Sprintf("  ⊘ %s  (disabled)", a.Role))
		case i < currentIdx:
			summary := a.Harness
			if a.Model != "" {
				summary += " / " + a.Model
			}
			if a.Effort != "" {
				summary += " / " + a.Effort
			}
			lines = append(lines, fmt.Sprintf("  ✓ %-10s %s", a.Role, summary))
		case i == currentIdx:
			lines = append(lines, fmt.Sprintf("  ▸ %-10s configuring...", a.Role))
		default:
			lines = append(lines, fmt.Sprintf("  ○ %s", a.Role))
		}
	}
	return strings.Join(lines, "\n")
}

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

// DefaultPhases returns the default lifecycle phase names.
func DefaultPhases() []string {
	return []string{"implementing", "spec_review", "quality_review", "planning"}
}

// DefaultAgentRoles returns the built-in agent role names.
func DefaultAgentRoles() []string {
	return []string{"coder", "reviewer", "planner"}
}

// Run executes all wizard stages in sequence.
// If existing is non-nil, pre-populates forms from existing config.
func Run(registry *harness.Registry, existing *config.TOMLConfigResult) (*State, error) {
	state := &State{
		Registry:      registry,
		DetectResults: registry.DetectAll(),
	}

	// Stage 1: Harness selection
	if err := runHarnessStage(state); err != nil {
		return nil, err
	}

	// Stage 2: Agent configuration
	if err := runAgentStage(state, existing); err != nil {
		return nil, err
	}

	// Stage 3: Phase mapping
	if err := runPhaseStage(state, existing); err != nil {
		return nil, err
	}

	// Stage 4: Tool discovery
	if err := runToolsStage(state); err != nil {
		return nil, err
	}

	return state, nil
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
// for use by scaffold and superpowers install.
func (s *State) ToAgentConfigs() []harness.AgentConfig {
	var configs []harness.AgentConfig
	for _, a := range s.Agents {
		if !a.Enabled {
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
