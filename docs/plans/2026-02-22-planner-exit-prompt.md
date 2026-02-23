# Planner-Exit Auto-Prompt Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a confirmation dialog when a planner session finishes, asking the user whether to start implementation — eliminating the invisible UX gap between "planner done" and "implementation starts".

**Architecture:** Add planner-exit detection to the existing metadata tick loop (alongside reviewer-exit and coder-exit detection), using the same `confirmAction` pattern. A `plannerPrompted` map prevents re-prompting after the user responds. Three outcomes: yes (start implement), no (kill planner, leave plan ready), esc (preserve everything, re-prompt next tick).

**Tech Stack:** Go, bubbletea (tea.Cmd/tea.Msg pattern), existing overlay system

---

## Wave 1

### Task 1: Add `plannerPrompted` field and `plannerCompleteMsg` type

**Files:**
- Modify: `app/app.go:191-198` (add field after `waveOrchestrators`)
- Modify: `app/app.go:234` (initialize in `newHome`)
- Modify: `app/app.go:906-908` (add msg type after `waveAbortMsg`)

**Step 1: Add `plannerPrompted` field to `home` struct**

In `app/app.go`, after the `pendingWaveAbortAction` field (line 198), add:

```go
	// plannerPrompted tracks plan files whose planner-exit dialog has been
	// answered (yes or no). Prevents re-prompting every metadata tick.
	// NOT set on esc — allows re-prompt.
	plannerPrompted map[string]bool
```

**Step 2: Initialize the map in `newHome`**

In `app/app.go`, in the `newHome` function where `waveOrchestrators` is initialized (line 234), add:

```go
		plannerPrompted:   make(map[string]bool),
```

**Step 3: Add `plannerCompleteMsg` type**

In `app/app.go`, after the `waveAbortMsg` type (around line 908), add:

```go
// plannerCompleteMsg is sent when the user confirms starting implementation
// after a planner session finishes.
type plannerCompleteMsg struct {
	planFile string
}
```

**Step 4: Verify it compiles**

Run: `go build ./app/...`
Expected: clean compile

**Step 5: Commit**

```bash
git add app/app.go
git commit -m "feat: add plannerPrompted field and plannerCompleteMsg type"
```

### Task 2: Add planner-exit detection in metadata tick

**Files:**
- Modify: `app/app.go:566-567` (insert planner-exit detection block between reviewer-exit and coder-exit blocks)

The planner-exit detection goes **after** the reviewer-exit block (ends ~line 566) and **before** the coder-exit comment at line 568. This ordering matters: reviewer-exit auto-completes plans, planner-exit prompts for implement, coder-exit prompts for push.

**Step 1: Add planner-exit detection loop**

Insert after the reviewer-exit block's closing brace (after line 566, before the "Coder-exit" comment at line 568):

```go
			// Planner-exit → implement-prompt: when a planner session's tmux pane
			// has exited and the plan status is ready, prompt the user to start
			// implementation. Skip if already prompted (yes/no answered) or if a
			// confirm overlay is already showing.
			for _, inst := range m.list.GetInstances() {
				if m.state == stateConfirm {
					break
				}
				if inst.AgentType != session.AgentTypePlanner || inst.PlanFile == "" {
					continue
				}
				if m.plannerPrompted[inst.PlanFile] {
					continue
				}
				alive, collected := tmuxAliveMap[inst.Title]
				if !collected || alive {
					continue
				}
				entry, ok := m.planState.Entry(inst.PlanFile)
				if !ok || entry.Status != planstate.StatusReady {
					continue
				}
				capturedPlanFile := inst.PlanFile
				capturedTitle := inst.Title
				m.confirmAction(
					fmt.Sprintf("Plan '%s' is ready. Start implementation?", planstate.DisplayName(capturedPlanFile)),
					func() tea.Msg {
						return plannerCompleteMsg{planFile: capturedPlanFile}
					},
				)
				// Store the planner instance title so the yes/no handlers can find it.
				m.pendingPlannerInstanceTitle = capturedTitle
				break // one prompt per tick
			}
```

**Step 2: Add `pendingPlannerInstanceTitle` field to `home` struct**

In `app/app.go`, after the `plannerPrompted` field, add:

```go
	// pendingPlannerInstanceTitle is the title of the planner instance that
	// triggered the current planner-exit confirmation dialog.
	pendingPlannerInstanceTitle string
```

**Step 3: Verify it compiles**

Run: `go build ./app/...`
Expected: clean compile

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat: detect planner exit and show implement confirmation dialog"
```

### Task 3: Handle `plannerCompleteMsg` in Update (yes path)

**Files:**
- Modify: `app/app.go:725-729` (add case after `waveAbortMsg` handler)

**Step 1: Add `plannerCompleteMsg` handler**

In the `Update` method's message switch, after the `waveAbortMsg` case (around line 729), add:

```go
	case plannerCompleteMsg:
		// User confirmed: start implementation. Kill the dead planner instance first.
		m.plannerPrompted[msg.planFile] = true
		if m.pendingPlannerInstanceTitle != "" {
			m.removeFromAllInstances(m.pendingPlannerInstanceTitle)
			m.list.RemoveByTitle(m.pendingPlannerInstanceTitle)
			m.saveAllInstances()
		}
		m.pendingPlannerInstanceTitle = ""
		m.updateSidebarItems()
		return m.triggerPlanStage(msg.planFile, "implement")
```

**Step 2: Check if `list.RemoveByTitle` exists; if not, use `list.Kill` pattern or find equivalent**

Search for how instances are removed from the list. The `removeFromAllInstances` handles the master list; the UI list may need `list.RemoveByTitle` or similar. Check `ui/list.go` for available methods.

If `RemoveByTitle` doesn't exist, the planner instance removal from the UI list can be handled by calling `m.updateSidebarItems()` which rebuilds the list from `allInstances`. After `removeFromAllInstances`, the next `updateSidebarItems` call will exclude it.

Adjust the handler to:

```go
	case plannerCompleteMsg:
		m.plannerPrompted[msg.planFile] = true
		if m.pendingPlannerInstanceTitle != "" {
			m.removeFromAllInstances(m.pendingPlannerInstanceTitle)
			m.saveAllInstances()
		}
		m.pendingPlannerInstanceTitle = ""
		m.updateSidebarItems()
		return m.triggerPlanStage(msg.planFile, "implement")
```

**Step 3: Verify it compiles**

Run: `go build ./app/...`
Expected: clean compile

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat: handle plannerCompleteMsg to start implementation after planner exit"
```

### Task 4: Handle no/esc paths in confirm key handler

**Files:**
- Modify: `app/app_input.go:596-609` (the cancel/esc branch of `stateConfirm`)

The existing cancel/esc handler at line 596 handles wave-confirm resets. We need to add planner-exit cleanup here too.

**Step 1: Add planner cleanup to the cancel key path (no)**

In `app/app_input.go`, the cancel key handler (line 596) currently does:

```go
		case m.confirmationOverlay.CancelKey, "esc":
			// Reset the orchestrator confirm latch...
			if m.pendingWaveConfirmPlanFile != "" {
				...
			}
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			return m, nil
```

Replace this with logic that distinguishes between cancel (n) and esc:

```go
		case m.confirmationOverlay.CancelKey:
			// "No" — user explicitly declined.
			if m.pendingWaveConfirmPlanFile != "" {
				if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmPlanFile]; ok {
					orch.ResetConfirm()
				}
				m.pendingWaveConfirmPlanFile = ""
			}
			// Planner-exit "no": kill planner instance, mark prompted, leave plan ready.
			if m.pendingPlannerInstanceTitle != "" {
				m.plannerPrompted[m.pendingPlannerInstanceTitle] = false // keyed by planFile below
				// Find the plan file from the instance before removing it.
				for _, inst := range m.list.GetInstances() {
					if inst.Title == m.pendingPlannerInstanceTitle {
						m.plannerPrompted[inst.PlanFile] = true
						break
					}
				}
				m.removeFromAllInstances(m.pendingPlannerInstanceTitle)
				m.saveAllInstances()
				m.updateSidebarItems()
				m.pendingPlannerInstanceTitle = ""
			}
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			return m, nil
		case "esc":
			// Esc — preserve everything, allow re-prompt.
			if m.pendingWaveConfirmPlanFile != "" {
				if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmPlanFile]; ok {
					orch.ResetConfirm()
				}
				m.pendingWaveConfirmPlanFile = ""
			}
			// Planner-exit esc: do NOT mark plannerPrompted — allows re-prompt next tick.
			m.pendingPlannerInstanceTitle = ""
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			return m, nil
```

**Important:** The existing code has `case m.confirmationOverlay.CancelKey, "esc":` as a single case. This must be split into two separate cases. The default CancelKey is `"n"`, so this splits `n` and `esc` into distinct behaviors.

**Step 2: Verify it compiles**

Run: `go build ./app/...`
Expected: clean compile

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "feat: split cancel/esc handling for planner-exit confirmation dialog"
```

## Wave 2

### Task 5: Write tests for planner-exit detection and confirmation flow

**Files:**
- Modify: `app/app_wave_orchestration_flow_test.go` (add tests at end of file)

**Step 1: Write test for planner-exit detection showing confirm dialog**

```go
// TestPlannerExit_ShowsImplementConfirm verifies that when a planner session's
// tmux pane dies and the plan status is ready, a confirmation dialog appears
// asking whether to start implementation.
func TestPlannerExit_ShowsImplementConfirm(t *testing.T) {
	const planFile = "2026-02-22-planner-exit.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "planner exit test", "plan/planner-exit", time.Now()))
	// Plan is ready (planner wrote it back)
	// StatusReady is the initial registration status — no SetStatus needed.

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-exit",
		Path:    t.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = make(map[string]bool)
	_ = h.list.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "planner-exit", TmuxAlive: false}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"dead planner with ready plan must trigger implement confirmation")
	require.NotNil(t, updated.confirmationOverlay)
}
```

**Step 2: Write test for planner-exit NOT prompting when already prompted**

```go
func TestPlannerExit_NoRePromptAfterAnswer(t *testing.T) {
	const planFile = "2026-02-22-no-reprompt.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "no reprompt test", "plan/no-reprompt", time.Now()))

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "no-reprompt",
		Path:    t.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = map[string]bool{planFile: true} // already answered
	_ = h.list.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "no-reprompt", TmuxAlive: false}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"must not re-prompt after user already answered")
}
```

**Step 3: Write test for planner still alive — no prompt**

```go
func TestPlannerExit_NoPromptWhileAlive(t *testing.T) {
	const planFile = "2026-02-22-still-alive.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "still alive test", "plan/still-alive", time.Now()))

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "still-alive",
		Path:    t.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = make(map[string]bool)
	_ = h.list.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "still-alive", TmuxAlive: true}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"must not prompt while planner tmux pane is still alive")
}
```

**Step 4: Run the tests**

Run: `go test ./app/ -run TestPlannerExit -v`
Expected: all 3 tests pass

**Step 5: Commit**

```bash
git add app/app_wave_orchestration_flow_test.go
git commit -m "test: add planner-exit confirmation dialog tests"
```

### Task 6: End-to-end verification

**Step 1: Run full test suite**

Run: `go test ./app/ -v -count=1`
Expected: all tests pass, no regressions

**Step 2: Run full project build**

Run: `go build ./...`
Expected: clean compile

**Step 3: Run linter if available**

Run: `go vet ./app/...`
Expected: no issues

**Step 4: Final commit if any fixups needed**

Only if previous steps required changes.
