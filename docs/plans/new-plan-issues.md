# New Plan Overlay Bugs Implementation Plan

**Goal:** Fix two bugs in the new-plan creation flow: (1) async confirmation dialogs (planner finished, wave complete, coder exit) clobber the new-plan overlay state, losing all user input; (2) the new-plan text input box grows when the agent pane loads a session or switches, because `tea.WindowSize()` emitted as a side-effect triggers a percentage-based resize.

**Architecture:** Both bugs live in `app/app.go`. Bug 1: the `metadataResultMsg` handler has four code paths that transition to `stateConfirm` but only guard against `m.state == stateConfirm` — they don't check for `stateNewPlan`, `stateNewPlanTopic`, or any other active overlay state. Fix: add an `isUserInOverlay()` predicate and use it everywhere. Bug 2: `updateHandleWindowSizeEvent()` unconditionally calls `textInputOverlay.SetSize(width*0.6, height*0.4)` on every `tea.WindowSizeMsg`, but `tea.WindowSize()` is emitted as a batched side-effect by `instanceStartedMsg`, `killInstanceMsg`, and several other handlers. Fix: track whether the terminal dimensions actually changed and only resize overlays when they did.

**Tech Stack:** Go, bubbletea (tea.Cmd/tea.Msg), overlay package

**Size:** Small (estimated ~40 min, 1 task, 1 wave)

---

## Wave 1: Overlay State Protection and Resize Guard

### Task 1: Guard overlay states from async interrupts and spurious resizes

**Files:**
- Modify: `app/app.go`
- Modify: `ui/overlay/textInput.go`
- Test: `app/app_plan_creation_test.go`

**Step 1: write the failing test**

Add two tests: one for the confirmation-clobber bug, one for the resize bug.

```go
func TestIsUserInOverlay(t *testing.T) {
    tests := []struct {
        state    state
        expected bool
    }{
        {stateDefault, false},
        {stateNewPlan, true},
        {stateNewPlanTopic, true},
        {stateConfirm, true},
        {statePrompt, true},
        {stateSpawnAgent, true},
        {stateFocusAgent, true},
        {statePermission, true},
    }
    for _, tt := range tests {
        h := &home{state: tt.state}
        require.Equal(t, tt.expected, h.isUserInOverlay(),
            "isUserInOverlay() for state %d", tt.state)
    }
}

func TestNewPlanOverlaySizePreservedOnSpuriousWindowSize(t *testing.T) {
    s := spinner.New()
    h := &home{
        state:        stateNewPlan,
        tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
        nav:          ui.NewNavigationPanel(&s),
        menu:         ui.NewMenu(),
    }
    // Simulate initial terminal size.
    h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

    // Now create the overlay with a fixed size.
    h.textInputOverlay = overlay.NewTextInputOverlay("new plan", "")
    h.textInputOverlay.SetMultiline(true)
    h.textInputOverlay.SetSize(70, 8)

    // Simulate a spurious WindowSize (same dimensions, triggered by instanceStartedMsg).
    h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

    // Overlay should still be 70 wide, not 120 (200*0.6).
    require.Equal(t, 70, h.textInputOverlay.Width())
    require.Equal(t, 8, h.textInputOverlay.Height())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run "TestIsUserInOverlay|TestNewPlanOverlaySizePreservedOnSpuriousWindowSize" -v
```

expected: FAIL — `isUserInOverlay` undefined, `Width()`/`Height()` undefined, and the resize check doesn't exist.

**Step 3: write minimal implementation**

Three changes in `app/app.go`:

**(a)** Add `isUserInOverlay()` method:

```go
// isUserInOverlay returns true when the user is actively interacting with
// any modal overlay. Used to prevent async metadata-tick handlers from
// clobbering the active overlay by showing a confirmation dialog.
func (m *home) isUserInOverlay() bool {
    switch m.state {
    case stateDefault:
        return false
    }
    return true
}
```

This is deliberately conservative — every non-default state counts as "in overlay". This prevents any async handler from interrupting any active state.

**(b)** Replace state guards in `metadataResultMsg` handler. Four locations:

1. PlannerFinished signal (~line 695):
   ```go
   // before: if m.plannerPrompted[capturedPlanFile] || m.state == stateConfirm {
   if m.plannerPrompted[capturedPlanFile] || m.isUserInOverlay() {
   ```

2. Coder-exit push prompt loop (~line 907):
   ```go
   // before: if m.state == stateConfirm {
   if m.isUserInOverlay() {
   ```

3. Wave all-complete (~line 999):
   ```go
   // before: if m.state != stateConfirm {
   if !m.isUserInOverlay() {
   ```

4. Wave decision confirm (~line 1015):
   ```go
   // before: if m.state != stateConfirm && ...
   if !m.isUserInOverlay() && ...
   ```

**(c)** Guard overlay resize in `updateHandleWindowSizeEvent()`. Before setting `m.termWidth`/`m.termHeight`, check whether the terminal dimensions actually changed. Only resize overlays when they did:

```go
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
    // ... navWidth, tabsWidth calculation (unchanged) ...

    // Detect actual terminal resize vs spurious tea.WindowSize() side-effects.
    termResized := msg.Width != m.termWidth || msg.Height != m.termHeight

    m.termWidth = msg.Width
    m.termHeight = msg.Height
    // ... toast, statusBar, tabbedWindow, nav sizing (unchanged) ...

    // Only resize overlays when the terminal dimensions actually changed.
    // Many handlers emit tea.WindowSize() as a batched side-effect (e.g.
    // instanceStartedMsg) — those fire with the same dimensions and should
    // not overwrite the overlay's explicit sizing.
    if m.textInputOverlay != nil && termResized {
        m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
    }
    if m.textOverlay != nil && termResized {
        m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
    }

    // ... rest unchanged ...
}
```

**(d)** Add `Width()`/`Height()` accessors to `ui/overlay/textInput.go`:

```go
func (t *TextInputOverlay) Width() int  { return t.width }
func (t *TextInputOverlay) Height() int { return t.height }
```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run "TestIsUserInOverlay|TestNewPlanOverlaySizePreservedOnSpuriousWindowSize" -v
```

expected: PASS

Run the full test suite to check for regressions:

```bash
go test ./app/... -v
go test ./ui/overlay/... -v
```

expected: all PASS

**Step 5: commit**

```bash
git add app/app.go app/app_plan_creation_test.go ui/overlay/textInput.go
git commit -m "fix: protect new-plan overlay from confirmation interrupts and spurious resizes"
```
