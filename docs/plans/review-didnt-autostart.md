# Review Didn't Auto-Start After All Waves Complete

**Goal:** Fix the bug where the review prompt is silently dropped when all waves complete while the user is in an overlay, leaving the plan stuck at `implementing` forever.

**Architecture:** The wave-all-complete handler in the metadata tick (`app.go` ~line 1049) unconditionally deletes the orchestrator from `waveOrchestrators` before checking `isUserInOverlay()`. When the user is in an overlay, the confirmation dialog is skipped, but the orchestrator is already gone — so the code path never re-enters on subsequent ticks. The fix adds a deferred queue (`pendingAllComplete`) that stores the plan file when the overlay blocks the prompt, and drains it on the next tick when the overlay clears.

**Tech Stack:** Go, bubbletea, testify

**Size:** Small (estimated ~45 min, 2 tasks, 1 wave)

---

## Wave 1: Fix overlay-blocked all-complete

### Task 1: Add deferred wave-all-complete recovery mechanism

**Files:**
- Modify: `app/app.go` (add `pendingAllComplete` field, defer when overlay blocks, drain on tick)

**Step 1: write the failing test**

The test for this behavior is written in Task 2 (same wave). This task modifies the production code to support the deferred queue. The test in Task 2 validates the full flow.

Skip TDD steps 1-2 for this task — the implementation is tightly coupled with the test in Task 2 and both must exist for either to work.

**Step 3: write minimal implementation**

Add a `pendingAllComplete []string` field to the `home` struct (next to `waveOrchestrators`). This stores plan files whose all-complete prompt was blocked by an active overlay.

In the metadata tick handler, change the `WaveStateAllComplete` block:

**Before (buggy):**
```go
delete(m.waveOrchestrators, planFile)

if !m.isUserInOverlay() {
    // show confirm dialog
}
```

**After (fixed):**
```go
delete(m.waveOrchestrators, planFile)

if !m.isUserInOverlay() {
    // show confirm dialog (unchanged)
} else {
    // Overlay is active — defer the prompt so it fires on the next
    // tick when the overlay clears. Without this, the orchestrator
    // deletion above means we never re-enter this code path.
    m.pendingAllComplete = append(m.pendingAllComplete, planFile)
}
```

Then, at the top of the metadata tick handler (before the wave orchestrator loop), add a drain loop:

```go
// Drain deferred all-complete prompts that were blocked by an overlay.
if !m.isUserInOverlay() && len(m.pendingAllComplete) > 0 {
    planFile := m.pendingAllComplete[0]
    m.pendingAllComplete = m.pendingAllComplete[1:]
    planName := planstate.DisplayName(planFile)
    if cmd := m.focusPlanInstanceForOverlay(planFile); cmd != nil {
        asyncCmds = append(asyncCmds, cmd)
    }
    message := fmt.Sprintf("all waves complete for '%s'. push branch and start review?", planName)
    m.confirmAction(message, func() tea.Msg {
        return waveAllCompleteMsg{planFile: planFile}
    })
}
```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestWaveMonitor_AllComplete -v
```

expected: PASS (existing tests still pass, new test from Task 2 validates the fix)

**Step 5: commit**

```bash
git add app/app.go
git commit -m "fix: defer wave-all-complete prompt when overlay blocks it"
```

### Task 2: Add test for overlay-blocked all-complete recovery

**Files:**
- Modify: `app/app_wave_orchestration_flow_test.go`

**Step 1: write the failing test**

```go
// TestWaveMonitor_AllComplete_DeferredWhenOverlayActive verifies that when all
// waves complete while the user is in an overlay (e.g. confirmation dialog),
// the review prompt is deferred and shown on the next tick when the overlay clears.
func TestWaveMonitor_AllComplete_DeferredWhenOverlayActive(t *testing.T) {
    const planFile = "2026-02-28-deferred-complete.md"

    plan := &planparser.Plan{
        Waves: []planparser.Wave{
            {Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Only task", Body: "do it"}}},
        },
    }
    orch := NewWaveOrchestrator(planFile, plan)
    orch.StartNextWave()

    dir := t.TempDir()
    plansDir := filepath.Join(dir, "docs", "plans")
    require.NoError(t, os.MkdirAll(plansDir, 0o755))
    ps, err := planstate.Load(plansDir)
    require.NoError(t, err)
    require.NoError(t, ps.Register(planFile, "deferred test", "plan/deferred", time.Now()))
    seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

    inst, err := session.NewInstance(session.InstanceOptions{
        Title:      "deferred-complete-W1-T1",
        Path:       t.TempDir(),
        Program:    "claude",
        PlanFile:   planFile,
        TaskNumber: 1,
        WaveNumber: 1,
    })
    require.NoError(t, err)
    inst.PromptDetected = true

    h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
    h.fsm = planfsm.New(plansDir)
    _ = h.nav.AddInstance(inst)

    // Simulate user being in an overlay (e.g. another confirmation dialog)
    h.state = stateConfirm

    msg := metadataResultMsg{
        Results:   []instanceMetadata{{Title: "deferred-complete-W1-T1", TmuxAlive: true}},
        PlanState: ps,
    }
    model, _ := h.Update(msg)
    updated := model.(*home)

    // Orchestrator must be deleted (tasks are paused)
    assert.Empty(t, updated.waveOrchestrators,
        "orchestrator must be deleted even when overlay is active")

    // But the confirm dialog must NOT have been shown (overlay was blocking)
    // Instead, the plan file must be in pendingAllComplete
    assert.Contains(t, updated.pendingAllComplete, planFile,
        "plan must be deferred to pendingAllComplete when overlay blocks")

    // Now simulate the overlay clearing and another metadata tick arriving
    updated.state = stateDefault
    msg2 := metadataResultMsg{
        Results:   []instanceMetadata{{Title: "deferred-complete-W1-T1", TmuxAlive: true}},
        PlanState: ps,
    }
    model2, _ := updated.Update(msg2)
    updated2 := model2.(*home)

    // Now the confirm dialog must appear
    assert.Equal(t, stateConfirm, updated2.state,
        "deferred all-complete must show confirm dialog on next tick")
    assert.Empty(t, updated2.pendingAllComplete,
        "pendingAllComplete must be drained after showing dialog")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestWaveMonitor_AllComplete_DeferredWhenOverlayActive -v
```

expected: FAIL — `pendingAllComplete` field does not exist yet (or if Task 1 runs first, the test should pass)

**Step 3: write minimal implementation**

The test itself is the implementation for this task. The production code change is in Task 1.

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestWaveMonitor_AllComplete -v
```

expected: PASS — both existing and new tests pass

**Step 5: commit**

```bash
git add app/app_wave_orchestration_flow_test.go
git commit -m "test: verify deferred all-complete prompt when overlay blocks"
```
