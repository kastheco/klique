# Wizard UX Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Load the `tui-design` project skill for aesthetic guidance.

**Goal:** Replace the 11-screen wizard with 5 context-preserving screens: one stacked form per agent with a progress Note header, filterable model Selects capped at 8 rows, and ThemeCharm throughout.

**Architecture:** Each agent gets a single `huh.NewForm()` with all fields in one group, a `huh.NewNote()` at the top showing previous answers, and `.Height(8).Filterable(true)` on model Selects. The stage_agents.go rewrite consolidates 3 form.Run() calls per agent into 1. Harness and phase stages get Note headers and theming.

**Tech Stack:** `charmbracelet/huh` v0.8.0 (already in go.mod), `huh.ThemeCharm()`, `huh.NewNote()`, `.Filterable(true)`, `.Height(8)`.

---

### Task 1: Add `buildProgressNote` helper to wizard.go

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go`
- Modify: `internal/initcmd/wizard/wizard_test.go`

**Step 1: Write the failing test**

Add to `wizard_test.go`:

```go
func TestBuildProgressNote(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "opencode", Model: "claude-sonnet-4-6", Effort: "high", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Effort: "high", Enabled: true},
		{Role: "planner", Harness: "codex", Model: "", Effort: "", Enabled: true},
	}

	t.Run("first agent shows current marker", func(t *testing.T) {
		note := BuildProgressNote(agents, 0)
		assert.Contains(t, note, "▸ coder")
		assert.Contains(t, note, "○ reviewer")
		assert.Contains(t, note, "○ planner")
	})

	t.Run("middle agent shows completed first", func(t *testing.T) {
		note := BuildProgressNote(agents, 1)
		assert.Contains(t, note, "✓ coder")
		assert.Contains(t, note, "opencode")
		assert.Contains(t, note, "claude-sonnet-4-6")
		assert.Contains(t, note, "▸ reviewer")
		assert.Contains(t, note, "○ planner")
	})

	t.Run("last agent shows all completed", func(t *testing.T) {
		note := BuildProgressNote(agents, 2)
		assert.Contains(t, note, "✓ coder")
		assert.Contains(t, note, "✓ reviewer")
		assert.Contains(t, note, "▸ planner")
	})

	t.Run("disabled agent shows skip marker", func(t *testing.T) {
		agents[0].Enabled = false
		note := BuildProgressNote(agents, 1)
		assert.Contains(t, note, "⊘ coder")
	})
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/initcmd/wizard/ -run TestBuildProgressNote -v
```

Expected: FAIL — `BuildProgressNote` undefined.

**Step 3: Implement `BuildProgressNote`**

Add to `wizard.go` (after the imports, add `"fmt"` and `"strings"`):

```go
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
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/initcmd/wizard/ -run TestBuildProgressNote -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/wizard.go internal/initcmd/wizard/wizard_test.go
git commit -m "feat(wizard): add BuildProgressNote for agent config progress display"
```

---

### Task 2: Rewrite `runSingleAgentForm` — single stacked form with Note header

This is the core change. Replace the 3 sequential `form.Run()` calls with one form
containing a Note header + all fields.

**Files:**
- Modify: `internal/initcmd/wizard/stage_agents.go:68-185`

**Step 1: Rewrite `runSingleAgentForm`**

Replace the entire function body. The new version:

```go
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

	// Model select — filterable with capped height for large lists
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
				Filterable(true),
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
```

Note: This is 2 form.Run() calls per agent (harness+enabled, then model+temp+effort)
rather than 3. We can't do a true single form because the model list depends on the
harness choice — it must be resolved between forms. But the user sees all settings
fields at once in the second form, which is the key improvement. The Note header
provides context in both screens.

**Step 2: Verify the import for `strconv` is present**

The file already imports `strconv`. Verify:

```bash
rg "strconv" internal/initcmd/wizard/stage_agents.go
```

**Step 3: Build and verify**

```bash
go build ./...
```

Expected: clean.

**Step 4: Commit**

```bash
git add internal/initcmd/wizard/stage_agents.go
git commit -m "feat(wizard): stacked agent form with Note header and filterable model select"
```

---

### Task 3: Add ThemeCharm to harness stage

**Files:**
- Modify: `internal/initcmd/wizard/stage_harness.go:27-34`

**Step 1: Add theme to harness form**

Change the form creation to:

```go
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which agent harnesses do you want to configure?").
				Options(options...).
				Value(&state.SelectedHarness),
		),
	).WithTheme(huh.ThemeCharm())
```

**Step 2: Build and verify**

```bash
go build ./...
```

Expected: clean.

**Step 3: Commit**

```bash
git add internal/initcmd/wizard/stage_harness.go
git commit -m "feat(wizard): apply ThemeCharm to harness selection stage"
```

---

### Task 4: Add Note header and ThemeCharm to phase stage

**Files:**
- Modify: `internal/initcmd/wizard/stage_phases.go:51-68`

**Step 1: Build agent summary for phase stage Note**

The phase stage runs after all agents are configured, so we show the full summary.
Add a Note as the first field, and apply ThemeCharm:

```go
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
```

Note: The `BuildProgressNote` call with `currentIdx = len(state.Agents)` means all
agents show as completed (`✓` or `⊘`), since the current index is past the last agent.

**Step 2: Add `"fmt"` import if missing**

The file already imports `fmt`. Verify the `huh` import is present as well.

**Step 3: Build and verify**

```bash
go build ./...
```

Expected: clean.

**Step 4: Commit**

```bash
git add internal/initcmd/wizard/stage_phases.go
git commit -m "feat(wizard): add agent summary Note and ThemeCharm to phase stage"
```

---

### Task 5: Build verification and manual smoke test

**Step 1: Run full test suite**

```bash
go test ./... -count=1
```

Expected: ALL PASS

**Step 2: Build the binary**

```bash
go build -o kq .
```

Expected: clean.

**Step 3: Manual smoke test**

```bash
./kq init --clean
```

Verify:
1. Harness selection uses ThemeCharm styling
2. Each agent form shows a Note header with progress (✓/▸/○)
3. Model Select shows max 8 rows and is filterable (type to search)
4. All agent settings (harness, enabled, model, temp, effort) visible at once
5. Phase mapping shows full agent summary in Note header
6. Disabled agent shows ⊘ in subsequent Note headers

**Step 4: Run typos check**

```bash
typos internal/initcmd/wizard/
```

Expected: no typos.

**Step 5: Commit**

```bash
git add -A
git commit -m "feat(wizard): UX overhaul — stacked forms, progress notes, filterable models"
```
