# Rewrite Tmux Layer Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in `session/tmux/` to remove AGPL-tainted lines, enabling a license change. The rewrite preserves identical behavior and public API while replacing every line that traces back to the ByteMirror/claude-squad fork point (`bbc8cad`).

**Architecture:** Each file in `session/tmux/` is rewritten in-place. The public API (struct fields, method signatures, exported functions) stays identical so callers compile without changes. The rewrite focuses on internal implementation: control flow, variable naming, error handling patterns, and code organization. Files that are 100% original (pty.go, shell.go, env_vars, session_title) are untouched. Existing tests (743 LOC in tmux_test.go + other test files) serve as the regression suite — they must all pass after each task.

**Tech Stack:** Go 1.24, tmux CLI, creack/pty, charmbracelet/x/ansi, testify

**Size:** Medium (estimated ~3 hours, 5 tasks, 2 waves)

---

## Wave 1: Core Session Management

### Task 1: Rewrite tmux.go — Session Lifecycle

**Files:**
- Modify: `session/tmux/tmux.go`
- Test: `session/tmux/tmux_test.go` (existing, no changes needed)

**Step 1: write the failing test**

No new tests needed — the existing `tmux_test.go` (743 LOC) covers `NewTmuxSession`, `Start`, `Restore`, `Close`, `DoesSessionExist`, `CleanupSessions`, `DiscoverOrphans`, `DiscoverAll`, `CountKasSessions`, `SetAgentType`, `SetInitialPrompt`, and prompt file handling. These tests ARE the regression gate.

**Step 2: run test to verify it passes (baseline)**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS — all existing tests pass before we touch anything

**Step 3: rewrite implementation**

Rewrite `session/tmux/tmux.go` from scratch based on this functional spec:

**Struct `TmuxSession`:** Holds session state — sanitized name (prefixed with `kas_`), program string, PTY file descriptor, status monitor, attach state (context, cancel, waitgroup, channel), and configuration fields (skipPermissions, agentType, initialPrompt, taskNumber/waveNumber/peerCount, progressFunc, promptFile, sessionTitle, titleFunc). The struct layout and field names must match exactly since tests access them directly.

**Key functions to rewrite (new implementation, same signatures):**
- `toKasTmuxName(str)` — sanitize name: strip whitespace, replace dots with underscores, prepend `kas_`
- `NewTmuxSession`, `NewTmuxSessionWithDeps`, `newTmuxSession` — constructors
- `NewReset` — create fresh session preserving injected deps
- `NewTmuxSessionFromExisting` — wrap existing tmux session
- `SetAgentType`, `SetInitialPrompt`, `SetTaskEnv`, `SetSessionTitle`, `SetTitleFunc` — setters
- `Start(workDir)` — create tmux session with program command, poll for existence, configure (history-limit, status off, KASMOS_MANAGED env), restore, wait for program ready string
- `Restore()` — attach to existing session, create status monitor
- `Close()` — close PTY, kill tmux session, clean up prompt file
- `DoesSessionExist()` — exact-match check via `tmux has-session -t=name`
- `CleanupSessions(cmdExec)` — list all tmux sessions, kill kas_/klique_/hivemind_ prefixed ones
- `DiscoverOrphans(cmdExec, knownNames)` — list kas_ sessions not in knownNames
- `DiscoverAll(cmdExec, knownNames)` — list all kas_ sessions with managed flag
- `CountKasSessions(cmdExec)` — count kas_ prefixed sessions
- `ToKasTmuxNamePublic(name)` — exported wrapper
- `statusMonitor` — hash-based change detection with ANSI stripping and debounce (15 ticks)
- Program detection helpers: `isClaudeProgram`, `isAiderProgram`, `isGeminiProgram`, `isOpenCodeProgram`

**Rewrite approach:** Write the entire file from scratch. Use the test suite as the specification. Do not copy-paste from the current file — write fresh implementations that satisfy the same contracts. Variable names, control flow, and error message strings should differ from upstream where possible without breaking test assertions that check specific strings.

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS — all existing tests pass with the rewritten implementation

**Step 5: commit**

```bash
git add session/tmux/tmux.go
git commit -m "feat(clean-room): rewrite session/tmux/tmux.go from scratch"
```

### Task 2: Rewrite tmux_io.go — Pane I/O and Status Detection

**Files:**
- Modify: `session/tmux/tmux_io.go`
- Test: `session/tmux/tmux_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `TapEnter`, `TapDAndEnter`, `SendKeys`, `HasUpdated`, `CapturePaneContent`. No new tests needed.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/tmux/... -run "TestTapEnter|TestTapDAndEnter|TestSendKeys" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/tmux/tmux_io.go` from scratch based on this functional spec:

**Key functions:**
- `TapRight()` — send Right arrow via `tmux send-keys`
- `SendPermissionResponse(choice)` — two-step permission flow: select choice (arrow keys + Enter), delay 300ms, confirm (Enter)
- `TapEnter()` — send Enter via `tmux send-keys`
- `TapDAndEnter()` — send D + Enter via `tmux send-keys`
- `SendKeys(keys)` — send literal text via `tmux send-keys -l`
- `HasUpdated()` — capture pane, detect program-specific prompts, hash content, debounce unchanged ticks
- `HasUpdatedWithContent()` — same as HasUpdated but also returns raw content
- `CapturePaneContent()` — `tmux capture-pane -p -e -J`
- `CapturePaneContentWithOptions(start, end)` — capture with line range
- `GetPTY()`, `GetSanitizedName()`, `GetPanePID()` — accessors

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux_io.go
git commit -m "feat(clean-room): rewrite session/tmux/tmux_io.go from scratch"
```

### Task 3: Rewrite tmux_attach.go — Attach/Detach Lifecycle

**Files:**
- Modify: `session/tmux/tmux_attach.go`
- Test: `session/tmux/tmux_test.go` (existing)

**Step 1: write the failing test**

Existing tests exercise Attach/Detach indirectly through Start/Restore. No new tests needed — the compile + existing suite is sufficient.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/tmux/tmux_attach.go` from scratch:

**Key functions:**
- `Attach()` — disable outer tmux mouse, create attach channel, spawn stdout copy goroutine, spawn stdin reader goroutine (with 50ms initial nuke window, Ctrl+Q/Ctrl+Space detach), call monitorWindowSize, return channel
- `restoreOuterMouse()` — re-enable mouse on outer tmux if it was disabled
- `DetachSafely()` — safe detach: close PTY, close channel, cancel context, wait goroutines, restore mouse
- `Detach()` — panicking detach: close PTY, restore via Restore(), cancel context, wait goroutines
- `SetDetachedSize(width, height)` — set window size while detached
- `updateWindowSize(cols, rows)` — set PTY window size via pty.Setsize
- `outerTmuxSession()` — detect enclosing tmux session name
- `outerMouseEnabled(session)` — check if mouse is on in given tmux session

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux_attach.go
git commit -m "feat(clean-room): rewrite session/tmux/tmux_attach.go from scratch"
```

## Wave 2: Platform-Specific Window Monitoring

> **depends on wave 1:** The monitorWindowSize methods in tmux_unix.go and tmux_windows.go are called by Attach() in tmux_attach.go. Wave 1 must land first so the Attach rewrite is stable.

### Task 4: Rewrite tmux_unix.go — Unix Window Size Monitor

**Files:**
- Modify: `session/tmux/tmux_unix.go`
- Test: `session/tmux/tmux_test.go` (existing)

**Step 1: write the failing test**

monitorWindowSize is an internal method exercised by Attach. No new tests needed.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/tmux/tmux_unix.go` from scratch:

**Spec:** `monitorWindowSize()` — listen for SIGWINCH, debounce resize events (50ms timer), update PTY window size. Uses two goroutines: one debounces raw SIGWINCH into a channel, the other reads debounced signals and calls updateWindowSize. Both goroutines respect `t.ctx.Done()` for cleanup. Initial size set via deferred doUpdate.

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux_unix.go
git commit -m "feat(clean-room): rewrite session/tmux/tmux_unix.go from scratch"
```

### Task 5: Rewrite tmux_windows.go — Windows Window Size Monitor

**Files:**
- Modify: `session/tmux/tmux_windows.go`
- Test: `session/tmux/tmux_test.go` (existing)

**Step 1: write the failing test**

Windows build tag means this only compiles on Windows. No new tests needed — compile check is sufficient.

**Step 2: run test to verify baseline passes**

```bash
GOOS=windows go vet ./session/tmux/...
```

expected: PASS (vet passes)

**Step 3: rewrite implementation**

Rewrite `session/tmux/tmux_windows.go` from scratch:

**Spec:** `monitorWindowSize()` — no SIGWINCH on Windows. Poll terminal size every 250ms via ticker. Track last known cols/rows, call doUpdate only when dimensions change. Single goroutine respects `t.ctx.Done()`.

**Step 4: run test to verify it passes**

```bash
GOOS=windows go vet ./session/tmux/...
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux_windows.go
git commit -m "feat(clean-room): rewrite session/tmux/tmux_windows.go from scratch"
```
