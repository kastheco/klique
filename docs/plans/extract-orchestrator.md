# Extract Orchestration Engine Implementation Plan

**Goal:** Extract the wave orchestration state machine and prompt builders from `app/` into a standalone `orchestration/` package, and consolidate the remaining TUI wiring from 4 scattered files into a single `app/wave_integration.go` — making orchestration logic independently testable and the TUI integration navigable.

**Architecture:** Create top-level `orchestration/` package containing the pure-logic engine (`WaveOrchestrator`, `WaveState`, prompt builders) with zero bubbletea dependencies. All TUI-coupled wiring (spawning instances, confirmation dialogs, signal processing, message types) is consolidated from `app.go`, `app_state.go`, `app_actions.go`, and `app_input.go` into one `app/wave_integration.go`. Type aliases in the consolidated file maintain backwards compatibility so existing test code compiles unchanged.

**Tech Stack:** Go 1.24+, `config/taskparser` (dependency of orchestration engine)

**Size:** Small (estimated ~2 hours, 2 tasks, 2 waves)

---

## Wave 1: Extract Core Engine

### Task 1: Create orchestration package with engine and prompt builders

**Files:**
- Create: `orchestration/engine.go`
- Create: `orchestration/prompt.go`
- Create: `orchestration/engine_test.go`
- Create: `orchestration/prompt_test.go`

**Step 1: Write failing tests**

Create `orchestration/engine_test.go` — adapt all orchestrator tests from `app/wave_orchestrator_test.go` into `package orchestration` (do NOT copy `TestBuildClickUpComment` — that tests ClickUp formatting and stays in `app/`). The adapted tests are:

- `TestNewWaveOrchestrator` — verify State, TotalWaves, TotalTasks after construction.
- `TestWaveOrchestrator_StartWave` — start wave 1, check Running state and task list.
- `TestWaveOrchestrator_TaskCompleted` — mark tasks complete, verify wave completion.
- `TestWaveOrchestrator_TaskFailed` — mark task failed, verify counts.
- `TestWaveOrchestrator_MultiWaveProgression` — two-wave lifecycle.
- `TestWaveOrchestrator_AllComplete` — single-wave plan completes to AllComplete.
- `TestWaveOrchestrator_ResetConfirmAllowsReprompt` — NeedsConfirm one-shot latch + reset.
- `TestIsTaskRunning` — verify running/complete/unknown task queries.
- `TestWaveOrchestrator_TaskStatusQueries` — IsTaskComplete, IsTaskFailed, IsTaskRunning.
- `TestWaveOrchestrator_RetryFailedTasksRestoresRunning` — retry transitions and state.

Add new tests for the three new methods:

```go
func TestRestoreToWave(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1}, {Number: 2}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 3}}},
		},
	}
	orch := NewWaveOrchestrator("plan.md", plan)
	orch.RestoreToWave(2, []int{3})
	assert.Equal(t, WaveStateAllComplete, orch.State())
	assert.Equal(t, 2, orch.CurrentWaveNumber())
}

func TestRestoreToWave_PartialCompletion(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2}, {Number: 3}}},
		},
	}
	orch := NewWaveOrchestrator("plan.md", plan)
	orch.RestoreToWave(2, []int{2}) // task 3 still running
	assert.Equal(t, WaveStateRunning, orch.State())
	assert.True(t, orch.IsTaskComplete(2))
	assert.True(t, orch.IsTaskRunning(3))
}

func TestShouldPostWaveCompleteComment(t *testing.T) {
	single := &taskparser.Plan{Waves: []taskparser.Wave{
		{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
	}}
	multi := &taskparser.Plan{Waves: []taskparser.Wave{
		{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
		{Number: 2, Tasks: []taskparser.Task{{Number: 2}}},
	}}
	assert.False(t, NewWaveOrchestrator("s.md", single).ShouldPostWaveCompleteComment())
	assert.True(t, NewWaveOrchestrator("m.md", multi).ShouldPostWaveCompleteComment())

	// nil receiver safety
	var nilOrch *WaveOrchestrator
	assert.False(t, nilOrch.ShouldPostWaveCompleteComment())
}

func TestBuildTaskPrompt_Method(t *testing.T) {
	plan := &taskparser.Plan{
		Goal: "Test goal",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}
	orch := NewWaveOrchestrator("plan.md", plan)
	orch.StartNextWave()
	prompt := orch.BuildTaskPrompt(plan.Waves[0].Tasks[0], 2)
	assert.Contains(t, prompt, "Task 1")
	assert.Contains(t, prompt, "Test goal")
	assert.Contains(t, prompt, "Wave 1 of 1")
	assert.Contains(t, prompt, "parallel") // peerCount > 1
}
```

Create `orchestration/prompt_test.go` — adapt the two tests from `app/wave_prompt_test.go` (`TestBuildTaskPrompt`, `TestBuildTaskPrompt_SingleTask`) into `package orchestration`. Add:

```go
func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := BuildWaveAnnotationPrompt("my-feature.md")
	assert.Contains(t, prompt, "## Wave")
	assert.Contains(t, prompt, "my-feature.md")
	assert.Contains(t, prompt, "planner-finished-my-feature.md")
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./orchestration/... -v
```

Expected: FAIL — package `orchestration` doesn't exist yet.

**Step 3: Write implementation**

Create `orchestration/engine.go`:
- Export `WaveState` type with 4 constants: `WaveStateIdle`, `WaveStateRunning`, `WaveStateWaveComplete`, `WaveStateAllComplete`.
- Keep `taskStatus` type and constants unexported (only used internally by the engine).
- `WaveOrchestrator` struct — same fields as current `app/wave_orchestrator.go`, all unexported.
- `NewWaveOrchestrator(planFile string, plan *taskparser.Plan) *WaveOrchestrator` constructor.
- All existing methods verbatim from `app/wave_orchestrator.go`: `State`, `TaskFile`, `TotalWaves`, `TotalTasks`, `CurrentWaveNumber`, `CurrentWaveTasks`, `StartNextWave`, `MarkTaskComplete`, `MarkTaskFailed`, `NeedsConfirm`, `ResetConfirm`, `RetryFailedTasks`, `IsCurrentWaveComplete`, `CompletedTaskCount`, `FailedTaskCount`, `IsTaskRunning`, `IsTaskComplete`, `IsTaskFailed`, `HeaderContext`, `checkWaveComplete`, `countCurrentWaveByStatus`.
- New method `ShouldPostWaveCompleteComment() bool` — nil-receiver safe, returns `o != nil && o.TotalWaves() > 1`. Extracted from `app/clickup_progress.go:shouldPostWaveCompleteComment`.
- New method `RestoreToWave(targetWave int, completedTasks []int)` — fast-forwards wave-by-wave to `targetWave`, auto-completing all tasks in earlier waves via `StartNextWave` + `MarkTaskComplete` loops, then marks the specified task numbers as complete in the target wave. Extracted from the manual fast-forward loop in `app/app_state.go:rebuildOrphanedOrchestrators` (lines 1637–1657).
- New method `BuildTaskPrompt(task taskparser.Task, peerCount int) string` — convenience wrapper: `return BuildTaskPrompt(o.plan, task, o.CurrentWaveNumber(), o.TotalWaves(), peerCount)`.

Create `orchestration/prompt.go`:
- `BuildTaskPrompt(plan *taskparser.Plan, task taskparser.Task, waveNumber, totalWaves, peerCount int) string` — moved verbatim from `app/wave_prompt.go:buildTaskPrompt`.
- `BuildWaveAnnotationPrompt(planFile string) string` — moved from `app/app_state.go:buildWaveAnnotationPrompt` (lines 1341–1353).

**Step 4: Run tests to verify they pass**

```bash
go test ./orchestration/... -v
```

Expected: PASS — all engine and prompt tests green.

Also verify the existing app tests still pass (old code untouched):

```bash
go test ./app/... -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add orchestration/
git commit -m "feat: extract orchestration engine into standalone package"
```

---

## Wave 2: Rewire App Layer

> **depends on wave 1:** Task 2 imports the `orchestration/` package created in Wave 1 to replace inline types and delegate to exported methods.

### Task 2: Consolidate TUI wiring and rewire app imports

**Files:**
- Create: `app/wave_integration.go`
- Create: `app/wave_integration_test.go`
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Modify: `app/app_input.go`
- Modify: `app/clickup_progress.go`
- Modify: `app/clickup_progress_test.go`
- Delete: `app/wave_orchestrator.go`
- Delete: `app/wave_prompt.go`
- Delete: `app/wave_orchestrator_test.go`
- Delete: `app/wave_prompt_test.go`
- Test: `app/*_test.go` (all must pass unchanged via type aliases)

**Step 1: Write failing test**

Create `app/wave_integration_test.go` with a compile-time type alias verification:

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/orchestration"
	"github.com/stretchr/testify/assert"
)

func TestWaveIntegration_TypeAliasesResolve(t *testing.T) {
	// Verify type aliases resolve correctly to orchestration package types.
	assert.Equal(t, orchestration.WaveStateIdle, WaveStateIdle)
	assert.Equal(t, orchestration.WaveStateRunning, WaveStateRunning)
	assert.Equal(t, orchestration.WaveStateWaveComplete, WaveStateWaveComplete)
	assert.Equal(t, orchestration.WaveStateAllComplete, WaveStateAllComplete)
}
```

**Step 2: Verify baseline**

```bash
go test ./app/... -count=1
```

Expected: PASS — everything works before refactoring.

**Step 3: Implement the consolidation**

**3a. Create `app/wave_integration.go`** — the single file consolidating all orchestration TUI wiring:

Type aliases and constructor alias (preserve backwards compat for all test files):
```go
package app

import "github.com/kastheco/kasmos/orchestration"

// Type aliases — all app/ code and tests use these unqualified names.
type WaveOrchestrator = orchestration.WaveOrchestrator
type WaveState = orchestration.WaveState

// Constructor alias.
var NewWaveOrchestrator = orchestration.NewWaveOrchestrator

// Constant aliases.
const (
	WaveStateIdle         = orchestration.WaveStateIdle
	WaveStateRunning      = orchestration.WaveStateRunning
	WaveStateWaveComplete = orchestration.WaveStateWaveComplete
	WaveStateAllComplete  = orchestration.WaveStateAllComplete
)
```

Move these into `wave_integration.go`:
- **From `app/app.go`:** wave message types (`waveAdvanceMsg`, `waveRetryMsg`, `waveAbortMsg`, `waveAllCompleteMsg`) — cut from ~lines 1862–1883.
- **From `app/app_state.go`:** methods `spawnWaveTasks`, `startNextWave`, `retryFailedWaveTasks`, `rebuildOrphanedOrchestrators` — cut from ~lines 1567–1765.
- **From `app/app_input.go`:** methods `waveStandardConfirmAction`, `waveFailedConfirmAction` — cut from ~lines 1567–1602.

**3b. Update `spawnWaveTasks`** (now in `wave_integration.go`):
- Replace `buildTaskPrompt(orch.plan, task, orch.CurrentWaveNumber(), orch.TotalWaves(), len(tasks))` with `orch.BuildTaskPrompt(task, len(tasks))`.

**3c. Update `rebuildOrphanedOrchestrators`** (now in `wave_integration.go`):
- Replace the manual fast-forward loop (lines 1625–1657 of app_state.go) with:
```go
orch := orchestration.NewWaveOrchestrator(planFile, plan)
var completed []int
for _, t := range tasks {
	if t.waveNumber == targetWave && t.paused {
		completed = append(completed, t.taskNumber)
	}
}
orch.RestoreToWave(targetWave, completed)
m.waveOrchestrators[planFile] = orch
```

**3d. Fix direct field access in `app/app_state.go`** (~line 94):
- Replace `switch orch.taskStates[task.Number]` with method-based switch:
```go
switch {
case orch.IsTaskComplete(task.Number):
	data.TaskGlyphs[i] = ui.TaskGlyphComplete
case orch.IsTaskFailed(task.Number):
	data.TaskGlyphs[i] = ui.TaskGlyphFailed
case orch.IsTaskRunning(task.Number):
	data.TaskGlyphs[i] = ui.TaskGlyphRunning
default:
	data.TaskGlyphs[i] = ui.TaskGlyphPending
}
```
This replaces the direct access to the unexported `taskStates` map and `taskComplete`/`taskFailed`/`taskRunning` constants.

**3e. Update `app/app_actions.go`:**
- Replace `buildWaveAnnotationPrompt(planFile)` with `orchestration.BuildWaveAnnotationPrompt(planFile)`.

**3f. Update `app/clickup_progress.go`:**
- Remove `shouldPostWaveCompleteComment` function (now `orch.ShouldPostWaveCompleteComment()` method).
- Keep `resolveClickUpTaskID` and `postClickUpProgress` (standalone function) unchanged.

**3g. Update callers of `shouldPostWaveCompleteComment` in `app/app.go`:**
- `shouldPostWaveCompleteComment(orch)` → `orch.ShouldPostWaveCompleteComment()` (two call sites, ~lines 1326 and 1367).

**3h. Update `app/clickup_progress_test.go`:**
- `TestSingleWavePlanSkipsWaveComment`: replace `shouldPostWaveCompleteComment(singleOrch)` → `singleOrch.ShouldPostWaveCompleteComment()` and same for multi.
- `TestShouldPostWaveCompleteCommentNilOrch`: replace `shouldPostWaveCompleteComment(nil)` → `var nilOrch *WaveOrchestrator; nilOrch.ShouldPostWaveCompleteComment()`.

**3i. Move `TestBuildClickUpComment`** from `app/wave_orchestrator_test.go` to `app/clickup_progress_test.go` (it tests `buildClickUpProgressComment` which is a ClickUp formatter, not orchestration logic).

**3j. Delete old files:**
- `app/wave_orchestrator.go` — all code now in `orchestration/engine.go` + aliases in `wave_integration.go`.
- `app/wave_prompt.go` — all code now in `orchestration/prompt.go`.
- `app/wave_orchestrator_test.go` — orchestrator tests moved to `orchestration/engine_test.go`, ClickUp test moved to `clickup_progress_test.go`.
- `app/wave_prompt_test.go` — tests moved to `orchestration/prompt_test.go`.

**3k. Remove `buildWaveAnnotationPrompt` from `app/app_state.go`** (lines 1337–1353) — moved to `orchestration/prompt.go`.

**Step 4: Run all tests to verify nothing broke**

```bash
go build ./...
go test ./... -count=1
```

Expected: all compile, all PASS. The 1197-line `app_wave_orchestration_flow_test.go` and all other test files compile unchanged thanks to type aliases.

**Step 5: Commit**

```bash
git add orchestration/ app/
git commit -m "refactor: consolidate wave TUI wiring into wave_integration.go, rewire to orchestration package"
```
