package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/kastheco/klique/config"
)

func runPhaseStage(state *State, existing *config.TOMLConfigResult) error {
	// Collect enabled agent names for dropdown options
	var enabledAgents []huh.Option[string]
	for _, a := range state.Agents {
		if a.Enabled {
			enabledAgents = append(enabledAgents, huh.NewOption(a.Role, a.Role))
		}
	}

	if len(enabledAgents) == 0 {
		return fmt.Errorf("no agents enabled; cannot map phases")
	}

	// Initialize phase mapping with defaults or existing values
	phases := DefaultPhases()
	state.PhaseMapping = make(map[string]string)

	defaults := map[string]string{
		"implementing":   "coder",
		"spec_review":    "reviewer",
		"quality_review": "reviewer",
		"planning":       "planner",
	}

	// Pre-populate from existing config or defaults
	for _, phase := range phases {
		if existing != nil && existing.PhaseRoles != nil {
			if role, ok := existing.PhaseRoles[phase]; ok {
				state.PhaseMapping[phase] = role
				continue
			}
		}
		state.PhaseMapping[phase] = defaults[phase]
	}

	// Build indexed slice for value binding
	phaseValues := make([]string, len(phases))
	for i, phase := range phases {
		phaseValues[i] = state.PhaseMapping[phase]
	}

	// Build summary of configured agents
	summary := BuildProgressNote(state.Agents, len(state.Agents))

	var fields []huh.Field

	// Note header showing agent summary
	fields = append(fields,
		huh.NewNote().
			Title("Map lifecycle phases to agents").
			Description(summary),
	)

	for i, phase := range phases {
		fields = append(fields,
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Phase: %s", phase)).
				Options(enabledAgents...).
				Value(&phaseValues[i]),
		)
	}

	form := huh.NewForm(
		huh.NewGroup(fields...),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return fmt.Errorf("phase mapping: %w", err)
	}

	// Write back to state
	for i, phase := range phases {
		state.PhaseMapping[phase] = phaseValues[i]
	}

	return nil
}
