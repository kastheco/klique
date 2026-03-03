# Rewrite Plumbing Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in the plumbing layer (main.go, cmd/cmd.go, config/config.go, config/state.go, keys/keys.go, log/log.go, daemon/daemon.go, app/help.go, app/app_input.go, app/app.go, app/app_actions.go) to remove AGPL-tainted lines. This is the largest plan — it covers the application entry point, configuration, key bindings, logging, daemon, help screen, and the core app model.

**Architecture:** Eleven files rewritten in-place. The app layer (app.go, app_input.go, app_actions.go) has the most upstream LOC but is also the most heavily modified — the upstream skeleton is buried under thousands of lines of original code. The rewrite approach for these files is surgical: identify and replace the upstream-derived sections while preserving the surrounding original code. For smaller files (main.go, cmd.go, config.go, state.go, keys.go, log.go, daemon.go, help.go), full file rewrites are appropriate. Existing tests across all packages serve as the regression suite.

**Tech Stack:** Go 1.24, cobra, bubbletea v1.3.x, lipgloss v1.1.x, bubbles v0.20+, testify

**Size:** Large (estimated ~5 hours, 6 tasks, 3 waves)

---

## Wave 1: Foundation — Config, Logging, Keys

### Task 1: Rewrite config/state.go and log/log.go — State Management and Logging

**Files:**
- Modify: `config/state.go`
- Modify: `log/log.go`
- Test: `config/config_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `LoadState`, `SaveState`, `DefaultState`, and the `StateManager` interface. Log initialization is tested implicitly by every test that calls `log.Initialize`.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/... -v -count=1 && go test ./log/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch:

**config/state.go:**
- Constants: `StateFileName`, `InstancesFileName`
- Interfaces: `InstanceStorage` (SaveInstances, GetInstances, DeleteAllInstances), `AppState` (GetHelpScreensSeen, SetHelpScreensSeen), `StateManager` (combines both)
- `State` struct — HelpScreensSeen uint32, InstancesData json.RawMessage
- `DefaultState()` — return default with empty instances array
- `LoadState()` — read from config dir, create default if missing, unmarshal JSON
- `SaveState(state)` — marshal JSON, write to config dir
- Interface implementations on `*State`

**log/log.go:**
- Global loggers: `WarningLog`, `InfoLog`, `ErrorLog`
- `Initialize(daemon, telemetryEnabled...)` — open log file, configure log format, create loggers with optional Sentry writers
- `Close()` — close log file, print path
- `Every` struct — rate-limited logging helper with timeout-based gating
- `NewEvery(timeout)`, `ShouldLog()` — timer-based rate limiter

**Step 4: run test to verify it passes**

```bash
go test ./config/... -v -count=1 && go test ./log/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add config/state.go log/log.go
git commit -m "feat(clean-room): rewrite config/state.go and log/log.go from scratch"
```

### Task 2: Rewrite config/config.go and keys/keys.go — Configuration and Key Bindings

**Files:**
- Modify: `config/config.go`
- Modify: `keys/keys.go`
- Test: `config/config_test.go`, `keys/keys_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `LoadConfig`, `SaveConfig`, config defaults, and key binding lookups.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/... -v -count=1 && go test ./keys/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch:

**config/config.go:**
- `Config` struct — all fields preserved (BranchPrefix, DefaultProgram, DaemonPollInterval, etc.)
- `DefaultConfig()` — return sensible defaults
- `LoadConfig()` — load from TOML + JSON, merge, return
- `SaveConfig(cfg)` — write to config dir
- `GetConfigDir()` — return `~/.kasmos/` (or `~/.klique/` for legacy)
- Config validation and migration helpers

**keys/keys.go:**
- `KeyName` enum — all key constants (KeyUp, KeyDown, KeyEnter, KeyKill, KeyAbort, etc.)
- `GlobalKeyStringsMap` — string-to-KeyName mapping
- `GlobalkeyBindings` — KeyName-to-key.Binding mapping with help text
- All key bindings preserved exactly (same keys, same help strings)

**Step 4: run test to verify it passes**

```bash
go test ./config/... -v -count=1 && go test ./keys/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add config/config.go keys/keys.go
git commit -m "feat(clean-room): rewrite config/config.go and keys/keys.go from scratch"
```

## Wave 2: Entry Points — Main, Cmd, Daemon, Help

> **depends on wave 1:** main.go uses config and log. daemon.go uses config, log, and session. cmd.go uses cobra. help.go uses keys. All depend on wave 1 rewrites being stable.

### Task 3: Rewrite main.go and cmd/cmd.go — Application Entry Point

**Files:**
- Modify: `main.go`
- Modify: `cmd/cmd.go`
- Test: `cmd/plan_test.go`, `cmd/serve_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `NewRootCmd`, `NewPlanCmd`, `NewServeCmd`, and the `Executor` interface.

**Step 2: run test to verify baseline passes**

```bash
go test ./cmd/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch:

**main.go:**
- `main()` — parse flags (--daemon, --version), initialize logging, load config, initialize Sentry, set up cobra root command, register subcommands, handle daemon mode, launch TUI
- Flag handling, version display, signal handling

**cmd/cmd.go:**
- `Executor` interface — `Run(cmd)`, `Output(cmd)`
- `Exec` struct implementing Executor
- `MakeExecutor()` — constructor
- `ToString(cmd)` — format command as string
- `NewRootCmd()` — create root cobra command with subcommands

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add main.go cmd/cmd.go
git commit -m "feat(clean-room): rewrite main.go and cmd/cmd.go from scratch"
```

### Task 4: Rewrite daemon/daemon.go and app/help.go — Daemon and Help Screen

**Files:**
- Modify: `daemon/daemon.go`
- Modify: `app/help.go`
- Test: `app/app_test.go` (existing, exercises help rendering)

**Step 1: write the failing test**

Existing tests exercise help screen rendering through the app test harness. Daemon is tested via integration.

**Step 2: run test to verify baseline passes**

```bash
go test ./app/... -run "TestHelp" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch:

**daemon/daemon.go:**
- `RunDaemon(cfg)` — load state, load instances, set AutoYes, poll loop (check HasUpdated, TapEnter, UpdateDiffStats), signal handling (SIGINT/SIGTERM), save instances on exit
- `LaunchDaemon()` — find executable, spawn detached child with --daemon flag, write PID file
- `StopDaemon()` — read PID file, kill process, remove PID file

**app/help.go:**
- `HelpScreen` — render multi-page help with key binding reference
- Help pages: navigation, instance management, plan lifecycle, shortcuts
- `HelpScreensSeen` bitmask tracking
- `View()` — render current help page with navigation hints

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run "TestHelp" -v -count=1 && go test ./daemon/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add daemon/daemon.go app/help.go
git commit -m "feat(clean-room): rewrite daemon/daemon.go and app/help.go from scratch"
```

## Wave 3: App Core — The Big Three

> **depends on wave 2:** The app model files depend on all lower layers (config, keys, log, cmd, daemon, session, ui) being stable.

### Task 5: Rewrite app/app_input.go — Input Handling

**Files:**
- Modify: `app/app_input.go`
- Test: `app/app_input_keybytes_test.go`, `app/app_input_right_on_instance_test.go`, `app/app_input_viewport_test.go`, `app/app_input_yes_keybind_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover key dispatch, viewport scrolling, yes keybind, and instance selection input.

**Step 2: run test to verify baseline passes**

```bash
go test ./app/... -run "TestInput|TestKey" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `app/app_input.go` from scratch. This is the largest upstream-derived file (~660 LOC upstream in 1533 LOC total). The rewrite replaces the key dispatch table and input routing logic:

- `handleKeyMsg(msg)` — main key dispatch: route to overlay handler if active, otherwise to global key handler
- `handleGlobalKey(msg)` — dispatch based on KeyName: navigation (up/down/left/right), instance actions (enter, kill, abort, checkout, resume, push), plan actions (new plan, search), view actions (tab cycle, sidebar toggle, help), agent actions (send prompt, send yes, spawn agent)
- `handleOverlayKey(msg)` — delegate to active overlay's Update method
- `handleSearchInput(msg)` — filter instances by search query
- `handleInteractiveMode(msg)` — forward keys to attached tmux session
- Scroll handling for viewport panels

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_input.go
git commit -m "feat(clean-room): rewrite app/app_input.go from scratch"
```

### Task 6: Rewrite app/app.go and app/app_actions.go — App Model and Actions

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_actions.go`
- Test: `app/app_test.go` and all app test files (existing)

**Step 1: write the failing test**

Existing tests cover the full app model: Init, Update, View, instance creation, plan actions, wave orchestration.

**Step 2: run test to verify baseline passes**

```bash
go test ./app/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch. These have the least upstream LOC relative to total size (~173 LOC upstream in 2037 LOC app.go, ~63 LOC upstream in 910 LOC app_actions.go), so the rewrite is mostly about replacing the skeletal upstream structure:

**app/app.go:**
- `Model` struct — all fields preserved (instances, storage, config, UI components, overlay state, etc.)
- `New(cfg, storage, state)` — constructor
- `Init()` — initialize UI components, load instances, start tick
- `Update(msg)` — main message dispatch: key messages, window size, tick messages, instance metadata, plan state changes
- `View()` — compose layout: sidebar + center pane (tabbed window) + bottom menu, overlay on top if active
- Tick management, instance metadata polling, plan state polling

**app/app_actions.go:**
- Instance actions: `createInstance`, `killInstance`, `pauseInstance`, `resumeInstance`, `restartInstance`, `attachInstance`, `detachInstance`
- Plan actions: `createPlan`, `implementPlan`, `cancelPlan`
- Push/PR actions: `pushChanges`, `createPR`
- Toast helpers, confirmation dialog helpers

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_actions.go
git commit -m "feat(clean-room): rewrite app/app.go and app/app_actions.go from scratch"
```
