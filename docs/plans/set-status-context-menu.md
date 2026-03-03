# Set Status Context Menu Implementation Plan

**Goal:** Add a "set status" option to the plan context menu that force-overrides a plan's status without triggering FSM events or automation side effects, enabling manual correction of out-of-sync plans.

**Architecture:** Adds a `set_status` action to `openPlanContextMenu()`, a new `stateSetStatus` app state, and a picker overlay listing all 6 valid statuses. On selection, calls `planstate.ForceSetStatus()` which writes directly to disk bypassing the FSM — no events fire, no agents spawn, no automation triggers. Follows the identical pattern used by `change_topic` (context menu → picker overlay → direct state mutation → refresh).

**Tech Stack:** Go, bubbletea, lipgloss, existing `overlay.PickerOverlay`, existing `planstate.ForceSetStatus()`

**Size:** Small (estimated ~30 min, 1 task, 1 wave)

---

## Wave 1: Set Status Picker

### Task 1: Add set-status context menu action with picker overlay

**Files:**
- Modify: `app/app.go` (add `stateSetStatus` state constant and `pendingSetStatusPlan` field)
- Modify: `app/app_actions.go` (add `set_status` to context menu items and `executeContextAction` handler)
- Modify: `app/app_input.go` (add `stateSetStatus` to overlay guard and key handling)
- Test: `app/app_plan_actions_test.go` (add test for force-set-status via context action)

**Step 1: write the failing test**

```go
func TestExecuteContextAction_SetStatusForceOverridesWithoutFSM(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	planFile := "2026-02-28-test-set-status.md"
	require.NoError(t, ps.Register(planFile, "test set status", "plan/test-set-status", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		planState:      ps,
		planStateDir:   plansDir,
		fsm:            newFSMForTest(plansDir).PlanStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:   overlay.NewToastManager(&sp),
		activeRepoPath: dir,
	}

	h.updateSidebarPlans()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile))

	// Simulate: context menu selected "set_status", which sets up the picker
	_, _ = h.executeContextAction("set_status")
	assert.Equal(t, stateSetStatus, h.state, "set_status action should enter stateSetStatus")
	assert.NotNil(t, h.pickerOverlay, "picker overlay should be created for status selection")
	assert.Equal(t, planFile, h.pendingSetStatusPlan, "pending plan file should be stored")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestExecuteContextAction_SetStatusForceOverridesWithoutFSM -v
```

expected: FAIL — `stateSetStatus` undefined, `pendingSetStatusPlan` undefined

**Step 3: write minimal implementation**

In `app/app.go`:
- Add `stateSetStatus` to the state enum (after `stateChangeTopic`)
- Add `pendingSetStatusPlan string` field to the `home` struct (next to `pendingChangeTopicPlan`)

In `app/app_actions.go` — `openPlanContextMenu()`:
- Add `overlay.ContextMenuItem{Label: "set status", Action: "set_status"}` to the common items list, before "cancel plan"

In `app/app_actions.go` — `executeContextAction()`:
- Add a `case "set_status":` handler that:
  1. Gets the selected plan file
  2. Stores it in `m.pendingSetStatusPlan`
  3. Builds the status list: `[]string{"ready", "planning", "implementing", "reviewing", "done", "cancelled"}`
  4. Creates a `PickerOverlay` with title "set status"
  5. Sets `m.state = stateSetStatus`

In `app/app_input.go`:
- Add `stateSetStatus` to the overlay guard condition (the long `if m.state == ...` line)
- Add a `stateSetStatus` handling block (modeled on the `stateChangeTopic` block):
  1. Delegates key events to `m.pickerOverlay.HandleKeyPress(msg)`
  2. On close+submit: calls `m.planState.ForceSetStatus(m.pendingSetStatusPlan, picked)`
  3. Refreshes sidebar: `m.loadPlanState()`, `m.updateSidebarPlans()`, `m.updateNavPanelStatus()`
  4. Shows a toast: `m.toastManager.Success(fmt.Sprintf("status → %s", picked))`
  5. Cleans up: resets state, clears picker and pending field

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestExecuteContextAction_SetStatusForceOverridesWithoutFSM -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_actions.go app/app_input.go app/app_plan_actions_test.go
git commit -m "feat: add set-status context menu for force-overriding plan status"
```
