# Quit Confirmation Dialog Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a confirmation dialog when the user presses `q` or `ctrl+c` if any agent sessions are actively running or loading, preventing accidental loss of in-progress work.

**Architecture:** Modify `handleQuit()` in `app/app.go` to check for active instances (status `Running` or `Loading`) and conditionally show a confirmation overlay using the existing `confirmAction` helper. If no instances are active, quit immediately as before. The quit action saves instances then returns `tea.Quit`. `ctrl+c` follows the same path — both already call `handleQuit()` from the same branch in `app_input.go:1033`.

**Tech Stack:** Go, bubbletea, existing `overlay.ConfirmationOverlay`

**Size:** Trivial (estimated ~20 min, 1 task, 1 wave)

---

## Wave 1

### Task 1: Add active-session guard to handleQuit and test both paths

**Files:**
- Modify: `app/app.go:1347-1352` — `handleQuit()` method
- Test: `app/app_test.go` — add two tests

The current `handleQuit()` unconditionally saves and quits:

```go
func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}
```

**Step 1: Write the failing tests**

Add to `app/app_test.go`:

```go
func TestHandleQuit_NoActiveSessions_QuitsImmediately(t *testing.T) {
	h := newTestHome()
	h.toastManager = overlay.NewToastManager(&h.spinner)

	// Add a paused instance (not active)
	inst := &session.Instance{Title: "paused-agent", Status: session.Paused}
	h.nav.AddInstance(inst)

	_, cmd := h.handleQuit()

	// Should return tea.Quit directly (no confirmation)
	assert.Equal(t, stateDefault, h.state, "state should remain default (no confirmation overlay)")
	assert.Nil(t, h.confirmationOverlay, "no confirmation overlay should be shown")
	require.NotNil(t, cmd, "should return a quit command")
}

func TestHandleQuit_ActiveSessions_ShowsConfirmation(t *testing.T) {
	h := newTestHome()
	h.toastManager = overlay.NewToastManager(&h.spinner)

	// Add a running instance
	inst := &session.Instance{Title: "running-agent", Status: session.Running}
	h.nav.AddInstance(inst)

	_, cmd := h.handleQuit()

	// Should show confirmation, not quit immediately
	assert.Equal(t, stateConfirm, h.state, "state should be stateConfirm")
	require.NotNil(t, h.confirmationOverlay, "confirmation overlay must be shown")
	assert.Nil(t, cmd, "confirmAction returns nil cmd (action stored in pendingConfirmAction)")
	assert.NotNil(t, h.pendingConfirmAction, "pending action must be set")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/ -run 'TestHandleQuit_' -v`
Expected: FAIL — `TestHandleQuit_ActiveSessions_ShowsConfirmation` fails because `handleQuit` always returns `tea.Quit` without checking for active sessions.

**Step 3: Implement the active-session guard**

Replace `handleQuit()` in `app/app.go`:

```go
func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	// Check if any instances are actively running or loading.
	hasActive := false
	for _, inst := range m.nav.GetInstances() {
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasActive = true
			break
		}
	}

	if hasActive {
		quitAction := func() tea.Msg {
			_ = m.saveAllInstances()
			return tea.QuitMsg{}
		}
		return m, m.confirmAction("quit kasmos? active sessions will be preserved.", quitAction)
	}

	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}
```

Key design decisions:
- The confirmation message is lowercase per project standards.
- The confirm action saves instances then returns `tea.QuitMsg{}` so bubbletea processes the quit.
- Only `Running` and `Loading` statuses count as "active" — `Ready` and `Paused` instances don't block quit.
- `ctrl+c` and `q` both go through `handleQuit()` already (line 1033 of `app_input.go`), so both paths are covered.

**Step 4: Run tests to verify they pass**

Run: `go test ./app/ -run 'TestHandleQuit_' -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: All tests pass, no regressions.

**Step 6: Commit**

```bash
git add app/app.go app/app_test.go
git commit -m "feat: quit confirmation dialog when active sessions exist"
```
