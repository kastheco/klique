# Wizard TUI Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the sequential huh-form init wizard with a full-screen bubbletea TUI that feels like a guided, polished onboarding experience using the existing Rosé Pine Moon aesthetic.

**Architecture:** Single `tea.Program` with `tea.WithAltScreen()` running three step sub-models (harness selection, agent configuration, review & apply). Each step is a self-contained sub-model implementing a shared `Step` interface; the root model delegates Update/View to the active step. The wizard package is fully self-contained — it redefines the Rosé Pine Moon palette locally (same hex values, no imports from `ui/`).

**Tech Stack:** bubbletea v1.3, lipgloss v1.1, bubbles (textinput, list), existing harness registry API

---

## File Map

```
internal/initcmd/wizard/
├── wizard.go              # KEEP: State, AgentState, ToTOMLConfig, ToAgentConfigs, RoleDefaults
├── wizard_test.go         # KEEP: all existing tests pass unchanged
├── stage_agents.go        # DELETE: replaced by model_agents.go
├── stage_harness.go       # DELETE: replaced by model_harness.go
├── styles.go              # CREATE: palette constants + lipgloss styles
├── roles.go               # CREATE: role metadata (descriptions, phase map text)
├── model.go               # CREATE: root bubbletea model, step routing
├── model_harness.go       # CREATE: step 1 sub-model
├── model_agents.go        # CREATE: step 2 sub-model (browse + edit modes)
├── model_review.go        # CREATE: step 3 sub-model
├── model_test.go          # CREATE: unit tests for sub-models (msg→state transitions)
├── model_harness_test.go  # CREATE: step 1 tests
├── model_agents_test.go   # CREATE: step 2 tests
└── model_review_test.go   # CREATE: step 3 tests
```

**Files outside wizard/ that change:**
- `internal/initcmd/initcmd.go` — no changes (it calls `wizard.Run(registry, existing)` which keeps the same signature)
- `main.go` — no changes

## Design Reference

The mockups from the design session are the source of truth. Key points:

- **Screen 1 (Harness):** Centered layout. Gradient KASMOS banner at top. List of 3 harnesses with `◉/○` toggles, one-line capability descriptions, detected paths. `›` cursor in iris.
- **Screen 2 (Agents):** Master-detail. Left: role list (~30 chars) with `●/○` status dots and harness names. Right: contextual info in browse mode, interactive form in edit mode. Thin `┊` separator.
- **Screen 3 (Review):** Full-width centered. Summary card in `Surface` background showing all configured agents. Config/scaffold paths. Enter to apply.

---

### Task 1: Palette & Styles Foundation

**Files:**
- Create: `internal/initcmd/wizard/styles.go`
- Test: `internal/initcmd/wizard/styles_test.go` (optional — style structs are declarative)

**Step 1: Create styles.go with palette and style definitions**

```go
package wizard

import "github.com/charmbracelet/lipgloss"

// Rosé Pine Moon — self-contained copy (no ui/ import).
var (
	colorBase    = lipgloss.Color("#232136")
	colorSurface = lipgloss.Color("#2a273f")
	colorOverlay = lipgloss.Color("#393552")
	colorMuted   = lipgloss.Color("#6e6a86")
	colorSubtle  = lipgloss.Color("#908caa")
	colorText    = lipgloss.Color("#e0def4")

	colorLove = lipgloss.Color("#eb6f92")
	colorGold = lipgloss.Color("#f6c177")
	colorRose = lipgloss.Color("#ea9a97")
	colorPine = lipgloss.Color("#3e8fb0")
	colorFoam = lipgloss.Color("#9ccfd8")
	colorIris = lipgloss.Color("#c4a7e7")

	gradientStart = "#9ccfd8"
	gradientEnd   = "#c4a7e7"
)

// Layout styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	subtitleStyle = lipgloss.NewStyle().Foreground(colorMuted)
	separatorStyle = lipgloss.NewStyle().Foreground(colorOverlay)
	hintKeyStyle  = lipgloss.NewStyle().Foreground(colorSubtle)
	hintDescStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Harness list
	harnessSelectedStyle = lipgloss.NewStyle().Foreground(colorIris)
	harnessNormalStyle   = lipgloss.NewStyle().Foreground(colorText)
	harnessDimStyle      = lipgloss.NewStyle().Foreground(colorSubtle)
	harnessDescStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	pathStyle            = lipgloss.NewStyle().Foreground(colorSubtle)

	// Agent list (left panel)
	roleActiveStyle  = lipgloss.NewStyle().Foreground(colorIris)
	roleNormalStyle  = lipgloss.NewStyle().Foreground(colorText)
	roleMutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	dotEnabledStyle  = lipgloss.NewStyle().Foreground(colorFoam)
	dotDisabledStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Detail panel (right)
	labelStyle = lipgloss.NewStyle().Foreground(colorSubtle)
	valueStyle = lipgloss.NewStyle().Foreground(colorText)

	// Review card
	cardStyle = lipgloss.NewStyle().
		Background(colorSurface).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay)

	// Inline field styles
	fieldActiveStyle  = lipgloss.NewStyle().Foreground(colorIris)
	fieldNormalStyle  = lipgloss.NewStyle().Foreground(colorText)
	defaultTagStyle   = lipgloss.NewStyle().Foreground(colorGold)

	// Step indicator
	stepDoneStyle    = lipgloss.NewStyle().Foreground(colorFoam)
	stepActiveStyle  = lipgloss.NewStyle().Foreground(colorIris)
	stepPendingStyle = lipgloss.NewStyle().Foreground(colorOverlay)
)
```

**Step 2: Verify it compiles**

Run: `go build ./internal/initcmd/wizard/`
Expected: no errors

**Step 3: Commit**

```bash
git add internal/initcmd/wizard/styles.go
git commit -m "feat(wizard): add self-contained Rosé Pine Moon palette and lipgloss styles"
```

---

### Task 2: Role Metadata

**Files:**
- Create: `internal/initcmd/wizard/roles.go`
- Test: `internal/initcmd/wizard/roles_test.go`

**Step 1: Write the test**

```go
func TestRoleDescription(t *testing.T) {
	desc := RoleDescription("coder")
	assert.Contains(t, desc, "implementation")

	desc = RoleDescription("unknown")
	assert.Equal(t, "", desc)
}

func TestRolePhaseText(t *testing.T) {
	text := RolePhaseText("coder")
	assert.Contains(t, text, "implementing")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestRoleDescription -v`
Expected: FAIL — `RoleDescription` undefined

**Step 3: Implement roles.go**

```go
package wizard

// RoleDescription returns a human-readable description for a known agent role.
func RoleDescription(role string) string {
	descs := map[string]string{
		"coder":    "Handles implementation tasks. Receives code-level instructions,\nwrites and edits files, runs tests.",
		"reviewer": "Reviews code for correctness, style, and architecture.\nProvides structured feedback before merge.",
		"planner":  "Breaks features into implementation plans.\nDecomposes specs into ordered tasks with file paths and tests.",
		"chat":     "General-purpose assistant for questions and exploration.\nAuto-configured for all selected harnesses.",
	}
	return descs[role]
}

// RolePhaseText returns which workflow phases map to this role.
func RolePhaseText(role string) string {
	phases := map[string]string{
		"coder":    "Default for phases: implementing",
		"reviewer": "Default for phases: spec_review, quality_review",
		"planner":  "Default for phases: planning",
		"chat":     "Available in all phases (ad-hoc)",
	}
	return phases[role]
}

// HarnessDescription returns a one-line summary for a known harness.
func HarnessDescription(name string) string {
	descs := map[string]string{
		"claude":   "Anthropic Claude Code · effort levels · MCP plugins",
		"opencode": "Multi-provider agent · temperature · effort · all models",
		"codex":    "OpenAI Codex CLI · temperature · effort",
	}
	return descs[name]
}

// HarnessCapabilities returns a capabilities list for the detail panel.
func HarnessCapabilities(name string) []string {
	caps := map[string][]string{
		"claude":   {"Model selection", "Effort levels", "MCP plugin support", "No temperature control"},
		"opencode": {"Model selection (50+ models)", "Temperature control", "Effort levels", "Provider-agnostic"},
		"codex":    {"Model selection", "Temperature control", "Effort levels", "Reasoning effort config"},
	}
	return caps[name]
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestRole -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/roles.go internal/initcmd/wizard/roles_test.go
git commit -m "feat(wizard): add role and harness metadata for TUI detail panels"
```

---

### Task 3: Root Model & Step Interface

**Files:**
- Create: `internal/initcmd/wizard/model.go`
- Create: `internal/initcmd/wizard/model_test.go`

This is the spine: the root bubbletea model that delegates to step sub-models and handles step transitions.

**Step 1: Write the test**

```go
func TestRootModelStepTransitions(t *testing.T) {
	t.Run("initial step is 0", func(t *testing.T) {
		m := newRootModel(nil, nil)
		assert.Equal(t, 0, m.step)
	})

	t.Run("nextStep advances and caps at maxStep", func(t *testing.T) {
		m := newRootModel(nil, nil)
		m.totalSteps = 3
		m.step = 1
		m.nextStep()
		assert.Equal(t, 2, m.step)
		m.nextStep()
		assert.Equal(t, 2, m.step) // capped
	})

	t.Run("prevStep decrements and floors at 0", func(t *testing.T) {
		m := newRootModel(nil, nil)
		m.step = 1
		m.prevStep()
		assert.Equal(t, 0, m.step)
		m.prevStep()
		assert.Equal(t, 0, m.step) // floored
	})
}

func TestStepIndicator(t *testing.T) {
	// Visual output test — just check it doesn't panic and has expected structure
	indicator := renderStepIndicator(1, 3)
	assert.Contains(t, indicator, "●")
	assert.Contains(t, indicator, "○")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestRootModel -v`
Expected: FAIL — `newRootModel` undefined

**Step 3: Implement model.go**

The root model holds wizard `*State`, a `step` int, window dimensions, and an array of step sub-models. Each sub-model implements:

```go
type stepModel interface {
	Init() tea.Cmd
	Update(tea.Msg) (stepModel, tea.Cmd)
	View(width, height int) string
	// Result populates wizard State when step completes
	Apply(state *State)
}
```

The root model:
- Initializes all 3 step sub-models in `Init()`
- Delegates `Update`/`View` to `steps[step]`
- Listens for `stepDoneMsg` to advance, `stepBackMsg` to retreat
- Listens for `tea.WindowSizeMsg` to track dimensions
- On final step done, calls `tea.Quit`

The `Run()` function changes from sequential huh forms to:
```go
func Run(registry *harness.Registry, existing *config.TOMLConfigResult) (*State, error) {
    m := newRootModel(registry, existing)
    p := tea.NewProgram(m, tea.WithAltScreen())
    finalModel, err := p.Run()
    if err != nil {
        return nil, err
    }
    rm := finalModel.(rootModel)
    if rm.cancelled {
        return nil, fmt.Errorf("wizard cancelled")
    }
    return rm.state, nil
}
```

Include `renderStepIndicator(current, total int) string` — renders `● ── ● ── ○` with color from styles, and step labels below ("Harness", "Agents", "Done").

Include a gradient text helper (copy the 15-line `gradientText` function from `ui/gradient.go` — we're self-contained) for the banner.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestRootModel -v`
Expected: PASS

**Step 5: Verify the package still compiles with existing code**

Run: `go build ./internal/initcmd/wizard/`
Expected: compiles (the old stage_*.go files still exist but are unused by the new `Run()`)

**Step 6: Commit**

```bash
git add internal/initcmd/wizard/model.go internal/initcmd/wizard/model_test.go
git commit -m "feat(wizard): add root bubbletea model with step routing and gradient banner"
```

---

### Task 4: Step 1 — Harness Selection Sub-Model

**Files:**
- Create: `internal/initcmd/wizard/model_harness.go`
- Create: `internal/initcmd/wizard/model_harness_test.go`

**Step 1: Write the tests**

```go
func TestHarnessStep_Toggle(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
		{Name: "opencode", Path: "/usr/bin/opencode", Found: true},
		{Name: "codex", Path: "", Found: false},
	})

	assert.True(t, h.selected["claude"])
	assert.True(t, h.selected["opencode"])
	assert.False(t, h.selected["codex"])

	// Toggle claude off
	h.cursor = 0
	h.toggle()
	assert.False(t, h.selected["claude"])

	// Toggle codex on
	h.cursor = 2
	h.toggle()
	assert.True(t, h.selected["codex"])
}

func TestHarnessStep_CannotProceedEmpty(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
	})
	h.selected["claude"] = false
	assert.False(t, h.canProceed())
}

func TestHarnessStep_SelectedNames(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
		{Name: "opencode", Path: "", Found: false},
		{Name: "codex", Path: "/usr/bin/codex", Found: true},
	})
	// opencode not found, not selected by default
	names := h.selectedNames()
	assert.Equal(t, []string{"claude", "codex"}, names)
}

func TestHarnessStep_CursorNavigation(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "a", Found: true},
		{Name: "b", Found: true},
		{Name: "c", Found: false},
	})
	assert.Equal(t, 0, h.cursor)

	h.cursorDown()
	assert.Equal(t, 1, h.cursor)

	h.cursorDown()
	assert.Equal(t, 2, h.cursor)

	h.cursorDown() // should clamp
	assert.Equal(t, 2, h.cursor)

	h.cursorUp()
	assert.Equal(t, 1, h.cursor)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestHarnessStep -v`
Expected: FAIL

**Step 3: Implement model_harness.go**

The harness step sub-model:
- Holds `items []harness.DetectResult`, `selected map[string]bool`, `cursor int`
- `Init()` — no commands needed
- `Update()`:
  - `j/↓` → `cursorDown()`, `k/↑` → `cursorUp()`
  - `space` → `toggle()` selected state
  - `enter` → if `canProceed()`, return `stepDoneMsg`
  - `q/ctrl+c` → return cancel msg
- `View(width, height)`:
  - Renders gradient banner centered at top
  - Step indicator: `● ── ○ ── ○` below banner
  - Section title: "Select Agent Harnesses"
  - For each harness: `› ◉ claude /usr/bin/claude` or `○ codex not found`
  - Below each: one-line capability description from `HarnessDescription()`
  - Key hints at bottom: `space toggle · enter continue · q quit`
  - All content centered horizontally with `lipgloss.Place()` or manual padding
- `Apply(state)` — sets `state.SelectedHarness` and `state.DetectResults`

The banner is rendered using the gradient helper from model.go. It's only shown on step 1 (steps 2-3 use a compact header).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestHarnessStep -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/model_harness.go internal/initcmd/wizard/model_harness_test.go
git commit -m "feat(wizard): implement harness selection TUI step with toggle and navigation"
```

---

### Task 5: Step 2 — Agent Configuration Sub-Model (Browse Mode)

**Files:**
- Create: `internal/initcmd/wizard/model_agents.go`
- Create: `internal/initcmd/wizard/model_agents_test.go`

This is the largest task. The agent step has two modes: **browse** (navigate roles, see summary) and **edit** (configure a specific role). This task covers browse mode; Task 6 covers edit mode.

**Step 1: Write the tests**

```go
func TestAgentStep_BrowseNavigation(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "gpt-5.3-codex", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, nil)
	assert.Equal(t, 0, s.cursor)
	assert.Equal(t, agentBrowseMode, s.mode)

	s.cursorDown()
	assert.Equal(t, 1, s.cursor)

	s.cursorDown()
	assert.Equal(t, 2, s.cursor)

	s.cursorDown() // chat is skipped in navigation
	assert.Equal(t, 2, s.cursor) // clamped at planner
}

func TestAgentStep_ToggleEnabled(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Enabled: true},
		{Role: "reviewer", Harness: "claude", Enabled: true},
		{Role: "planner", Harness: "claude", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.cursor = 0
	s.toggleEnabled()
	assert.False(t, s.agents[0].Enabled)
	s.toggleEnabled()
	assert.True(t, s.agents[0].Enabled)
}

func TestAgentStep_DetailPanelContent(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Temperature: "0.1", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	detail := s.renderDetailPanel(60, 20)
	assert.Contains(t, detail, "CODER")
	assert.Contains(t, detail, "claude-sonnet-4-6")
	assert.Contains(t, detail, "medium")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStep -v`
Expected: FAIL

**Step 3: Implement browse mode in model_agents.go**

The agent step sub-model:
- Holds `agents []AgentState` (copy from wizard State), `cursor int`, `mode agentMode`, `harnesses []string`, `modelCache map[string][]string`
- `agentMode` enum: `agentBrowseMode`, `agentEditMode`
- Navigation only over indices 0-2 (skip chat at index 3)
- `View()` renders master-detail:
  - Left panel (~30 chars): `ROLES` header, then for each navigable role: `● rolename   harness`
  - `›` cursor on focused role, iris color
  - Right panel (remaining width): role description, current settings table, phase text
  - `┊` separator between panels (overlay color)
  - Key hints: `j/k navigate · enter edit · space toggle · tab next step · q quit`

Use `lipgloss.JoinHorizontal()` for the two-panel layout. Calculate widths from the available `width` parameter: left = `min(32, width/3)`, right = `width - left - 1` (1 for separator).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStep -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/model_agents.go internal/initcmd/wizard/model_agents_test.go
git commit -m "feat(wizard): implement agent configuration browse mode with master-detail layout"
```

---

### Task 6: Step 2 — Agent Configuration Sub-Model (Edit Mode)

**Files:**
- Modify: `internal/initcmd/wizard/model_agents.go`
- Modify: `internal/initcmd/wizard/model_agents_test.go`

**Step 1: Write the tests**

```go
func TestAgentStep_EnterEditMode(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}
	modelCache := map[string][]string{
		"claude": {"claude-sonnet-4-6", "claude-opus-4-6", "claude-sonnet-4-5", "claude-haiku-4-5"},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, modelCache)
	s.enterEditMode()
	assert.Equal(t, agentEditMode, s.mode)
	assert.Equal(t, 0, s.editField) // first field focused
}

func TestAgentStep_EditFieldCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, nil)
	s.enterEditMode()

	s.nextField()
	assert.Equal(t, 1, s.editField) // model

	s.nextField()
	assert.Equal(t, 2, s.editField) // effort

	// Continue cycling...
}

func TestAgentStep_ExitEditMode(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.enterEditMode()
	s.exitEditMode()
	assert.Equal(t, agentBrowseMode, s.mode)
}

func TestAgentStep_HarnessCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Enabled: true},
	}
	harnesses := []string{"claude", "opencode"}

	s := newAgentStep(agents, harnesses, nil)
	s.enterEditMode()
	s.editField = 0 // harness field

	s.cycleFieldValue(1) // next
	assert.Equal(t, "opencode", s.agents[0].Harness)

	s.cycleFieldValue(1) // wraps
	assert.Equal(t, "claude", s.agents[0].Harness)

	s.cycleFieldValue(-1) // prev wraps
	assert.Equal(t, "opencode", s.agents[0].Harness)
}

func TestAgentStep_EffortCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.effortLevels = map[string][]string{"claude": {"", "low", "medium", "high", "max"}}
	s.enterEditMode()
	s.editField = 2 // effort field

	s.cycleFieldValue(1) // medium → high
	assert.Equal(t, "high", s.agents[0].Effort)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStep_Enter -v`
Expected: FAIL

**Step 3: Implement edit mode**

Add to `model_agents.go`:

**Edit mode fields** (top to bottom on the right panel):
1. **Harness** — inline cycle: `‹ claude ›` — left/right arrows cycle through `harnesses`
2. **Model** — filterable list in a rounded box. When this field is focused, render a `bubbles/list` or custom filter list showing models for the current harness. Type to filter. Enter to select. The list pulls from `modelCache[harness]`.
3. **Effort** — inline cycle: `‹ medium ›` — pulls from harness `ListEffortLevels(model)`
4. **Temperature** — `bubbles/textinput` inline, only shown if harness supports it

**Update routing in edit mode:**
- `tab` → next field (wraps)
- `shift+tab` → previous field
- `h/←` and `l/→` on cycle fields → cycle value
- On model field: `j/k` or `↑/↓` navigate model list, `/` activates filter, `enter` selects
- `esc` → exit edit mode back to browse
- `enter` on last field → exit edit mode

**View in edit mode:**
- Left panel: same as browse, but focused role shows `◂` editing indicator
- Right panel: `ROLENAME ── editing` header, then stacked form fields with active field highlighted in iris

For the model selector, use a lightweight custom implementation rather than full `bubbles/list`:
- Hold `allModels []string`, `filteredModels []string`, `filterText string`, `modelCursor int`, `filtering bool`
- When filtering, show a `/ filterText█` input at top of the list
- Show max 6 models at a time (scrollable)

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStep -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/model_agents.go internal/initcmd/wizard/model_agents_test.go
git commit -m "feat(wizard): implement agent edit mode with harness/model/effort/temp fields"
```

---

### Task 7: Step 3 — Review & Apply Sub-Model

**Files:**
- Create: `internal/initcmd/wizard/model_review.go`
- Create: `internal/initcmd/wizard/model_review_test.go`

**Step 1: Write the test**

```go
func TestReviewStep_RendersSummary(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Temperature: "0.1", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "gpt-5.3-codex", Effort: "xhigh", Temperature: "0.2", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "claude-opus-4-6", Effort: "max", Temperature: "0.5", Enabled: false},
	}

	r := newReviewStep(agents, []string{"claude", "opencode"})
	view := r.View(100, 36)
	assert.Contains(t, view, "claude-sonnet-4-6")
	assert.Contains(t, view, "gpt-5.3-codex")
	assert.Contains(t, view, "disabled") // planner
}

func TestReviewStep_FormatSummaryLine(t *testing.T) {
	a := AgentState{
		Role: "coder", Harness: "claude",
		Model: "claude-sonnet-4-6", Effort: "medium",
		Temperature: "0.1", Enabled: true,
	}
	line := formatReviewLine(a)
	assert.Contains(t, line, "claude")
	assert.Contains(t, line, "claude-sonnet-4-6")
	assert.Contains(t, line, "medium")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestReviewStep -v`
Expected: FAIL

**Step 3: Implement model_review.go**

The review step:
- Read-only — no editing, just display
- Renders: step indicator (all dots done/active), "Review Configuration" title, separator
- Shows "Harnesses: claude opencode" in foam
- Summary card (surface bg, rounded border) with each agent:
  - `● coder   claude / claude-sonnet-4-6 / medium / temp 0.1`
  - `○ planner  (disabled)`
- Shows config and scaffold paths below the card
- Key hints: `enter apply · esc go back · q quit`
- `enter` → `stepDoneMsg`, `esc` → `stepBackMsg`, `q` → cancel

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestReviewStep -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/model_review.go internal/initcmd/wizard/model_review_test.go
git commit -m "feat(wizard): implement review & apply TUI step with summary card"
```

---

### Task 8: Wire Everything Together

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go` — update `Run()` function
- Delete: `internal/initcmd/wizard/stage_agents.go`
- Delete: `internal/initcmd/wizard/stage_harness.go`

**Step 1: Write an integration-style test**

```go
func TestRunReturnsCorrectState(t *testing.T) {
	// We can't easily run a full tea.Program in tests, but we can verify
	// that the root model produces the correct State output when steps complete.
	// This tests the Apply chain.
	m := newRootModel(nil, nil)
	m.state = &State{}

	// Simulate harness step result
	m.state.SelectedHarness = []string{"claude", "opencode"}
	m.state.DetectResults = []harness.DetectResult{
		{Name: "claude", Found: true},
		{Name: "opencode", Found: true},
	}

	// Simulate agent step result
	m.state.Agents = []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Temperature: "0.1", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "gpt-5.3-codex", Effort: "xhigh", Temperature: "0.2", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "claude-opus-4-6", Effort: "max", Temperature: "0.5", Enabled: true},
		{Role: "chat", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "high", Temperature: "0.3", Enabled: true},
	}

	// Verify TOML conversion still works (existing test but via new flow)
	tc := m.state.ToTOMLConfig()
	assert.NotNil(t, tc)
	assert.Len(t, tc.Agents, 4)
	assert.Equal(t, "claude", tc.Agents["coder"].Program)
}
```

**Step 2: Run all existing tests to verify they still pass**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: all existing tests PASS (they test State/AgentState directly, not the huh forms)

**Step 3: Update wizard.go Run() function**

Replace the current `Run()` body (lines 122-150) with the bubbletea program launch:

```go
func Run(registry *harness.Registry, existing *config.TOMLConfigResult) (*State, error) {
	m := newRootModel(registry, existing)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	rm := finalModel.(rootModel)
	if rm.cancelled {
		return nil, fmt.Errorf("wizard cancelled")
	}
	return rm.state, nil
}
```

Remove `BuildProgressNote()` — it was only used by the huh forms. The progress display is now in the step indicator and the role list.

Keep everything else in wizard.go unchanged: `State`, `AgentState`, `DefaultAgentRoles()`, `RoleDefaults()`, `IsCustomized()`, `parseTemperature()`, `ToTOMLConfig()`, `ToAgentConfigs()`.

**Step 4: Delete old stage files**

```bash
git rm internal/initcmd/wizard/stage_agents.go
git rm internal/initcmd/wizard/stage_harness.go
```

**Step 5: Update wizard_test.go**

Remove `TestBuildProgressNote` and `TestFormatAgentSummary` since those functions are deleted. All other tests (`TestStateToTOMLConfig`, `TestStateToAgentConfigs`, `TestDefaultAgentRoles`, `TestRoleDefaults`, `TestIsCustomized`, `TestPrePopulateFromExisting`) remain unchanged.

**Step 6: Run all tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: all PASS

Run: `go test ./internal/initcmd/... -v`
Expected: all PASS (initcmd.go is unchanged, just calls wizard.Run())

**Step 7: Full build check**

Run: `go build ./...`
Expected: clean build, no errors

**Step 8: Commit**

```bash
git add -A internal/initcmd/wizard/
git commit -m "feat(wizard): wire bubbletea TUI, remove old huh form stages"
```

---

### Task 9: Manual QA & Polish

**Files:**
- Modify: various files in `internal/initcmd/wizard/` for visual polish

This task is iterative visual tuning. Run the wizard and fix layout issues.

**Step 1: Run the wizard**

Run: `go run . init --clean`

**Step 2: Check each screen**

Verify against mockups:
- [ ] Screen 1: Banner centered? Harness list aligned? Toggle works? Path detection shows?
- [ ] Screen 2 browse: Role list navigable? Detail panel shows correct role info? `●/○` dots correct?
- [ ] Screen 2 edit: Harness cycles? Model list filters? Effort cycles? Temp input works?
- [ ] Screen 3: Summary card renders? All agents shown? Config path correct?
- [ ] Navigation: Esc goes back between steps? q quits? enter advances?
- [ ] Edge cases: narrow terminal (<80 cols)? Only 1 harness selected? All agents disabled?

**Step 3: Fix any visual/layout issues found**

Common fixes needed:
- Centering math off by 1
- Text truncation for long model names
- Model list scroll position after filtering
- Separator alignment

**Step 4: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 5: Commit**

```bash
git add -A internal/initcmd/wizard/
git commit -m "fix(wizard): visual polish and layout alignment fixes"
```

---

### Task 10: Pre-populate from Existing Config

**Files:**
- Modify: `internal/initcmd/wizard/model.go`
- Modify: `internal/initcmd/wizard/model_agents.go`
- Test already exists in `wizard_test.go` (`TestPrePopulateFromExisting`)

The existing wizard pre-populates agent settings from `~/.config/kasmos/config.toml` when re-running `kas init`. This must work in the new TUI.

**Step 1: Write the test**

```go
func TestAgentStepPrePopulatesFromExisting(t *testing.T) {
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
	}

	agents := initAgentsFromExisting([]string{"claude", "opencode"}, existing)
	assert.Equal(t, "opencode", agents[0].Harness) // coder from existing
	assert.Equal(t, "high", agents[0].Effort)
	assert.Equal(t, "0.5", agents[0].Temperature)
	// reviewer gets defaults
	assert.Equal(t, "openai/gpt-5.3-codex", agents[1].Model)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStepPrePopulates -v`
Expected: FAIL

**Step 3: Implement**

Extract the agent initialization logic from the old `runAgentStage()` into a standalone function `initAgentsFromExisting(harnesses []string, existing *config.TOMLConfigResult) []AgentState` that the root model calls during setup. This is the same logic that already exists — just moved out of the huh-form context into a reusable function.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestAgentStepPrePopulates -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/model.go internal/initcmd/wizard/model_agents.go internal/initcmd/wizard/model_agents_test.go
git commit -m "feat(wizard): pre-populate agent settings from existing config on re-init"
```

---

## Dependency Order

```
Task 1 (styles) ──┐
Task 2 (roles)  ──┼── Task 3 (root model) ── Task 4 (harness step) ──┐
                  │                                                    │
                  │   Task 5 (agents browse) ── Task 6 (agents edit) ──┼── Task 8 (wire) ── Task 9 (QA)
                  │                                                    │
                  └── Task 7 (review step) ────────────────────────────┘
                                                                       │
                                                        Task 10 (prepopulate) ── after Task 8
```

Tasks 1 and 2 are independent and can be done in parallel.
Tasks 4, 5, and 7 can be done in parallel after Task 3.
Task 6 depends on Task 5.
Task 8 depends on Tasks 4, 6, and 7.
Task 9 depends on Task 8.
Task 10 can be done after Task 8 (or folded into Task 8 if preferred).
