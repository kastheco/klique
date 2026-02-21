package wizard

import (
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/kastheco/klique/config"
)

func runAgentStage(state *State, existing *config.TOMLConfigResult) error {
	roles := DefaultAgentRoles()

	// Initialize agent states with defaults or existing values
	defaultHarness := ""
	if len(state.SelectedHarness) > 0 {
		defaultHarness = state.SelectedHarness[0]
	}
	for _, role := range roles {
		as := AgentState{
			Role:    role,
			Harness: defaultHarness,
			Enabled: true,
		}

		// Pre-populate from existing config
		if existing != nil {
			if profile, ok := existing.Profiles[role]; ok {
				as.Harness = profile.Program
				as.Model = profile.Model
				as.Effort = profile.Effort
				as.Enabled = profile.Enabled
				if profile.Temperature != nil {
					as.Temperature = fmt.Sprintf("%g", *profile.Temperature)
				}
			}
		}

		state.Agents = append(state.Agents, as)
	}

	// Pre-cache models for each selected harness to avoid repeated lookups
	modelCache := make(map[string][]string)
	for _, name := range state.SelectedHarness {
		h := state.Registry.Get(name)
		if h == nil {
			continue
		}
		models, err := h.ListModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not list models for %s: %v\n", name, err)
			continue
		}
		modelCache[name] = models
	}

	// Build a form for each agent role
	for i := range state.Agents {
		if err := runSingleAgentForm(state, i, modelCache); err != nil {
			return err
		}
	}

	return nil
}

func runSingleAgentForm(state *State, idx int, modelCache map[string][]string) error {
	agent := &state.Agents[idx]

	// Build harness options (only selected harnesses)
	var harnessOpts []huh.Option[string]
	for _, name := range state.SelectedHarness {
		harnessOpts = append(harnessOpts, huh.NewOption(name, name))
	}

	// Resolve harness adapter; fall back if pre-populated config named an unknown harness
	if h := state.Registry.Get(agent.Harness); h == nil {
		if len(state.SelectedHarness) > 0 {
			agent.Harness = state.SelectedHarness[0]
		}
		if state.Registry.Get(agent.Harness) == nil {
			return fmt.Errorf("no valid harness available for agent %q", agent.Role)
		}
	}

	// --- Build all fields for a single stacked form ---
	var fields []huh.Field

	// Progress note header
	fields = append(fields,
		huh.NewNote().
			Title(fmt.Sprintf("Configure: %s", agent.Role)).
			Description(BuildProgressNote(state.Agents, idx)),
	)

	// Harness select
	fields = append(fields,
		huh.NewSelect[string]().
			Title("Harness").
			Options(harnessOpts...).
			Value(&agent.Harness),
	)

	// Enabled toggle
	fields = append(fields,
		huh.NewConfirm().
			Title("Enabled").
			Value(&agent.Enabled),
	)

	form := huh.NewForm(
		huh.NewGroup(fields...),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return err
	}

	// If disabled, skip model/temp/effort
	if !agent.Enabled {
		return nil
	}

	// Resolve harness after user selection (may have changed)
	h := state.Registry.Get(agent.Harness)
	if h == nil {
		return fmt.Errorf("unknown harness %q for agent %q", agent.Harness, agent.Role)
	}

	// --- Build model + settings form ---
	var settingsFields []huh.Field

	// Updated progress note (harness now chosen)
	settingsFields = append(settingsFields,
		huh.NewNote().
			Title(fmt.Sprintf("Configure: %s (%s)", agent.Role, agent.Harness)).
			Description(BuildProgressNote(state.Agents, idx)),
	)

	// Model select -- filterable with capped height for large lists
	models := modelCache[agent.Harness]
	if len(models) > 1 {
		var modelOpts []huh.Option[string]
		for _, m := range models {
			modelOpts = append(modelOpts, huh.NewOption(m, m))
		}
		settingsFields = append(settingsFields,
			huh.NewSelect[string]().
				Title("Model").
				Options(modelOpts...).
				Value(&agent.Model).
				Height(8).
				Filtering(true),
		)
	} else {
		if agent.Model == "" && len(models) > 0 {
			agent.Model = models[0]
		}
		settingsFields = append(settingsFields,
			huh.NewInput().
				Title("Model").
				Value(&agent.Model),
		)
	}

	// Temperature (if harness supports it)
	if h.SupportsTemperature() {
		settingsFields = append(settingsFields,
			huh.NewInput().
				Title("Temperature (empty = default)").
				Placeholder("e.g. 0.7").
				Value(&agent.Temperature).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					if _, err := strconv.ParseFloat(s, 64); err != nil {
						return fmt.Errorf("must be a number (e.g. 0.7)")
					}
					return nil
				}),
		)
	}

	// Effort (if harness supports it)
	if h.SupportsEffort() {
		levels := h.ListEffortLevels(agent.Model)
		var effortOpts []huh.Option[string]
		for _, lvl := range levels {
			label := lvl
			if label == "" {
				label = "default"
			}
			effortOpts = append(effortOpts, huh.NewOption(label, lvl))
		}
		settingsFields = append(settingsFields,
			huh.NewSelect[string]().
				Title("Effort").
				Options(effortOpts...).
				Value(&agent.Effort),
		)
	}

	settingsForm := huh.NewForm(
		huh.NewGroup(settingsFields...),
	).WithTheme(huh.ThemeCharm())

	return settingsForm.Run()
}
