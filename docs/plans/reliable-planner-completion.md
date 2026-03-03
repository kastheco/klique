# Reliable Planner Completion Implementation Plan

**Goal:** Make kasmos respond to the planner-finished sentinel by killing the planner agent and showing a confirmation dialog, instead of relying on the tmux-death fallback which fires unreliably and causes spurious transitions on manual planner kills.

**Architecture:** Add a `PlannerFinished` case to the signal processing switch in `app.go` that focuses the planner instance, shows "plan ready — start implementation?" dialog, and kills the planner on user response. Remove the tmux-death fallback block entirely — the sentinel is the authoritative completion signal. Update confirm/cancel/esc handlers to kill the planner tmux session (currently they only remove from lists, assuming the pane was already dead). Add `pendingPlannerPlanFile` field so cancel/esc handlers can mark `plannerPrompted` without looking up a deleted instance.

**Tech Stack:** Go (bubbletea), testify

**Size:** Small (estimated ~1.5h, 2 tasks, 2 waves)

---

## Wave 1: Signal-driven planner completion handler

### Task 1: Add PlannerFinished signal handler and update dialog handlers

**Files:**
- Modify: `app/app.go` — signal processing switch, `plannerCompleteMsg` handler, add `pendingPlannerPlanFile` field
- Modify: `app/app_input.go` — cancel and esc handlers for planner dialog
- Create: `app/app_planner_signal_test.go` — new tests for signal-driven completion
- Test: `app/app_planner_signal_test.go`

**Step 1: write the failing tests**

Create `app/app_planner_signal_test.go` with tests:

1. `TestPlannerFinishedSignal_ShowsConfirmDialog` — when a `PlannerFinished` signal is processed, the app enters `stateConfirm` with a confirmation overlay and `pendingPlannerPlanFile` is set.

2. `TestPlannerFinishedSignal_ConfirmKillsPlannerAndTriggersImplement` — after confirm (plannerCompleteMsg), the planner instance is removed from nav+allInstances, `plannerPrompted` is set, and `triggerPlanStage("implement")` is called.

3. `TestPlannerFinishedSignal_CancelKillsPlannerAndLeavesReady` — after cancel (no), the planner instance is removed, `plannerPrompted` is set, plan stays at `StatusReady`.

4. `TestPlannerFinishedSignal_SkipsWhenAlreadyPrompted` — if `plannerPrompted[planFile]` is already true, no dialog is shown.

5. `TestPlannerFinishedSignal_SkipsWhenConfirmActive` — if `state == stateConfirm`, no dialog is shown (avoids clobbering an active overlay).

**Step 2: run tests to verify they fail**

```bash
go test ./app/... -run TestPlannerFinishedSignal -v
```

expected: FAIL — signal handler not implemented yet

**Step 3: implement the changes**

**3a. Add `pendingPlannerPlanFile` field** to the `home` struct (app.go, near `pendingPlannerInstanceTitle`):

```go
pendingPlannerPlanFile string
```

Initialize it as empty string (zero value is fine).

**3b. Add `PlannerFinished` case** to the signal processing switch in the `metadataResultMsg` handler (app.go, after the `ReviewChangesRequested` case around line 668):

```go
case planfsm.PlannerFinished:
    capturedPlanFile := sig.PlanFile
    if m.plannerPrompted[capturedPlanFile] || m.state == stateConfirm {
        break
    }
    // Focus the planner instance so the user sees its output behind the overlay.
    for _, inst := range m.nav.GetInstances() {
        if inst.PlanFile == sig.PlanFile && inst.AgentType == session.AgentTypePlanner {
            if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
                signalCmds = append(signalCmds, cmd)
            }
            m.pendingPlannerInstanceTitle = inst.Title
            break
        }
    }
    m.pendingPlannerPlanFile = capturedPlanFile
    m.confirmAction(
        fmt.Sprintf("plan '%s' is ready. start implementation?", planstate.DisplayName(capturedPlanFile)),
        func() tea.Msg {
            return plannerCompleteMsg{planFile: capturedPlanFile}
        },
    )
```

**3c. Update `plannerCompleteMsg` handler** (app.go, around line 1279) to kill the planner via `killExistingPlanAgent` instead of just removing from `allInstances`:

```go
case plannerCompleteMsg:
    m.plannerPrompted[msg.planFile] = true
    m.killExistingPlanAgent(msg.planFile, session.AgentTypePlanner)
    _ = m.saveAllInstances()
    m.pendingPlannerInstanceTitle = ""
    m.pendingPlannerPlanFile = ""
    m.updateNavPanelStatus()
    return m.triggerPlanStage(msg.planFile, "implement")
```

**3d. Update cancel handler** (app_input.go, around line 603) to kill the planner tmux session using `pendingPlannerPlanFile` instead of looking up the instance by title:

```go
// Planner signal "no": kill planner instance, mark prompted, leave plan ready.
if m.pendingPlannerPlanFile != "" {
    m.plannerPrompted[m.pendingPlannerPlanFile] = true
    m.killExistingPlanAgent(m.pendingPlannerPlanFile, session.AgentTypePlanner)
    _ = m.saveAllInstances()
    m.updateNavPanelStatus()
    m.pendingPlannerInstanceTitle = ""
    m.pendingPlannerPlanFile = ""
}
```

**3e. Update esc handler** (app_input.go, around line 631) — esc now acts the same as cancel since the signal is consumed and can't re-trigger:

```go
// Planner signal esc: same as cancel — signal is consumed, can't re-trigger.
if m.pendingPlannerPlanFile != "" {
    m.plannerPrompted[m.pendingPlannerPlanFile] = true
    m.killExistingPlanAgent(m.pendingPlannerPlanFile, session.AgentTypePlanner)
    _ = m.saveAllInstances()
    m.updateNavPanelStatus()
    m.pendingPlannerInstanceTitle = ""
    m.pendingPlannerPlanFile = ""
}
```

**Step 4: run tests to verify they pass**

```bash
go test ./app/... -run TestPlannerFinishedSignal -v
```

expected: PASS

Also run the existing signal tests to verify no regressions:

```bash
go test ./app/... -run 'TestMetadataResultMsg_Signal|TestImplementFinished|TestReviewChanges' -v
```

**Step 5: commit**

```bash
git add app/app.go app/app_input.go app/app_planner_signal_test.go
git commit -m "feat: signal-driven planner completion with confirmation dialog"
```

---

## Wave 2: Remove tmux-death fallback and update legacy tests

> **depends on wave 1:** the signal-driven handler must be in place before removing the fallback, otherwise planner completion has no trigger mechanism.

### Task 2: Remove tmux-death planner fallback and update tests

**Files:**
- Modify: `app/app.go` — remove planner-exit tmux-death fallback block (lines 853-890)
- Modify: `app/app_wave_orchestration_flow_test.go` — remove/update tmux-death planner tests
- Modify: `app/app_plan_completion_test.go` — update `TestMetadataResultMsg_SignalDoesNotClobberFreshPlanState` if needed
- Test: `app/app_wave_orchestration_flow_test.go`, `app/app_plan_completion_test.go`

**Step 1: write tests that assert the fallback is gone**

Add to `app/app_planner_signal_test.go`:

`TestPlannerTmuxDeath_NoFallbackDialog` — when a planner pane dies but NO sentinel was written (plan is `StatusPlanning`), NO dialog is shown. The plan stays in `StatusPlanning`. This verifies the aggressive fallback is removed.

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestPlannerTmuxDeath_NoFallbackDialog -v
```

expected: FAIL — the fallback still fires

**Step 3: remove the tmux-death fallback**

Remove the entire "Planner-exit → implement-prompt" block from `app.go` (the block starting with the comment `// Planner-exit → implement-prompt:` and ending at `break // one prompt per tick`). This is approximately lines 853-890.

Then update/remove the tests that relied on the fallback:

- **Remove** `TestPlannerExit_ShowsImplementConfirm` — tested the fallback path
- **Remove** `TestPlannerExit_NoRePromptAfterAnswer` — tested `plannerPrompted` with tmux death; Wave 1's tests cover this via signals
- **Remove** `TestPlannerExit_NoPromptWhileAlive` — no longer relevant (signals, not pane death, trigger the dialog)
- **Remove** `TestPlannerExit_EscPreservesForRePrompt` — esc now acts like cancel; Wave 1's tests cover this
- **Update** `TestPlannerExitCancel_KillsInstanceAndMarksPrompted` — if it references the tmux-death flow, update to use the signal-driven flow instead
- **Update** `TestFocusInstance_PlannerExit` — if it depends on tmux-death triggering, update to use signal-driven flow
- **Verify** `TestMetadataResultMsg_SignalDoesNotClobberFreshPlanState` — should still pass since it tests PlannerFinished signal processing (not the removed fallback)

**Step 4: run all tests**

```bash
go test ./app/... -v
```

expected: PASS — all tests pass, no tmux-death planner fallback

```bash
go test ./config/planfsm/... -v
```

expected: PASS — FSM tests unaffected

**Step 5: commit**

```bash
git add app/app.go app/app_wave_orchestration_flow_test.go app/app_plan_completion_test.go app/app_planner_signal_test.go
git commit -m "fix: remove aggressive planner tmux-death fallback that caused spurious transitions"
```
