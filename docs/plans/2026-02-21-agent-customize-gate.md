# Agent Customize Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `customize? y[n]` inline prompt before each agent's configuration form in `kq init`, so users can skip agents that already have sensible defaults.

**Architecture:** Replace the unconditional `runSingleAgentForm()` call per agent with a summary display + raw stdin `y[n]` prompt. On `n`/Enter, keep the pre-populated defaults. On `y`, open the existing huh forms. Add per-role hardcoded defaults for fresh inits.

**Tech Stack:** Go, `bufio`, `fmt`, existing `huh` forms unchanged

---

### Task 1: Add per-role default settings

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go`
- Test: `internal/initcmd/wizard/wizard_test.go`

**Step 1: Write the failing test**

Add to `wizard_test.go`:

```go
func TestRoleDefaults(t *testing.T) {
	defaults := RoleDefaults()

	t.Run("has all three roles", func(t *testing.T) {
		assert.Contains(t, defaults, "coder")
		assert.Contains(t, defaults, "reviewer")
		assert.Contains(t, defaults, "planner")
	})

	t.Run("coder defaults", func(t *testing.T) {
		c := defaults["coder"]
		assert.Equal(t, "anthropic/claude-sonnet-4-6", c.Model)
		assert.Equal(t, "medium", c.Effort)
		assert.Equal(t, "0.1", c.Temperature)
		assert.True(t, c.Enabled)
	})

	t.Run("planner defaults", func(t *testing.T) {
		p := defaults["planner"]
		assert.Equal(t, "anthropic/claude-opus-4-6", p.Model)
		assert.Equal(t, "max", p.Effort)
		assert.Equal(t, "0.5", p.Temperature)
	})

	t.Run("reviewer defaults", func(t *testing.T) {
		r := defaults["reviewer"]
		assert.Equal(t, "openai/gpt-5.3-codex", r.Model)
		assert.Equal(t, "xhigh", r.Effort)
		assert.Equal(t, "0.2", r.Temperature)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestRoleDefaults -v`
Expected: FAIL — `RoleDefaults` undefined

**Step 3: Write the implementation**

Add to `wizard.go`:

```go
// RoleDefaults returns sensible per-role defaults for fresh inits.
// Harness is left empty — filled by the caller from selected harnesses.
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
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestRoleDefaults -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/wizard.go internal/initcmd/wizard/wizard_test.go
git commit -m "feat(wizard): add per-role default agent settings"
```

---

### Task 2: Add formatAgentSummary helper

**Files:**
- Modify: `internal/initcmd/wizard/stage_agents.go`
- Test: `internal/initcmd/wizard/wizard_test.go`

**Step 1: Write the failing test**

Add to `wizard_test.go`:

```go
func TestFormatAgentSummary(t *testing.T) {
	t.Run("full settings", func(t *testing.T) {
		a := AgentState{
			Role: "coder", Harness: "opencode",
			Model: "anthropic/claude-sonnet-4-6",
			Effort: "medium", Temperature: "0.1", Enabled: true,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "opencode")
		assert.Contains(t, s, "anthropic/claude-sonnet-4-6")
		assert.Contains(t, s, "medium")
		assert.Contains(t, s, "temp=0.1")
	})

	t.Run("no temperature", func(t *testing.T) {
		a := AgentState{
			Role: "coder", Harness: "claude",
			Model: "claude-sonnet-4-6",
			Effort: "high", Temperature: "", Enabled: true,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "claude")
		assert.Contains(t, s, "claude-sonnet-4-6")
		assert.Contains(t, s, "high")
		assert.NotContains(t, s, "temp=")
	})

	t.Run("disabled", func(t *testing.T) {
		a := AgentState{
			Role: "planner", Harness: "codex", Enabled: false,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "disabled")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestFormatAgentSummary -v`
Expected: FAIL — `FormatAgentSummary` undefined

**Step 3: Write the implementation**

Add to `stage_agents.go`:

```go
// FormatAgentSummary returns a one-line summary of an agent's settings.
func FormatAgentSummary(a AgentState) string {
	if !a.Enabled {
		return "(disabled)"
	}
	parts := []string{a.Harness}
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
```

Make sure `"strings"` is in the import block of `stage_agents.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestFormatAgentSummary -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/stage_agents.go internal/initcmd/wizard/wizard_test.go
git commit -m "feat(wizard): add agent summary formatter"
```

---

### Task 3: Add promptCustomize function

**Files:**
- Modify: `internal/initcmd/wizard/stage_agents.go`
- Test: `internal/initcmd/wizard/wizard_test.go`

This function accepts `io.Reader` for testability but defaults to `os.Stdin` at the call site.

**Step 1: Write the failing test**

Add to `wizard_test.go`:

```go
func TestPromptCustomize(t *testing.T) {
	t.Run("empty input (Enter) returns false", func(t *testing.T) {
		r := strings.NewReader("\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("n returns false", func(t *testing.T) {
		r := strings.NewReader("n\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("N returns false", func(t *testing.T) {
		r := strings.NewReader("N\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("y returns true", func(t *testing.T) {
		r := strings.NewReader("y\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.True(t, result)
	})

	t.Run("Y returns true", func(t *testing.T) {
		r := strings.NewReader("Y\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.True(t, result)
	})

	t.Run("junk defaults to false", func(t *testing.T) {
		r := strings.NewReader("hello\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})
}
```

Add `"strings"` and `"io"` to the test imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestPromptCustomize -v`
Expected: FAIL — `PromptCustomize` undefined

**Step 3: Write the implementation**

Add to `stage_agents.go`:

```go
// PromptCustomize prints an agent summary and asks "customize? y[n]".
// Returns true only if the user types "y" or "Y". Default (Enter) is no.
func PromptCustomize(r io.Reader, w io.Writer, role string, summary string) bool {
	fmt.Fprintf(w, "  %-10s %s\n", role, summary)
	fmt.Fprintf(w, "  customize? y[n]: ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(answer, "y")
}
```

Add `"bufio"`, `"io"`, and `"strings"` to the import block (some may already be there).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestPromptCustomize -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/stage_agents.go internal/initcmd/wizard/wizard_test.go
git commit -m "feat(wizard): add customize? y[n] prompt function"
```

---

### Task 4: Wire the gate into runAgentStage

**Files:**
- Modify: `internal/initcmd/wizard/stage_agents.go`

**Step 1: Update agent initialization to use RoleDefaults**

Replace the initialization loop in `runAgentStage` (lines 16-41). The new logic:
1. Get `RoleDefaults()` for fallback values
2. For each role, populate from existing config first, then role defaults, then bare minimum

Replace the body of `runAgentStage` with:

```go
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
				if profile.Temperature != nil {
					as.Temperature = fmt.Sprintf("%g", *profile.Temperature)
				}
			}
		}

		state.Agents = append(state.Agents, as)
	}

	// Pre-cache models for each selected harness
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

	// Gate each agent with customize? prompt
	fmt.Println("\nAgent configuration:")
	for i := range state.Agents {
		agent := &state.Agents[i]
		summary := FormatAgentSummary(*agent)

		if PromptCustomize(os.Stdin, os.Stdout, agent.Role, summary) {
			if err := runSingleAgentForm(state, i, modelCache); err != nil {
				return err
			}
		}
	}

	return nil
}
```

**Step 2: Run all wizard tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: All existing tests pass. The `TestPrePopulateFromExisting` test still passes because it tests the same pre-population logic (existing config overrides defaults).

**Step 3: Build and verify no compile errors**

Run: `go build ./...`
Expected: Clean build

**Step 4: Manual smoke test**

Run: `go run . init --clean`
Expected:
- After harness selection, see agent summaries with default settings
- Each agent shows `customize? y[n]: ` prompt
- Enter/n skips to next agent
- y opens the full huh form
- Flow continues to phases and tools stages

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/stage_agents.go
git commit -m "feat(wizard): gate agent config with customize? y[n] prompt"
```

---

### Task 5: Update existing tests for new initialization

**Files:**
- Modify: `internal/initcmd/wizard/wizard_test.go`

**Step 1: Update TestPrePopulateFromExisting to use RoleDefaults**

The test manually simulates what `runAgentStage` does. Update it to use `RoleDefaults()` as the base instead of bare `AgentState`:

```go
func TestPrePopulateFromExisting(t *testing.T) {
	temp := 0.5
	existing := &config.TOMLConfigResult{
		Profiles: map[string]config.AgentProfile{
			"coder": {
				Program:     "opencode",
				Model:       "anthropic/claude-sonnet-4-6",
				Temperature: &temp,
				Effort:      "high",
				Enabled:     true,
			},
		},
		PhaseRoles: map[string]string{
			"implementing": "coder",
		},
	}

	// Simulate what runAgentStage now does for pre-population
	roles := DefaultAgentRoles()
	defaults := RoleDefaults()
	var agents []AgentState
	for _, role := range roles {
		as := defaults[role]
		if as.Harness == "" {
			as.Harness = "claude"
		}
		if profile, ok := existing.Profiles[role]; ok {
			as.Harness = profile.Program
			as.Model = profile.Model
			as.Effort = profile.Effort
			as.Enabled = profile.Enabled
			if profile.Temperature != nil {
				as.Temperature = "0.5"
			}
		}
		agents = append(agents, as)
	}

	assert.Equal(t, "opencode", agents[0].Harness) // coder got pre-populated
	assert.Equal(t, "claude", agents[1].Harness)    // reviewer got harness default
	// reviewer still has role defaults for model/effort
	assert.Equal(t, "openai/gpt-5.3-codex", agents[1].Model)
	assert.Equal(t, "xhigh", agents[1].Effort)
}
```

**Step 2: Run all tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: All PASS

**Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/initcmd/wizard/wizard_test.go
git commit -m "test(wizard): update pre-populate test for role defaults"
```
