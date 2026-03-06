# Headless Coder Execution Exploration Implementation Plan

**Goal:** Explore and validate a coder-only headless execution path so kasmos can delegate implementation without depending on tmux prompt detection, and leave behind a prototype plus evidence that makes the execution-model decision concrete.

**Architecture:** First, replace wave-task completion's implicit tmux prompt heuristic with explicit per-task completion signals so orchestration no longer assumes an interactive terminal. Then introduce a session execution abstraction in `session/` that keeps tmux as the default backend while adding an experimental direct-process headless runner for coder agents only. Finally, wire execution-mode selection through config/app spawn paths and capture the trade-offs in a repo doc so future delegation work can choose between interactive and headless modes from real implementation data.

**Tech Stack:** Go 1.24, Bubble Tea app orchestration, `session/tmux`, new headless session backend, task FSM signal scanning, shared git worktrees, task-store-backed plans, repo docs.

**Size:** Medium (estimated ~4-6 hours, 3 tasks, 3 waves)

---

## Wave 1: Explicit Task Completion Signals

Deliver a wave-safe completion contract that does not rely on a tmux prompt returning after work is done.

### Task 1: Replace prompt-detected wave completion with explicit task-finished signals

**Files:**
- Create: `config/taskfsm/task_signal.go`
- Create: `config/taskfsm/task_signal_test.go`
- Modify: `app/app.go`
- Modify: `app/app_task_completion_test.go`
- Modify: `app/app_wave_orchestration_flow_test.go`
- Modify: `orchestration/prompt.go`
- Modify: `.agents/skills/kasmos-coder/SKILL.md`
- Test: `config/taskfsm/task_signal_test.go`
- Test: `app/app_task_completion_test.go`
- Test: `app/app_wave_orchestration_flow_test.go`

**Step 1: write the failing test**

```go
func TestParseTaskSignal(t *testing.T) {
    sig, ok := ParseTaskSignal("implement-task-finished-w1-t2-headless-coder-execution-exploration.md")
    require.True(t, ok)
    assert.Equal(t, 1, sig.WaveNumber)
    assert.Equal(t, 2, sig.TaskNumber)
    assert.Equal(t, "headless-coder-execution-exploration.md", sig.TaskFile)
}

func TestMetadataTick_TaskFinishedSignalMarksWaveTaskComplete(t *testing.T) {
    // Build an active orchestrator, inject a task-finished signal, and assert
    // the task becomes complete without relying on PromptDetected.
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskfsm ./app -run 'TestParseTaskSignal|TestMetadataTick_TaskFinishedSignalMarksWaveTaskComplete' -v
```

expected: FAIL - `ParseTaskSignal` does not exist and wave completion still depends on `PromptDetected` / `HasWorked`.

**Step 3: write minimal implementation**

Add a task-scoped signal format such as `implement-task-finished-w<wave>-t<task>-<plan>.md`, scan and consume it beside existing planner/review/wave signals, and update the metadata tick so active orchestrators mark only the matching task complete when that signal arrives. Keep `implement-finished-<plan>.md` reserved for whole-plan completion, update `BuildTaskPrompt` so wave coders are explicitly told which task-finished signal to write after their commit, and change `.agents/skills/kasmos-coder/SKILL.md` managed-mode guidance to use the new signal instead of tmux prompt detection.

**Step 4: run test to verify it passes**

```bash
go test ./config/taskfsm ./app -run 'TestParseTaskSignal|TestMetadataTick_TaskFinishedSignalMarksWaveTaskComplete' -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/taskfsm/task_signal.go config/taskfsm/task_signal_test.go app/app.go app/app_task_completion_test.go app/app_wave_orchestration_flow_test.go orchestration/prompt.go .agents/skills/kasmos-coder/SKILL.md
git commit -m "feat: add explicit task-finished signals for wave coders"
```

## Wave 2: Execution Backend Abstraction

> **depends on wave 1:** headless coders need a non-interactive completion path before a direct-process backend can participate in wave orchestration.

### Task 2: Introduce pluggable execution backends and a headless session prototype

**Files:**
- Create: `session/execution.go`
- Create: `session/headless/session.go`
- Create: `session/headless/session_test.go`
- Modify: `session/instance.go`
- Modify: `session/instance_lifecycle.go`
- Modify: `session/instance_session.go`
- Modify: `session/storage.go`
- Modify: `session/instance_lifecycle_test.go`
- Modify: `session/instance_taskfile_test.go`
- Test: `session/headless/session_test.go`
- Test: `session/instance_lifecycle_test.go`
- Test: `session/instance_taskfile_test.go`

**Step 1: write the failing test**

```go
func TestInstance_DefaultExecutionModeIsTmux(t *testing.T) {
    inst, err := NewInstance(InstanceOptions{Title: "coder", Path: ".", Program: "opencode"})
    require.NoError(t, err)
    assert.Equal(t, ExecutionModeTmux, inst.ExecutionMode)
}

func TestHeadlessSession_StartRunsProgramAndCapturesOutput(t *testing.T) {
    // Start a short-lived command, wait for exit, and assert preview/log capture works.
}

func TestInstance_AttachReturnsErrorForHeadlessExecution(t *testing.T) {
    // Headless instances must fail cleanly for interactive-only actions.
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session ./session/headless -run 'TestInstance_DefaultExecutionModeIsTmux|TestHeadlessSession_StartRunsProgramAndCapturesOutput|TestInstance_AttachReturnsErrorForHeadlessExecution' -v
```

expected: FAIL - there is no execution-mode abstraction, no headless backend, and `Instance` only understands tmux sessions.

**Step 3: write minimal implementation**

Introduce `ExecutionMode` constants (`tmux`, `headless`) plus a shared execution interface in `session/`, keep tmux as the default backend, and adapt `Instance` to hold the generic backend instead of a hard-coded `*tmux.TmuxSession`. Implement `session/headless/session.go` as a direct-process runner built on `exec.CommandContext` that writes stdout/stderr to a per-instance log under `.kasmos/logs`, exposes latest output for preview/history, tracks process exit, and returns clear `unsupported` errors for interactive-only operations like attach/send-keys. Persist `ExecutionMode` in `session/storage.go` so reloads preserve how an instance was launched.

**Step 4: run test to verify it passes**

```bash
go test ./session ./session/headless -run 'TestInstance_DefaultExecutionModeIsTmux|TestHeadlessSession_StartRunsProgramAndCapturesOutput|TestInstance_AttachReturnsErrorForHeadlessExecution' -v
```

expected: PASS

**Step 5: commit**

```bash
git add session/execution.go session/headless/session.go session/headless/session_test.go session/instance.go session/instance_lifecycle.go session/instance_session.go session/storage.go session/instance_lifecycle_test.go session/instance_taskfile_test.go
git commit -m "refactor: add pluggable execution backends for instances"
```

## Wave 3: Coder-Only Headless Prototype

> **depends on wave 2:** the app cannot route coder launches into a headless backend until the execution abstraction and prototype runner exist.

### Task 3: Wire an experimental headless coder mode through config, spawn paths, and documentation

**Files:**
- Create: `docs/headless-coder-execution-models.md`
- Modify: `config/profile.go`
- Modify: `config/toml.go`
- Modify: `config/toml_test.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Modify: `app/app_task_actions_test.go`
- Modify: `app/app_task_completion_test.go`
- Test: `config/toml_test.go`
- Test: `app/app_task_actions_test.go`
- Test: `app/app_task_completion_test.go`

**Step 1: write the failing test**

```go
func TestResolveProfile_ExecutionMode(t *testing.T) {
    // TOML config with execution_mode = "headless" should round-trip into AgentProfile.
}

func TestSpawnWaveTasks_HeadlessCoderUsesHeadlessExecution(t *testing.T) {
    // A coder profile configured for headless mode should create wave instances
    // with ExecutionModeHeadless instead of the tmux default.
}

func TestShouldPromptPushAfterCoderExit_HeadlessCoderExited(t *testing.T) {
    // Non-wave headless coders should still trigger the push/review flow on exit.
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config ./app -run 'TestResolveProfile_ExecutionMode|TestSpawnWaveTasks_HeadlessCoderUsesHeadlessExecution|TestShouldPromptPushAfterCoderExit_HeadlessCoderExited' -v
```

expected: FAIL - config cannot express execution mode, coder spawn paths always use tmux, and app completion logic only understands interactive sessions.

**Step 3: write minimal implementation**

Extend `AgentProfile` / TOML parsing with `execution_mode`, default it to `tmux`, and add an `executionModeForAgent` helper that only honors `headless` for coder agents in this exploration. Update `spawnTaskAgent`, `spawnCoderWithFeedback`, and `spawnWaveTasks` to instantiate coder instances with the selected execution mode and route startup through the matching backend. Guard attach/focus actions in `app/app_actions.go` so headless instances show a lowercase informational toast instead of attempting tmux attach, and treat headless coder completion as process exit plus explicit task or plan sentinels. Capture the results in `docs/headless-coder-execution-models.md` with a comparison of tmux vs headless behavior, operational limits (log-based preview, no interactive attach), and a recommendation for the next delegation milestone.

**Step 4: run test to verify it passes**

```bash
go test ./config ./app -run 'TestResolveProfile_ExecutionMode|TestSpawnWaveTasks_HeadlessCoderUsesHeadlessExecution|TestShouldPromptPushAfterCoderExit_HeadlessCoderExited' -v
```

expected: PASS

**Step 5: commit**

```bash
git add docs/headless-coder-execution-models.md config/profile.go config/toml.go config/toml_test.go app/app_state.go app/app_actions.go app/app_task_actions_test.go app/app_task_completion_test.go
git commit -m "feat: add experimental headless coder execution mode"
```
