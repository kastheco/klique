# Solo Agent Mode Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "start solo agent" context menu action that spawns a single coder agent in a worktree with a minimal prompt, bypassing wave orchestration and lifecycle automation.

**Architecture:** Extend the existing `spawnPlanAgent` and `triggerPlanStage` with a `"solo"` stage. Add a `SoloAgent` bool to `Instance` to gate off automatic push/review transitions. The solo agent reuses all existing worktree and branch infrastructure.

**Tech Stack:** Go, bubbletea, existing planstate/planfsm/session packages

---

## Wave 1

### Task 1: Add SoloAgent field to Instance

**Files:**
- Modify: `session/instance.go:33` (Instance struct)

**Step 1: Write the failing test**

Create a test that constructs an Instance and checks SoloAgent defaults to false.

File: `session/instance_test.go`

```go
func TestNewInstance_SoloAgentDefaultsFalse(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:   "test",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	assert.False(t, inst.SoloAgent, "SoloAgent must default to false")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./session/ -run TestNewInstance_SoloAgentDefaultsFalse -v`
Expected: FAIL — `inst.SoloAgent` field does not exist

**Step 3: Add the SoloAgent field**

In `session/instance.go`, add to the Instance struct after the `ImplementationComplete` field (around line 71):

```go
	// SoloAgent is true when this instance was spawned via "start solo agent" — no
	// automatic lifecycle transitions (push prompt, review spawning) apply.
	SoloAgent bool
```

**Step 4: Run test to verify it passes**

Run: `go test ./session/ -run TestNewInstance_SoloAgentDefaultsFalse -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/instance.go session/instance_test.go
git commit -m "feat: add SoloAgent field to Instance"
```

---

### Task 2: Add buildSoloPrompt helper

**Files:**
- Modify: `app/app_state.go` (add function near line 1020, next to buildPlanPrompt/buildImplementPrompt)
- Modify: `app/app_plan_actions_test.go` (add test)

**Step 1: Write the failing test**

File: `app/app_plan_actions_test.go`

```go
func TestBuildSoloPrompt_WithDescription(t *testing.T) {
	prompt := buildSoloPrompt("auth-refactor", "Refactor JWT auth", "2026-02-21-auth-refactor.md")
	assert.Contains(t, prompt, "Implement auth-refactor")
	assert.Contains(t, prompt, "Goal: Refactor JWT auth")
	assert.Contains(t, prompt, "docs/plans/2026-02-21-auth-refactor.md")
}

func TestBuildSoloPrompt_StubOnly(t *testing.T) {
	prompt := buildSoloPrompt("quick-fix", "Fix the login bug", "")
	assert.Contains(t, prompt, "Implement quick-fix")
	assert.Contains(t, prompt, "Goal: Fix the login bug")
	assert.NotContains(t, prompt, "docs/plans/")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBuildSoloPrompt -v`
Expected: FAIL — `buildSoloPrompt` undefined

**Step 3: Implement buildSoloPrompt**

In `app/app_state.go`, add after `buildImplementPrompt`:

```go
// buildSoloPrompt returns a minimal prompt for a solo agent session.
// If planFile is non-empty, it references the plan file. Otherwise just name + description.
func buildSoloPrompt(planName, description, planFile string) string {
	if planFile != "" {
		return fmt.Sprintf("Implement %s. Goal: %s. Plan: docs/plans/%s", planName, description, planFile)
	}
	return fmt.Sprintf("Implement %s. Goal: %s.", planName, description)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestBuildSoloPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app_state.go app/app_plan_actions_test.go
git commit -m "feat: add buildSoloPrompt helper"
```

---

### Task 3: Add "solo" to agentTypeForSubItem

**Files:**
- Modify: `app/app_state.go:1033-1045` (agentTypeForSubItem switch)
- Modify: `app/app_plan_actions_test.go` (extend TestAgentTypeForSubItem)

**Step 1: Update the existing test**

In `app/app_plan_actions_test.go`, add to the `tests` map in `TestAgentTypeForSubItem`:

```go
	"solo": session.AgentTypeCoder,
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestAgentTypeForSubItem -v`
Expected: FAIL — `agentTypeForSubItem("solo")` returns ok=false

**Step 3: Add the solo case**

In `app/app_state.go`, inside `agentTypeForSubItem`, add a case:

```go
	case "implement", "solo":
		return session.AgentTypeCoder, true
```

(Merge the existing `"implement"` case with `"solo"` — both map to AgentTypeCoder.)

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestAgentTypeForSubItem -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app_state.go app/app_plan_actions_test.go
git commit -m "feat: map solo stage to AgentTypeCoder"
```

---

### Task 4: Handle "solo" in spawnPlanAgent — set SoloAgent flag

**Files:**
- Modify: `app/app_state.go:1047-1113` (spawnPlanAgent function)

**Step 1: Write the failing test**

File: `app/app_plan_actions_test.go`

```go
func TestSpawnPlanAgent_SoloSetsSoloAgentFlag(t *testing.T) {
	dir := t.TempDir()

	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	planFile := "2026-02-25-test-solo.md"
	require.NoError(t, ps.Register(planFile, "test solo", "plan/test-solo", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	h := &home{
		planState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		list:               list,
		menu:               ui.NewMenu(),
		sidebar:            ui.NewSidebar(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnPlanAgent(planFile, "solo", "solo prompt")

	instances := list.GetInstances()
	require.NotEmpty(t, instances, "expected instance after spawnPlanAgent(solo)")
	inst := instances[len(instances)-1]
	assert.True(t, inst.SoloAgent, "solo agent must have SoloAgent=true")
	assert.Equal(t, session.AgentTypeCoder, inst.AgentType)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestSpawnPlanAgent_SoloSetsSoloAgentFlag -v`
Expected: FAIL — SoloAgent is false (not set)

**Step 3: Set SoloAgent in spawnPlanAgent**

In `app/app_state.go`, inside `spawnPlanAgent`, after the `if agentType == session.AgentTypeReviewer` block (around line 1074), add:

```go
	if action == "solo" {
		inst.SoloAgent = true
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestSpawnPlanAgent_SoloSetsSoloAgentFlag -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app_state.go app/app_plan_actions_test.go
git commit -m "feat: set SoloAgent flag in spawnPlanAgent for solo stage"
```

---

## Wave 2

### Task 5: Handle "solo" stage in triggerPlanStage

**Files:**
- Modify: `app/app_actions.go:602-700` (triggerPlanStage function)

**Step 1: Add the solo case**

In `app/app_actions.go`, inside `triggerPlanStage`, add a new case in the switch (after the `"plan"` case, before `"implement"`):

```go
	case "solo":
		if err := m.fsmSetImplementing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		// Check if plan .md file exists on disk to decide prompt content.
		planName := planstate.DisplayName(planFile)
		planPath := filepath.Join(m.activeRepoPath, "docs", "plans", planFile)
		refFile := ""
		if _, err := os.Stat(planPath); err == nil {
			refFile = planFile
		}
		prompt := buildSoloPrompt(planName, entry.Description, refFile)
		return m.spawnPlanAgent(planFile, "solo", prompt)
```

This transitions the FSM to `implementing`, then spawns a single coder with the minimal prompt. No wave parsing, no orchestrator creation.

**Step 2: Run existing tests to verify no regressions**

Run: `go test ./app/ -v -count=1`
Expected: all existing tests PASS

**Step 3: Commit**

```bash
git add app/app_actions.go
git commit -m "feat: handle solo stage in triggerPlanStage"
```

---

### Task 6: Gate shouldPromptPushAfterCoderExit for solo agents

**Files:**
- Modify: `app/app_state.go:961-977` (shouldPromptPushAfterCoderExit function)
- Modify: `app/app_plan_completion_test.go` (add test)

**Step 1: Write the failing test**

File: `app/app_plan_completion_test.go`

```go
func TestShouldPromptPushAfterCoderExit_NoPromptForSoloAgent(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeCoder, SoloAgent: true}

	assert.False(t, shouldPromptPushAfterCoderExit(entry, inst, false),
		"solo agents must not trigger automatic push prompt")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestShouldPromptPushAfterCoderExit_NoPromptForSoloAgent -v`
Expected: FAIL — returns true (no solo gate yet)

**Step 3: Add the solo gate**

In `app/app_state.go`, inside `shouldPromptPushAfterCoderExit`, add after the `AgentType` check:

```go
	if inst.SoloAgent {
		return false
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestShouldPromptPushAfterCoderExit -v`
Expected: all three tests PASS (existing + new)

**Step 5: Commit**

```bash
git add app/app_state.go app/app_plan_completion_test.go
git commit -m "feat: gate off push prompt for solo agents"
```

---

### Task 7: Add "start solo agent" to plan context menu

**Files:**
- Modify: `app/app_actions.go:514-562` (openPlanContextMenu function)
- Modify: `app/app_actions.go:179-198` (context menu action dispatch)

**Step 1: Add the menu item**

In `app/app_actions.go`, inside `openPlanContextMenu`, add the `"start solo agent"` option to each status case where it makes sense:

For `StatusReady` and `StatusPlanning`:
```go
	case planstate.StatusReady, planstate.StatusPlanning:
		items = append(items,
			overlay.ContextMenuItem{Label: "start plan", Action: "start_plan"},
			overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
			overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
			overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
		)
```

For `StatusImplementing`:
```go
	case planstate.StatusImplementing:
		items = append(items,
			overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
			overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
			overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
		)
```

**Step 2: Add the action dispatch**

In `app/app_actions.go`, in the context menu action handler (around line 186, near `"start_implement"`), add:

```go
	case "start_solo":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "solo")
```

**Step 3: Run all tests**

Run: `go test ./app/ -v -count=1`
Expected: all tests PASS

**Step 4: Commit**

```bash
git add app/app_actions.go
git commit -m "feat: add 'start solo agent' context menu action"
```

---

### Task 8: Full integration test — solo agent lifecycle

**Files:**
- Create: `app/app_solo_agent_test.go`

**Step 1: Write integration test**

```go
package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoloAgent_NoAutomaticPushPromptOnExit verifies that when a solo agent's
// tmux session exits, the automatic push-then-review flow does NOT trigger.
func TestSoloAgent_NoAutomaticPushPromptOnExit(t *testing.T) {
	const planFile = "2026-02-25-solo-test.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "solo test", "plan/solo-test", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     "solo-test-solo",
		Path:      t.TempDir(),
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)
	inst.SoloAgent = true

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(inst)

	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         list,
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager: overlay.NewToastManager(&sp),
		planState:    ps,
		planStateDir: plansDir,
		fsm:          planfsm.New(plansDir),
		waveOrchestrators: make(map[string]*WaveOrchestrator),
	}

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: inst.Title, TmuxAlive: false},
		},
		PlanState: ps,
	}

	model, _ := h.Update(msg)
	updated, ok := model.(*home)
	require.True(t, ok)

	assert.NotEqual(t, stateConfirm, updated.state,
		"solo agent exit must NOT trigger confirmation overlay")
	assert.Nil(t, updated.confirmationOverlay,
		"solo agent exit must NOT set confirmation overlay")
}
```

**Step 2: Run the test**

Run: `go test ./app/ -run TestSoloAgent -v`
Expected: PASS

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: all tests PASS

**Step 4: Commit**

```bash
git add app/app_solo_agent_test.go
git commit -m "test: integration test for solo agent lifecycle"
```
