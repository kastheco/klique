# Rewrite Session Core Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in the session core layer (instance.go, instance_lifecycle.go, instance_session.go, storage.go, terminal.go) to remove AGPL-tainted lines. Preserves identical public API and behavior.

**Architecture:** Five files rewritten in-place. The `Instance` struct, `Status` enum, `Storage` type, `EmbeddedTerminal`, and all exported methods keep exact signatures. The session layer depends on `session/tmux` and `session/git` (rewritten in plans 01 and 02). Existing tests (instance_test.go, instance_lifecycle_test.go, instance_session_async_test.go, instance_planfile_test.go, instance_title_test.go, instance_wave_test.go) serve as the regression suite.

**Tech Stack:** Go 1.24, charmbracelet/x/vt, creack/pty, testify

**Size:** Large (estimated ~4 hours, 5 tasks, 2 waves)

---

## Wave 1: Data Layer and Terminal

### Task 1: Rewrite instance.go — Instance Struct and Serialization

**Files:**
- Modify: `session/instance.go`
- Test: `session/instance_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `NewInstance`, `ToInstanceData`, `FromInstanceData`, `SetStatus`, `SetTitle`, `Started`, `Paused`, `TmuxAlive`, `RepoName`, `GetRepoPath`, `GetWorktreePath`.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestNewInstance|TestToInstanceData|TestFromInstanceData|TestSetStatus" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/instance.go` from scratch:

- `Status` enum: Running, Ready, Loading, Paused
- `AgentType` constants: planner, coder, reviewer, fixer
- `Instance` struct — all fields preserved exactly (public + private)
- `ToInstanceData()` — serialize Instance to InstanceData
- `FromInstanceData(data)` — deserialize: create Instance, restore gitWorktree from storage, check tmux session existence, handle paused/exited/live states
- `InstanceOptions` struct and `NewInstance(opts)` constructor
- Accessors: `RepoName()`, `GetRepoPath()`, `GetWorktreePath()`, `SetStatus()`, `setLoadingProgress()`, `Started()`, `SetTitle()`, `Paused()`, `TmuxAlive()`

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance.go
git commit -m "feat(clean-room): rewrite session/instance.go from scratch"
```

### Task 2: Rewrite storage.go — Instance Persistence

**Files:**
- Modify: `session/storage.go`
- Test: `session/instance_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `NewStorage`, `SaveInstances`, `LoadInstances`, `DeleteInstance`, `UpdateInstance`, `DeleteAllInstances`.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestStorage|TestSave|TestLoad|TestDelete" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/storage.go` from scratch:

- `InstanceData`, `GitWorktreeData`, `DiffStatsData` — serializable structs with JSON tags
- `Storage` struct wrapping `config.StateManager`
- `NewStorage(state)` — constructor
- `SaveInstances(instances)` — filter started, convert to InstanceData, marshal JSON, save via state
- `LoadInstances()` — unmarshal JSON, skip stale worktrees, skip unrestorable instances
- `DeleteInstance(title)` — load, filter, save
- `UpdateInstance(instance)` — load, find by title, replace, save
- `DeleteAllInstances()` — delegate to state

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/storage.go
git commit -m "feat(clean-room): rewrite session/storage.go from scratch"
```

### Task 3: Rewrite terminal.go — Embedded Terminal

**Files:**
- Modify: `session/terminal.go`
- Test: `session/instance_test.go` (existing, exercises terminal via Instance)

**Step 1: write the failing test**

Existing tests exercise `NewDummyTerminal`, `Close`, `Render`, `WaitForRender`, `Resize`, `SendKey` indirectly.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/terminal.go` from scratch:

- `EmbeddedTerminal` struct — ptmx, cmd, emu (vt.SafeEmulator), cancel channel, signal channels (dataReady, renderReady), render cache (cacheMu, cached, hasNew)
- `NewEmbeddedTerminal(sessionName, cols, rows)` — create emulator, spawn tmux attach with PTY, start readLoop/responseLoop/renderLoop goroutines
- `readLoop()` — read PTY, write to emulator, signal dataReady
- `responseLoop()` — read emulator responses, pipe back to PTY
- `renderLoop()` — wait on dataReady, snapshot emulator screen to cache, signal renderReady
- `drainChannel(ch)` — discard pending signals
- `SendKey(data)` — write to PTY
- `Render()` — return cached content, clear hasNew flag
- `WaitForRender(timeout)` — block until renderReady or timeout
- `Resize(cols, rows)` — resize emulator + PTY
- `NewDummyTerminal()` — minimal terminal for tests
- `Close()` — close cancel channel, close emulator, close PTY, kill process

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/terminal.go
git commit -m "feat(clean-room): rewrite session/terminal.go from scratch"
```

## Wave 2: Instance Lifecycle and Session Management

> **depends on wave 1:** instance_lifecycle.go and instance_session.go call methods on Instance, Storage, and EmbeddedTerminal that are rewritten in wave 1.

### Task 4: Rewrite instance_lifecycle.go — Start, Kill, Pause, Resume, Restart

**Files:**
- Modify: `session/instance_lifecycle.go`
- Test: `session/instance_lifecycle_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `Start`, `Kill`, `Pause`, `Resume`, `Restart`, and the loading progress flow.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestStart|TestKill|TestPause|TestResume|TestRestart" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/instance_lifecycle.go` from scratch. Key methods:

- `Start(newSession bool)` — create/restore tmux session, setup git worktree (if new), set task env, set initial prompt, configure progress reporting, handle shared worktrees for plan topics
- `Kill()` — close tmux session, cleanup git worktree (unless shared), clean up embedded terminal
- `Pause()` — close tmux session, remove worktree (keep branch), set status to Paused
- `Resume()` — setup worktree from existing branch, start tmux session, restore
- `Restart()` — kill and re-start with fresh tmux session
- Loading progress helpers, shared worktree detection

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_lifecycle.go
git commit -m "feat(clean-room): rewrite session/instance_lifecycle.go from scratch"
```

### Task 5: Rewrite instance_session.go — Attach, Detach, and Metadata

**Files:**
- Modify: `session/instance_session.go`
- Test: `session/instance_session_async_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `Attach`, `Detach`, `HasUpdated`, `TapEnter`, `SendKeys`, `UpdateDiffStats`, `GetDiffStats`, `SetDetachedSize`, embedded terminal management.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestAttach|TestDetach|TestHasUpdated|TestDiffStats" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/instance_session.go` from scratch:

- `Attach()` — delegate to tmux session Attach, return channel
- `Detach()` — delegate to tmux session DetachSafely
- `HasUpdated()` — delegate to tmux HasUpdated, update status based on updated/hasPrompt
- `HasUpdatedWithContent()` — same with content return
- `TapEnter()` — delegate to tmux
- `SendKeys(keys)` — delegate to tmux
- `SendPermissionResponse(choice)` — delegate to tmux
- `UpdateDiffStats()` — compute diff via gitWorktree.Diff()
- `GetDiffStats()` — return cached diff stats
- `SetDetachedSize(width, height)` — delegate to tmux
- `CapturePaneContent()` — delegate to tmux
- Embedded terminal management: `StartEmbeddedTerminal`, `StopEmbeddedTerminal`, `GetEmbeddedTerminal`

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_session.go
git commit -m "feat(clean-room): rewrite session/instance_session.go from scratch"
```
