# Live Preview Terminal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace capture-pane polling in the preview pane with a live EmbeddedTerminal that renders from a VT emulator, and unify it with focus mode so entering focus just forwards keys to the same terminal.

**Architecture:** A single `previewTerminal` on the `home` struct replaces `embeddedTerminal`. It attaches to the selected instance's tmux session via a dedicated PTY + VT emulator and renders directly from memory. Focus mode reuses the same terminal by toggling key forwarding. The `capture-pane` preview path and `focusPreviewTickMsg` are removed.

**Tech Stack:** Go 1.24+, bubbletea v1.3.x, charmbracelet/x/vt, creack/pty

**Design doc:** `docs/plans/2026-02-24-live-preview-terminal-design.md`

---

## Wave 1: Core terminal lifecycle

### Task 1: Add previewTerminal field and ready message

**Files:**
- Modify: `app/app.go:174-175` (replace `embeddedTerminal` field)
- Modify: `app/app.go:1000-1010` (add message types)

**Step 1: Replace `embeddedTerminal` with `previewTerminal` fields**

In the `home` struct, replace:

```go
// embeddedTerminal is the VT emulator for focus mode (nil when not in focus mode)
embeddedTerminal *session.EmbeddedTerminal
```

With:

```go
// previewTerminal is the VT emulator for the selected instance's preview.
// Also used for focus mode — entering focus just forwards keys to this terminal.
previewTerminal         *session.EmbeddedTerminal
previewTerminalInstance string // title of the instance the terminal is attached to
```

**Step 2: Add message types**

Near the existing `focusPreviewTickMsg` definition (~line 1005), add:

```go
// previewTerminalReadyMsg signals that the async terminal attach completed.
type previewTerminalReadyMsg struct {
	term          *session.EmbeddedTerminal
	instanceTitle string
	err           error
}
```

**Step 3: Fix all compilation errors from the rename**

Find every reference to `m.embeddedTerminal` in the codebase and replace with `m.previewTerminal`. Key locations:
- `app/app.go:413-427` (focusPreviewTickMsg handler)
- `app/app_input.go:530-538` (focus mode key forwarding)
- `app/app_state.go:153-158` (enterFocusMode)
- `app/app_state.go:195-197` (exitFocusMode)

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: clean build (behavior unchanged at this point — just a rename)

**Step 5: Commit**

```
feat(preview): rename embeddedTerminal to previewTerminal, add ready msg type
```

---

### Task 2: Async terminal spawn on selection change

**Files:**
- Modify: `app/app_state.go:425-462` (instanceChanged)
- Modify: `app/app.go` (add previewTerminalReadyMsg handler in Update)

**Step 1: Write test for selection-change terminal lifecycle**

Create test in `app/app_test.go` that:
- Sets up a `home` with a `previewTerminal` attached to instance "A"
- Simulates selecting instance "B"
- Asserts: old terminal is closed (previewTerminal becomes nil), previewTerminalInstance is cleared
- Asserts: a tea.Cmd is returned (the async spawn command)

**Step 2: Run test, verify it fails**

Run: `go test ./app -run TestPreviewTerminal_SelectionChange -v`
Expected: FAIL

**Step 3: Implement terminal swap in instanceChanged()**

In `instanceChanged()`, before the existing `UpdatePreview` call, add terminal lifecycle management:

```go
// Manage preview terminal lifecycle on selection change.
if selected == nil || !selected.Started() || selected.Status == session.Paused {
	// No valid instance — tear down terminal.
	if m.previewTerminal != nil {
		m.previewTerminal.Close()
		m.previewTerminal = nil
		m.previewTerminalInstance = ""
	}
} else if selected.Title != m.previewTerminalInstance {
	// Different instance selected — swap terminal.
	if m.previewTerminal != nil {
		m.previewTerminal.Close()
		m.previewTerminal = nil
		m.previewTerminalInstance = ""
	}
	cols, rows := m.tabbedWindow.GetPreviewSize()
	if cols < 10 {
		cols = 80
	}
	if rows < 5 {
		rows = 24
	}
	capturedTitle := selected.Title
	capturedInstance := selected
	spawnCmd = func() tea.Msg {
		term, err := capturedInstance.NewEmbeddedTerminalForInstance(cols, rows)
		return previewTerminalReadyMsg{term: term, instanceTitle: capturedTitle, err: err}
	}
}
```

Return `spawnCmd` from `instanceChanged()` when non-nil (change return type or batch it).

**Step 4: Handle previewTerminalReadyMsg in Update()**

Add a case in the `Update` switch:

```go
case previewTerminalReadyMsg:
	// Discard stale attach if selection changed while spawning.
	selected := m.list.GetSelectedInstance()
	if msg.err != nil || selected == nil || selected.Title != msg.instanceTitle {
		if msg.term != nil {
			msg.term.Close()
		}
		return m, nil
	}
	m.previewTerminal = msg.term
	m.previewTerminalInstance = msg.instanceTitle
	return m, nil
```

**Step 5: Run test, verify it passes**

Run: `go test ./app -run TestPreviewTerminal_SelectionChange -v`
Expected: PASS

**Step 6: Commit**

```
feat(preview): spawn previewTerminal on selection change with stale-discard
```

---

### Task 3: Render from previewTerminal instead of capture-pane

**Files:**
- Modify: `app/app.go:398-411` (previewTickMsg handler)

**Step 1: Rewrite the previewTickMsg handler**

Replace the current handler that calls `instanceChanged()` with:

```go
case previewTickMsg:
	// If previewTerminal is active, render from it (zero-latency VT emulator).
	if m.previewTerminal != nil && !m.tabbedWindow.IsDocumentMode() {
		if content, changed := m.previewTerminal.Render(); changed {
			m.tabbedWindow.SetPreviewContent(content)
		}
	}
	// Banner animation (only when no terminal is active / fallback showing).
	m.previewTickCount++
	if m.previewTickCount%20 == 0 {
		m.tabbedWindow.TickBanner()
	}
	// Use event-driven wakeup when terminal is live, fall back to 50ms poll otherwise.
	term := m.previewTerminal
	return m, func() tea.Msg {
		if term != nil {
			term.WaitForRender(50 * time.Millisecond)
		} else {
			time.Sleep(50 * time.Millisecond)
		}
		return previewTickMsg{}
	}
```

Note: `instanceChanged()` is still called from other places (selection change, key handlers, etc.) — it just no longer drives the preview content. Its remaining responsibilities (diff pane, menu, sidebar updates) stay intact.

**Step 2: Verify it compiles and existing tests pass**

Run: `go build ./... && go test ./app/... -v`
Expected: clean build, all tests pass

**Step 3: Commit**

```
feat(preview): render from previewTerminal VT emulator instead of capture-pane
```

---

## Wave 2: Unify focus mode

### Task 4: Rewrite enterFocusMode to reuse previewTerminal

**Files:**
- Modify: `app/app_state.go:139-166` (enterFocusMode)

**Step 1: Write test for focus mode reuse**

Test that entering focus mode when `previewTerminal` is already attached does NOT create a new terminal — it reuses the existing one and just sets the state.

**Step 2: Run test, verify it fails**

Run: `go test ./app -run TestFocusMode_ReusesPreviewTerminal -v`
Expected: FAIL

**Step 3: Rewrite enterFocusMode**

Replace the current implementation with:

```go
func (m *home) enterFocusMode() tea.Cmd {
	m.tabbedWindow.ClearDocumentMode()
	selected := m.list.GetSelectedInstance()
	if selected == nil || !selected.Started() || selected.Status == session.Paused {
		return nil
	}

	// If previewTerminal is already attached to this instance, just enter focus mode.
	if m.previewTerminal != nil && m.previewTerminalInstance == selected.Title {
		m.state = stateFocusAgent
		m.tabbedWindow.SetFocusMode(true)
		return nil
	}

	// No terminal yet (shouldn't normally happen) — spawn one synchronously-ish.
	cols, rows := m.tabbedWindow.GetPreviewSize()
	if cols < 10 {
		cols = 80
	}
	if rows < 5 {
		rows = 24
	}
	term, err := selected.NewEmbeddedTerminalForInstance(cols, rows)
	if err != nil {
		return m.handleError(err)
	}
	m.previewTerminal = term
	m.previewTerminalInstance = selected.Title
	m.state = stateFocusAgent
	m.tabbedWindow.SetFocusMode(true)
	return nil
}
```

**Step 4: Run test, verify it passes**

Run: `go test ./app -run TestFocusMode_ReusesPreviewTerminal -v`
Expected: PASS

**Step 5: Commit**

```
feat(preview): enterFocusMode reuses previewTerminal instead of spawning new
```

---

### Task 5: Simplify exitFocusMode — keep terminal alive

**Files:**
- Modify: `app/app_state.go:194-201` (exitFocusMode)

**Step 1: Write test for exit focus keeping terminal alive**

Test that exiting focus mode does NOT close `previewTerminal` — it stays alive for preview rendering.

**Step 2: Run test, verify it fails**

Run: `go test ./app -run TestExitFocusMode_KeepsPreviewTerminal -v`
Expected: FAIL

**Step 3: Simplify exitFocusMode**

Replace:

```go
func (m *home) exitFocusMode() {
	if m.embeddedTerminal != nil {
		m.embeddedTerminal.Close()
		m.embeddedTerminal = nil
	}
	m.state = stateDefault
	m.tabbedWindow.SetFocusMode(false)
}
```

With:

```go
func (m *home) exitFocusMode() {
	// previewTerminal stays alive — it continues rendering in normal preview mode.
	m.state = stateDefault
	m.tabbedWindow.SetFocusMode(false)
}
```

**Step 4: Run test, verify it passes**

Run: `go test ./app -run TestExitFocusMode_KeepsPreviewTerminal -v`
Expected: PASS

**Step 5: Commit**

```
feat(preview): exitFocusMode no longer kills terminal, preview stays live
```

---

### Task 6: Remove focusPreviewTickMsg — unified tick

**Files:**
- Modify: `app/app.go:412-428` (remove focusPreviewTickMsg handler)
- Modify: `app/app.go:1005-1006` (remove type definition)
- Modify: `app/app_state.go:162-165` (enterFocusMode no longer returns focus tick)

**Step 1: Remove the focusPreviewTickMsg type and handler**

Delete the `focusPreviewTickMsg` struct definition and its entire case block in `Update()`. The `previewTickMsg` handler (already updated in Task 3) handles both normal and focus mode rendering.

**Step 2: Update enterFocusMode to not start a focus tick**

`enterFocusMode` no longer needs to return `func() tea.Msg { return focusPreviewTickMsg{} }` — the `previewTickMsg` loop is already running from `Init()`.

**Step 3: Verify build and tests pass**

Run: `go build ./... && go test ./app/... -v`
Expected: all pass

**Step 4: Commit**

```
refactor(preview): remove focusPreviewTickMsg, unified render tick
```

---

## Wave 3: Resize, spinner, and dead code cleanup

### Task 7: Resize previewTerminal on WindowSizeMsg

**Files:**
- Modify: `app/app.go` (WindowSizeMsg handler, around line 340)

**Step 1: Add resize call**

In the `tea.WindowSizeMsg` handler, after `m.tabbedWindow.SetSize(...)` where the preview dimensions are known, add:

```go
if m.previewTerminal != nil {
	cols, rows := m.tabbedWindow.GetPreviewSize()
	m.previewTerminal.Resize(cols, rows)
}
```

**Step 2: Verify build and test**

Run: `go build ./... && go test ./app/... -v`

**Step 3: Commit**

```
feat(preview): resize previewTerminal on window size change
```

---

### Task 8: Show spinner while terminal is attaching

**Files:**
- Modify: `app/app.go` (previewTickMsg handler)
- Modify: `ui/preview.go` or use existing `setFallbackState`

**Step 1: When previewTerminal is nil and an instance is selected, show spinner**

In the `previewTickMsg` handler, when `m.previewTerminal == nil` and a valid instance is selected (meaning we're waiting for attach), call the fallback path to show a loading indicator. The existing `setFallbackState` with a "connecting..." message or similar works here.

Integrate with the `instanceChanged()` flow — when the terminal is torn down for a selection change, `instanceChanged()` still calls `UpdatePreview()` which will show the loading/fallback state since there's no cached content for the new instance yet.

**Step 2: Verify visually**

Run the app, switch between instances, confirm spinner shows briefly before live content appears.

**Step 3: Commit**

```
feat(preview): show loading indicator while terminal attaches
```

---

### Task 9: Remove capture-pane preview dead code

**Files:**
- Modify: `ui/preview.go:151-249` (remove `UpdateContent` method body, keep for nil/loading/paused fallback)
- Modify: `ui/tabbed_window.go:142-149` (simplify `UpdatePreview`)
- Modify: `session/instance_session.go:29-39` (remove `PreviewCached`)
- Modify: `session/instance.go:109` (remove `CachedContent` usage for preview, keep if metadata needs it)
- Modify: `app/app_state.go:445` (remove `UpdatePreview` call from `instanceChanged` for agent tab)
- Remove: `ui/preview_test.go` tests that test `UpdateContent` with capture-pane mocking
- Remove: `session/instance_session_async_test.go` `TestPreviewCached_*` tests

**Step 1: Audit all callers of the dead code paths**

Use `ast-grep` or grep to find all callers of `PreviewCached`, `UpdateContent`, `UpdatePreview` and confirm they're only used by the preview pipeline (not metadata/status detection).

**Step 2: Remove or simplify each dead path**

- `UpdateContent`: keep the nil/loading/paused fallback branches (they set fallback messages), remove the `PreviewCached()` call and content rendering branch.
- `PreviewCached`: delete entirely.
- `UpdatePreview`: simplify to only handle fallback states (nil instance, loading, paused). The live content comes from `SetPreviewContent` via the tick loop.

**Step 3: Remove tests that test the deleted code paths**

Delete tests that mock `capture-pane` for preview rendering. Keep tests for fallback states.

**Step 4: Verify build and full test suite**

Run: `go build ./... && go test ./... -v`
Expected: all pass, no references to deleted functions remain

**Step 5: Commit**

```
refactor(preview): remove capture-pane preview path, dead code cleanup
```

---

### Task 10: Update tests for new preview terminal architecture

**Files:**
- Modify: `app/app_test.go` (update any tests referencing `embeddedTerminal`)
- Modify: `app/app_wave_orchestration_flow_test.go` (if any test helpers reference old field)
- Modify: `ui/tabbed_window_test.go` (update preview-related tests)

**Step 1: Fix any remaining test compilation errors**

Search for `embeddedTerminal` across all test files and update to `previewTerminal`.

**Step 2: Add integration-style test**

Test the full flow: selection change → previewTerminalReadyMsg → render tick picks up content → selection change again → old terminal discarded, new one attached.

**Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: all pass

**Step 4: Commit**

```
test(preview): update tests for previewTerminal architecture
```
