# Fix Double-Kill Requirement for Exited Instances

**Goal:** Allow single-action cleanup of instances whose tmux session has died naturally — currently users must press `k` (soft kill) before `delete` will work on exited instances like reviewers or fixers.

**Architecture:** The root cause is a status/guard mismatch: when the metadata tick detects a dead tmux session it sets `Exited = true` but leaves `Status = Running`. The delete handler (`KeyDelete`/`KeyBackspace`) blocks on `Status == Running`, forcing users to press `k` first to transition status to `Ready`. The fix updates the delete guard to also allow removal when `Exited` is true, transitions status to `Ready` at the point of tmux death detection so all downstream guards behave consistently, and makes the `k` keybind no-op on already-exited instances.

**Tech Stack:** Go, bubbletea (tea.Cmd/tea.Msg), session package (Instance, Status)

**Size:** Small (estimated ~30 min, 1 task, 1 wave)

---

## Wave 1: Fix exited-instance cleanup guards

### Task 1: Allow delete on exited instances, no-op kill on exited, transition status on tmux death

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app.go`
- Test: `app/app_test.go`

**Step 1: write the failing tests**

Add three tests to `app/app_test.go`:

1. `TestDeleteKey_AllowsRemovalOfExitedRunningInstance` — creates a home with an instance that has `Status = Running` and `Exited = true`, sends a `KeyDelete` message, and asserts the instance is removed from the nav panel.

2. `TestKillKey_NoopsOnExitedInstance` — creates a home with an exited instance, sends `KeyKill`, and asserts no async cmd is returned (the kill is a no-op since tmux is already dead).

3. `TestMetadataTick_ExitedInstanceTransitionsToReady` — creates a home model with a Running instance, sends a `metadataResultMsg` with `TmuxAlive: false`, and asserts both `Exited == true` and `Status == Ready`.

```go
func TestDeleteKey_AllowsRemovalOfExitedRunningInstance(t *testing.T) {
    h := newTestHome()
    inst, err := newTestInstance("exited-reviewer")
    require.NoError(t, err)
    inst.Status = session.Running
    inst.Exited = true
    _ = h.nav.AddInstance(inst)
    h.nav.SelectInstance(inst)
    h.allInstances = append(h.allInstances, inst)

    msg := tea.KeyMsg{Type: tea.KeyDelete}
    _, _ = h.handleKeyPress(msg)

    assert.Equal(t, 0, h.nav.TotalInstances(),
        "delete should remove exited instance even if status is Running")
}

func TestKillKey_NoopsOnExitedInstance(t *testing.T) {
    h := newTestHome()
    inst, err := newTestInstance("exited-reviewer")
    require.NoError(t, err)
    inst.Status = session.Running
    inst.Exited = true
    _ = h.nav.AddInstance(inst)
    h.nav.SelectInstance(inst)

    msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
    _, cmd := h.handleKeyPress(msg)

    assert.Nil(t, cmd, "k should no-op on an already-exited instance")
}

func TestMetadataTick_ExitedInstanceTransitionsToReady(t *testing.T) {
    h := newTestHome()
    inst, err := newTestInstance("reviewer-done")
    require.NoError(t, err)
    inst.Status = session.Running
    _ = h.nav.AddInstance(inst)
    h.allInstances = append(h.allInstances, inst)
    h.planState = &planstate.State{Plans: map[string]planstate.Entry{}}

    // Simulate metadata tick with dead tmux
    msg := metadataResultMsg{
        Results: []instanceMetadata{{Title: "reviewer-done", TmuxAlive: false}},
    }
    h.Update(msg)

    assert.True(t, inst.Exited, "instance should be marked exited")
    assert.Equal(t, session.Ready, inst.Status,
        "exited instance status should transition to Ready")
}
```

**Step 2: run tests to verify they fail**

```bash
go test ./app/... -run 'TestDeleteKey_AllowsRemovalOfExitedRunningInstance|TestKillKey_NoopsOnExitedInstance|TestMetadataTick_ExitedInstanceTransitionsToReady' -v
```

expected: FAIL — delete test fails because the guard blocks Running status; kill test fails because the handler returns a cmd instead of nil; metadata test fails because status stays Running after death detection.

**Step 3: write minimal implementation**

Three changes:

**a) `app/app_input.go` — delete guard (line ~1160):**

Change:
```go
if selected != nil && selected.Status != session.Running && selected.Status != session.Loading {
```
To:
```go
if selected != nil && (selected.Exited || (selected.Status != session.Running && selected.Status != session.Loading)) {
```

This allows delete when `Exited` is true regardless of status.

**b) `app/app_input.go` — kill guard (line ~1288):**

Change:
```go
if selected == nil || !selected.Started() || selected.Paused() {
```
To:
```go
if selected == nil || !selected.Started() || selected.Paused() || selected.Exited {
```

This makes `k` a no-op on already-exited instances (tmux is already dead, nothing to kill).

**c) `app/app.go` — tmux death detection (line ~1243-1244):**

After setting `Exited = true`, also transition status to `Ready` so all other guards (focus mode, send-yes, etc.) behave consistently:

```go
if collected && !alive {
    inst.Exited = true
    if inst.Status == session.Running {
        inst.SetStatus(session.Ready)
    }
    m.audit(auditlog.EventAgentFinished, fmt.Sprintf("agent finished: %s", inst.Title),
        auditlog.WithInstance(inst.Title),
        auditlog.WithAgent(inst.AgentType),
        auditlog.WithPlan(inst.PlanFile),
    )
}
```

**Step 4: run tests to verify they pass**

```bash
go test ./app/... -v -count=1
```

expected: PASS — all existing tests and new tests pass.

**Step 5: commit**

```bash
git add app/app_input.go app/app.go app/app_test.go
git commit -m "fix: allow single-action cleanup of exited instances without double-kill"
```
