package wizard

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/kastheco/kasmos/config"
)

// FormatAgentSummary returns a one-line summary of an agent's settings.
func FormatAgentSummary(a AgentState) string {
	if !a.Enabled {
		return "(disabled)"
	}
	var parts []string
	if a.Harness != "" {
		parts = append(parts, a.Harness)
	}
	if a.Model != "" {
		parts = append(parts, a.Model)
	}
	if a.Effort != "" {
		parts = append(parts, a.Effort)
	}
	if a.Temperature != "" {
		parts = append(parts, "temp="+a.Temperature)
	}
	return strings.Join(parts, " / ")
}

func runAgentStage(state *State, existing *config.TOMLConfigResult) error {
	roles := DefaultAgentRoles()
	defaults := RoleDefaults()

	// Initialize agent states with existing values, role defaults, or bare minimums
	defaultHarness := ""
	if len(state.SelectedHarness) > 0 {
		defaultHarness = state.SelectedHarness[0]
	}
	for _, role := range roles {
		// Start from role defaults
		as := defaults[role]
		if as.Role == "" {
			// Unknown role (shouldn't happen) — minimal fallback
			as = AgentState{Role: role, Enabled: true}
		}
		// Set harness to first selected if not already set
		if as.Harness == "" {
			as.Harness = defaultHarness
		}

		// Override with existing config if available
		if existing != nil {
			if profile, ok := existing.Profiles[role]; ok {
				as.Harness = profile.Program
				as.Model = profile.Model
				as.Effort = profile.Effort
				as.Enabled = profile.Enabled
				// Explicitly reset temperature so nil in existing config clears the role default.
				as.Temperature = ""
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

	// Configure each agent via huh forms
	for i := range state.Agents {
		agent := &state.Agents[i]

		// chat role is not configurable in the wizard
		if agent.Role == "chat" {
			continue
		}

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

	// --- Form 1: Summary + Customize gate ---
	// Group 1 always visible: progress note + customize confirm
	// Group 2 conditionally visible: harness select
	var customize bool
	summary := FormatAgentSummary(*agent)
	defHarness := ""
	if len(state.SelectedHarness) > 0 {
		defHarness = state.SelectedHarness[0]
	}
	if IsCustomized(*agent, defHarness) {
		summary = "\033[1m" + summary + "\033[0m"
	}

	gateForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("agent: %s", agent.Role)).
				Description(fmt.Sprintf("%s\n\n%s",
					summary,
					BuildProgressNote(state.Agents, idx))),
			huh.NewConfirm().
				Title("customize?").
				Description("change harness, model, or tuning").
				Affirmative("yes").
				Negative("no").
				Value(&customize),
		),
	).WithTheme(huh.ThemeCharm())

	if err := gateForm.Run(); err != nil {
		return err
	}

	// If not customizing, we're done; otherwise implicitly enable the agent
	if !customize {
		return nil
	}
	agent.Enabled = true

	// Resolve harness adapter based on current selection (used for capability checks below)
	h := state.Registry.Get(agent.Harness)
	if h == nil {
		return fmt.Errorf("unknown harness %q for agent %q", agent.Harness, agent.Role)
	}

	// --- Form 2: Harness + model + settings ---
	var settingsFields []huh.Field

	settingsFields = append(settingsFields,
		huh.NewNote().
			Title(fmt.Sprintf("configure: %s", agent.Role)).
			Description(BuildProgressNote(state.Agents, idx)),
		huh.NewSelect[string]().
			Title("harness").
			Options(harnessOpts...).
			Value(&agent.Harness),
	)

	// Model select — filterable with capped height for large lists
	models := modelCache[agent.Harness]
	if len(models) > 1 {
		var modelOpts []huh.Option[string]
		for _, m := range models {
			modelOpts = append(modelOpts, huh.NewOption(m, m))
		}
		settingsFields = append(settingsFields,
			huh.NewSelect[string]().
				Title("model").
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
				Title("model").
				Value(&agent.Model),
		)
	}

	// Temperature (if harness supports it)
	if h.SupportsTemperature() {
		settingsFields = append(settingsFields,
			huh.NewInput().
				Title("temperature (empty = default)").
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
				Title("effort").
				Options(effortOpts...).
				Value(&agent.Effort),
		)
	}

	settingsForm := huh.NewForm(
		huh.NewGroup(settingsFields...),
	).WithTheme(huh.ThemeCharm())

	return settingsForm.Run()
}
