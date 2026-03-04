# Rewrite Plumbing Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in `config/config.go`, `config/state.go`, `cmd/cmd.go`, and `daemon/` to remove AGPL-tainted lines from the fork point (`bbc8cad`), enabling a license change. The rewrite preserves identical behavior and public API while replacing every line that traces back to the upstream fork.

**Architecture:** Six files are rewritten in-place across three packages: `config/` (config.go, state.go), `cmd/` (cmd.go), and `daemon/` (daemon.go, daemon_unix.go, daemon_windows.go). Each file is deleted and rewritten from its functional specification — no copy-paste from the original. All exported types, functions, and method signatures stay identical so callers compile without changes. Files that are 100% post-fork original (`config/toml.go`, `config/profile.go`, `config/permission_store.go`, `config/permission_migrate.go`, `config/taskfsm/`, `config/taskparser/`, `config/taskstate/`, `config/taskstore/`, `config/auditlog/`, `cmd/task.go`, `cmd/serve.go`) are untouched. Existing tests in `config/config_test.go` and `cmd/cmd_test/` serve as the regression suite.

**Tech Stack:** Go 1.24, encoding/json, os/exec, os/user, os/signal, syscall, cobra, testify

**Size:** Small (estimated ~1.5 hours, 3 tasks, 1 wave)

---

## Wave 1: Plumbing Rewrite

All three tasks are independent — each rewrites files in a different package. No task depends on another task's output.

### Task 1: Rewrite config/config.go and config/state.go

**Files:**
- Modify: `config/config.go`
- Modify: `config/state.go`
- Test: `config/config_test.go` (existing, no changes needed)

**Step 1: write the failing test**

No new tests needed. The existing `config/config_test.go` (335 LOC) covers `GetConfigDir` (including legacy migration from `.hivemind` and `.klique`), `GetDefaultCommand` (opencode preference, claude fallback, empty SHELL, alias parsing), `DefaultConfig`, `LoadConfig` (missing file, valid file, invalid JSON), `SaveConfig`, and `IsTelemetryEnabled`. These tests ARE the regression gate.

**Step 2: run test to verify it passes (baseline)**

```bash
go test ./config/... -run 'TestGetDefaultCommand|TestDefaultConfig|TestGetConfigDir|TestLoadConfig|TestSaveConfig|TestIsTelemetryEnabled' -v -count=1
```

expected: PASS — all existing tests pass before we touch anything

**Step 3: rewrite implementation**

Delete `config/config.go` and rewrite from scratch based on this functional spec:

**Constants:**
- `ConfigFileName = "config.json"`
- `defaultProgram = "opencode"` (unexported)
- `aliasRegex` — compiled regex `(?:aliased to|->|=)\s*([^\s]+)` for parsing shell alias output

**`GetConfigDir() (string, error)`** — returns `~/.config/kasmos/`. On first call: if the directory already exists, return it (fast path). Otherwise try migrating legacy directories in order: `~/.klique`, `~/.hivemind`. Migration: ensure `~/.config/` parent exists via `os.MkdirAll`, then `os.Rename` old → new. On any migration error, log and return the old directory. If no legacy dir exists, return the new path (caller creates it on first write).

**`Config` struct** — JSON-tagged fields: `DefaultProgram string`, `AutoYes bool`, `DaemonPollInterval int`, `BranchPrefix string`, `NotificationsEnabled *bool` (omitempty), `Profiles map[string]AgentProfile` (omitempty), `PhaseRoles map[string]string` (omitempty), `AnimateBanner bool` (omitempty), `AutoAdvanceWaves bool` (omitempty), `TelemetryEnabled *bool` (omitempty), `DatabaseURL string` (omitempty).

**`DefaultConfig() *Config`** — calls `GetDefaultCommand()` for program (falls back to `"opencode"` on error), sets `AutoYes: false`, `DaemonPollInterval: 1000`, `BranchPrefix: "<username>/"` from `user.Current()` (falls back to `"session/"` on error), `NotificationsEnabled: &true`.

**`AreNotificationsEnabled() bool`** — returns true when `NotificationsEnabled` is nil, otherwise dereferences.

**`IsTelemetryEnabled() bool`** — returns true when `TelemetryEnabled` is nil, otherwise dereferences.

**`GetDefaultCommand() (string, error)`** — tries `findCommand("opencode")` then `findCommand("claude")`. Returns error if neither found.

**`findCommand(name string) (string, error)`** — reads `$SHELL` (defaults to `/bin/bash`). For zsh: `source ~/.zshrc &>/dev/null || true; which <name>`. For bash: `source ~/.bashrc &>/dev/null || true; which <name>`. Otherwise: `which <name>`. Runs via `exec.Command(shell, "-c", shellCmd)`. On success, parses output via `parseCommandOutput`. Falls back to `exec.LookPath(name)`.

**`parseCommandOutput(output string) string`** — trims whitespace, returns empty on blank. Checks `aliasRegex` for alias resolution, otherwise returns the trimmed path.

**`LoadConfig() *Config`** — gets config dir, reads `config.json`. On not-exist: creates and saves default. On parse error: returns default. After JSON parse, overlays TOML config (`LoadTOMLConfig()`): copies `Profiles`, `PhaseRoles`, `AnimateBanner`, `AutoAdvanceWaves`, `TelemetryEnabled`, `DatabaseURL` when present in TOML.

**`saveConfig(config *Config) error`** — ensures config dir exists, marshals to indented JSON, writes file.

**`SaveConfig(config *Config) error`** — exported wrapper for `saveConfig`.

Imports: `encoding/json`, `fmt`, `os`, `os/exec`, `os/user`, `path/filepath`, `regexp`, `strings`, `github.com/kastheco/kasmos/log`.

---

Delete `config/state.go` and rewrite from scratch based on this functional spec:

**Constants:**
- `StateFileName = "state.json"`
- `InstancesFileName = "instances.json"`

**Interfaces:**
- `InstanceStorage` — `SaveInstances(json.RawMessage) error`, `GetInstances() json.RawMessage`, `DeleteAllInstances() error`
- `AppState` — `GetHelpScreensSeen() uint32`, `SetHelpScreensSeen(uint32) error`
- `StateManager` — embeds `InstanceStorage` + `AppState`

**`State` struct** — JSON-tagged: `HelpScreensSeen uint32`, `InstancesData json.RawMessage` (tag: `"instances"`).

**`DefaultState() *State`** — returns `HelpScreensSeen: 0`, `InstancesData: json.RawMessage("[]")`.

**`LoadState() *State`** — gets config dir, reads `state.json`. On not-exist: saves and returns default. On parse error: returns default.

**`SaveState(state *State) error`** — ensures config dir, marshals indented JSON, writes file.

**`State` methods implementing interfaces:**
- `SaveInstances` — sets `InstancesData`, calls `SaveState`
- `GetInstances` — returns `InstancesData`
- `DeleteAllInstances` — sets `InstancesData` to `"[]"`, calls `SaveState`
- `GetHelpScreensSeen` — returns field
- `SetHelpScreensSeen` — sets field, calls `SaveState`

Imports: `encoding/json`, `fmt`, `os`, `path/filepath`, `github.com/kastheco/kasmos/log`.

**Step 4: run test to verify it passes**

```bash
go test ./config/... -run 'TestGetDefaultCommand|TestDefaultConfig|TestGetConfigDir|TestLoadConfig|TestSaveConfig|TestIsTelemetryEnabled' -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add config/config.go config/state.go
git commit -m "feat(clean-room): rewrite config/config.go and config/state.go from scratch"
```

### Task 2: Rewrite cmd/cmd.go

**Files:**
- Modify: `cmd/cmd.go`
- Test: `cmd/cmd_test/testutils.go` (existing, no changes needed)

**Step 1: write the failing test**

No new tests needed. The `cmd/cmd.go` file defines the `Executor` interface, `Exec` struct, `MakeExecutor`, `ToString`, and `NewRootCmd` — all exercised by the existing test infrastructure in `cmd/cmd_test/testutils.go` and indirectly by `cmd/task_test.go` and `cmd/serve_test.go`. The `NewRootCmd` function is also tested by the cobra command tree tests.

**Step 2: run test to verify it passes (baseline)**

```bash
go test ./cmd/... -v -count=1
```

expected: PASS — all existing tests pass before we touch anything

**Step 3: rewrite implementation**

Delete `cmd/cmd.go` and rewrite from scratch based on this functional spec:

**`Executor` interface** — `Run(cmd *exec.Cmd) error`, `Output(cmd *exec.Cmd) ([]byte, error)`.

**`Exec` struct** — empty struct implementing `Executor`. `Run` delegates to `cmd.Run()`. `Output` delegates to `cmd.Output()`.

**`MakeExecutor() Executor`** — returns `Exec{}`.

**`ToString(cmd *exec.Cmd) string`** — returns `"<nil>"` for nil cmd, otherwise `strings.Join(cmd.Args, " ")`.

**`NewRootCmd() *cobra.Command`** — creates root command with `Use: "kas"`, `Short: "kas - Manage multiple AI agents"`. Adds `NewTaskCmd()` and `NewServeCmd()` as subcommands.

Imports: `os/exec`, `strings`, `github.com/spf13/cobra`.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/cmd.go
git commit -m "feat(clean-room): rewrite cmd/cmd.go from scratch"
```

### Task 3: Rewrite daemon/ package

**Files:**
- Modify: `daemon/daemon.go`
- Modify: `daemon/daemon_unix.go`
- Modify: `daemon/daemon_windows.go`
- Test: (no dedicated test file — daemon is integration-tested via the main binary)

**Step 1: write the failing test**

No new tests needed. The daemon package has no unit tests (it requires real tmux sessions and process management). The rewrite is verified by: (a) compilation, (b) the existing `go vet ./daemon/...` passing, and (c) manual smoke test of `kas --daemon` if desired. The daemon is a thin orchestration layer over `config.LoadState`, `session.NewStorage`, and `session.LoadInstances` — all of which have their own test suites.

**Step 2: run test to verify baseline compiles**

```bash
go build ./daemon/...
go vet ./daemon/...
```

expected: no errors

**Step 3: rewrite implementation**

Delete `daemon/daemon.go` and rewrite from scratch based on this functional spec:

**`RunDaemon(cfg *config.Config) error`** — loads state via `config.LoadState()`, creates storage via `session.NewStorage(state)`, loads instances via `storage.LoadInstances()`. Sets `AutoYes = true` on all instances. Creates a polling goroutine with `time.NewTimer(pollInterval)` where `pollInterval = time.Duration(cfg.DaemonPollInterval) * time.Millisecond`. Each tick: for each started, non-paused instance, calls `HasUpdated()` — if `hasPrompt` is true, calls `TapEnter()` and `UpdateDiffStats()` (logging errors via `log.NewEvery(60s)`). Listens for `SIGINT`/`SIGTERM` via `signal.Notify`, stops the goroutine via a `stopCh` channel, waits on a `sync.WaitGroup`, saves instances, returns nil.

**`LaunchDaemon() error`** — finds own executable via `os.Executable()`, creates `exec.Command(execPath, "--daemon")` with nil stdin/stdout/stderr and platform-specific `SysProcAttr` from `getSysProcAttr()`. Starts the process, writes PID to `<configDir>/daemon.pid`.

**`StopDaemon() error`** — reads PID from `<configDir>/daemon.pid`. If file doesn't exist, returns nil. Finds process via `os.FindProcess(pid)`, kills it, removes PID file.

Imports: `fmt`, `os`, `os/exec`, `os/signal`, `path/filepath`, `sync`, `syscall`, `time`, `github.com/kastheco/kasmos/config`, `github.com/kastheco/kasmos/log`, `github.com/kastheco/kasmos/session`.

---

Delete `daemon/daemon_unix.go` and rewrite from scratch:

Build tag: `//go:build !windows`

**`getSysProcAttr() *syscall.SysProcAttr`** — returns `&syscall.SysProcAttr{Setsid: true}` to create a new session for process detachment.

Imports: `syscall`.

---

Delete `daemon/daemon_windows.go` and rewrite from scratch:

Build tag: `//go:build windows`

**`getSysProcAttr() *syscall.SysProcAttr`** — returns `&syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}`.

Imports: `golang.org/x/sys/windows`, `syscall`.

**Step 4: run test to verify it compiles**

```bash
go build ./daemon/...
go vet ./daemon/...
go build ./...
```

expected: no errors

**Step 5: commit**

```bash
git add daemon/daemon.go daemon/daemon_unix.go daemon/daemon_windows.go
git commit -m "feat(clean-room): rewrite daemon/ package from scratch"
```
