# Rewrite Session Core Implementation Plan

**Goal:** Clean-room rewrite all files in `session/` (excluding the already-rewritten `session/tmux/` and `session/git/` subdirectories) to remove AGPL-tainted lines. The rewrite preserves the identical public API so all callers (`app/`, `ui/`, `daemon/`, `main.go`) compile without changes, while replacing every line of implementation.

**Architecture:** Eight files rewritten in-place across the session core: `instance.go` (struct, constructors, accessors, serialization), `instance_lifecycle.go` (Start variants, Kill, Pause, Resume, Restart, Adopt), `instance_session.go` (pane I/O, metadata collection, resource monitoring, diff stats), `storage.go` (JSON persistence via `config.StateManager`), `activity.go` (agent activity parsing from pane content), `permission_prompt.go` (opencode permission dialog detection), `cli_prompt.go` (program CLI prompt support detection), and `notify.go` (desktop notifications). The `terminal.go` file (EmbeddedTerminal) is 100% original code (written post-fork for the live-preview feature) and is untouched. All existing test files are preserved as the regression suite — they must pass after each task. The public API (exported types, method signatures, constants) stays identical.

**Tech Stack:** Go 1.24, testify, `session/tmux` (rewritten), `session/git` (rewritten), `config.StateManager`, `charmbracelet/x/ansi`, `charmbracelet/x/vt`, `creack/pty`, `atotto/clipboard`

**Size:** Large (estimated ~6 hours, 8 tasks, 3 waves)

---

## Wave 1: Foundation Types and Pure Functions

Rewrites the files with zero internal dependencies on other session core files. These are leaf nodes in the dependency graph — they import only stdlib and external packages, never other `session/` files. All four tasks are independently implementable.

### Task 1: Rewrite activity.go — Agent Activity Parsing

**Files:**
- Modify: `session/activity.go`
- Test: `session/activity_test.go` (existing — 230 LOC, 11 test functions)

**Step 1: write the failing test**

No new tests needed. Existing `activity_test.go` covers: Claude editing/writing/reading/running/searching/shell-command patterns, Aider editing, no-match case, 30-line scan window, bottom-up scanning priority, ANSI stripping, detail truncation, and `cleanFilename`/`truncateDetail` helpers. These tests ARE the regression gate.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestParseActivity|TestTruncateDetail|TestCleanFilename" -v -count=1
```

expected: PASS — all existing tests green before rewrite

**Step 3: rewrite implementation**

Delete the contents of `session/activity.go` and rewrite from scratch based on this functional spec:

- **`Activity` struct** — three fields: `Action string` (e.g. "editing", "running", "reading", "searching"), `Detail string` (e.g. filename or command), `Timestamp time.Time`.
- **`ansiRegex`** — compiled `regexp.Regexp` matching ANSI escape sequences `\x1b\[[0-9;]*[a-zA-Z]`.
- **Agent-specific regexes** — compiled patterns for Claude (editing/writing, reading, running, searching, shell `$` prefix) and Aider (editing). Each regex captures the detail portion.
- **`ParseActivity(content string, program string) *Activity`** — strips ANSI codes from content, splits into lines, takes last 30 lines, scans bottom-up. For each non-empty trimmed line: dispatches to agent-specific parser based on program name (case-insensitive substring match for "claude" or "aider"), then falls back to generic parser. Returns first match or nil.
- **`parseClaudeLine(line string) *Activity`** — matches against Claude regexes in order: editing/writing → reading → running → searching → shell command. Returns `Activity` with appropriate action, cleaned/truncated detail, and `time.Now()` timestamp.
- **`parseAiderLine(line string) *Activity`** — matches Aider editing regex. Returns `Activity` or nil.
- **`parseGenericLine(line string) *Activity`** — matches shell command pattern (`$ <cmd>`). Returns `Activity` or nil.
- **`cleanFilename(s string) string`** — trims whitespace, returns `filepath.Base(s)` if path contains `/`, otherwise returns trimmed string.
- **`truncateDetail(s string, maxLen int) string`** — returns `s` if `len(s) <= maxLen`. If `maxLen <= 3`, returns `s[:maxLen]`. Otherwise returns `s[:maxLen-3] + "..."`.

Imports: `path/filepath`, `regexp`, `strings`, `time`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestParseActivity|TestTruncateDetail|TestCleanFilename" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/activity.go
git commit -m "feat(clean-room): rewrite session/activity.go from scratch"
```

### Task 2: Rewrite permission_prompt.go — Permission Dialog Detection

**Files:**
- Modify: `session/permission_prompt.go`
- Test: `session/permission_prompt_test.go` (existing — 70 LOC, 6 test functions)

**Step 1: write the failing test**

No new tests needed. Existing tests cover: opencode prompt detection with description + pattern extraction, no-prompt case, non-opencode program rejection, ANSI code handling, missing pattern graceful handling, and conversation text false-positive rejection (the structural two-check approach: `△ Permission required` header + button bar).

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestParsePermissionPrompt" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `session/permission_prompt.go` and rewrite from scratch:

- **`PermissionPrompt` struct** — two fields: `Description string` (human-readable, e.g. "Access external directory /opt"), `Pattern string` (permission pattern, e.g. "/opt/*").
- **`ParsePermissionPrompt(content string, program string) *PermissionPrompt`** — returns nil immediately if program doesn't contain "opencode" (case-insensitive). Strips ANSI codes via `ansi.Strip()`. Splits into lines. Takes only the last 25 lines (permission dialog renders at the bottom of opencode's TUI). Performs two structural checks to avoid false-positives:
  1. Finds `△` + `Permission required` on the same line (records index as `permIdx`). Returns nil if not found.
  2. Scans from `permIdx` forward for a line containing both `Allow once` AND `Allow always`. Returns nil if not found.
- **Description extraction:** Starting from `permIdx + 1`, finds first non-empty line. Strips leading arrow prefixes (`← `, `←`, `→ `, `→`) and trims whitespace. Sets as `Description`.
- **Pattern extraction:** From `permIdx` forward, finds `Patterns` header line. From the next line, finds first non-empty line starting with `- `. Strips the `- ` prefix and sets as `Pattern`. If no pattern found, `Pattern` stays empty.

Imports: `strings`, `github.com/charmbracelet/x/ansi`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestParsePermissionPrompt" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/permission_prompt.go
git commit -m "feat(clean-room): rewrite session/permission_prompt.go from scratch"
```

### Task 3: Rewrite cli_prompt.go and notify.go — Small Utilities

**Files:**
- Modify: `session/cli_prompt.go`
- Modify: `session/notify.go`
- Test: `session/cli_prompt_test.go` (existing — 27 LOC, 1 table-driven test)

**Step 1: write the failing test**

No new tests needed. Existing `cli_prompt_test.go` covers `programSupportsCliPrompt` with 6 cases (opencode, claude, full paths, aider, gemini, empty string). `notify.go` has no tests — it's fire-and-forget OS notification.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestProgramSupportsCliPrompt" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

**cli_prompt.go** — Delete contents and rewrite:

- **`programSupportsCliPrompt(program string) bool`** — returns true if `program` ends with `"opencode"` or `"claude"` (checked via `strings.HasSuffix`). These programs accept an initial prompt via CLI flag or positional arg. Returns false for all other programs (aider, gemini, etc.).

Imports: `strings`.

**notify.go** — Delete contents and rewrite:

- **`NotificationsEnabled`** — package-level `bool` variable, defaults to `true`. Set from config at startup.
- **`escapeAppleScript(s string) string`** — escapes backslashes then double quotes for AppleScript string literals.
- **`SendNotification(title, body string)`** — returns immediately if `NotificationsEnabled` is false. On `darwin`: fires `osascript -e 'display notification "..." with title "..."'` via `exec.Command` + `Start()` (fire-and-forget). On `linux`: checks for `notify-send` via `exec.LookPath`, fires `notify-send <title> <body>` via `Start()`. No-op on other platforms.

Imports: `os/exec`, `runtime`, `strings`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestProgramSupportsCliPrompt" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/cli_prompt.go session/notify.go
git commit -m "feat(clean-room): rewrite session/cli_prompt.go and session/notify.go from scratch"
```

### Task 4: Rewrite storage.go — Instance Persistence

**Files:**
- Modify: `session/storage.go`
- Test: `session/instance_taskfile_test.go` (existing — exercises `ToInstanceData`/`FromInstanceData` round-trips)

**Step 1: write the failing test**

No new tests needed. Existing tests exercise `InstanceData` JSON marshaling/unmarshaling (including the `plan_file` → `task_file` migration), `ToInstanceData`/`FromInstanceData` round-trips for TaskFile, AgentType, ImplementationComplete, SoloAgent, and wave fields. The `Storage` type's `SaveInstances`/`LoadInstances`/`DeleteInstance`/`UpdateInstance`/`DeleteAllInstances` methods are integration-tested through the app layer.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestInstanceData|TestNewInstance_Sets" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `session/storage.go` and rewrite from scratch:

- **`InstanceData` struct** — JSON-serializable mirror of `Instance` fields: Title, Path, Branch, Status, Height, Width, CreatedAt, UpdatedAt, AutoYes, SkipPermissions, TaskFile, AgentType, TaskNumber, WaveNumber, PeerCount, IsReviewer, ImplementationComplete, SoloAgent, QueuedPrompt, ReviewCycle, Program, Worktree (`GitWorktreeData`), DiffStats (`DiffStatsData`). JSON tags must match exactly (including `omitempty` on optional fields).
- **`InstanceData.UnmarshalJSON(data []byte) error`** — custom unmarshaler for backward compatibility: defines an alias type to avoid recursion, adds a `PlanFile string` field with `json:"plan_file,omitempty"` tag. If `TaskFile` is empty after unmarshal but `PlanFile` is set, copies `PlanFile` to `TaskFile`.
- **`GitWorktreeData` struct** — five string fields: RepoPath, WorktreePath, SessionName, BranchName, BaseCommitSHA. All with JSON tags.
- **`DiffStatsData` struct** — three fields: Added (int), Removed (int), Content (string). All with JSON tags.
- **`Storage` struct** — holds a `config.StateManager` reference.
- **`NewStorage(state config.StateManager) (*Storage, error)`** — constructor, returns `&Storage{state: state}, nil`.
- **`SaveInstances(instances []*Instance) error`** — iterates instances, calls `ToInstanceData()` on each started instance, marshals the slice to JSON, calls `state.SaveInstances()`.
- **`LoadInstances() ([]*Instance, error)`** — calls `state.GetInstances()`, unmarshals JSON into `[]InstanceData`. For each entry: skips non-paused instances whose worktree path no longer exists on disk (logs warning). Calls `FromInstanceData()` on each — logs and skips entries that fail to restore (instead of hard-failing all instances).
- **`DeleteInstance(title string) error`** — loads all instances, filters out the one matching `title`, saves the rest. Returns error if not found.
- **`UpdateInstance(instance *Instance) error`** — loads all instances, replaces the one matching by title, saves. Returns error if not found.
- **`DeleteAllInstances() error`** — delegates to `state.DeleteAllInstances()`.

Imports: `encoding/json`, `fmt`, `os`, `time`, `github.com/kastheco/kasmos/config`, `github.com/kastheco/kasmos/log`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestInstanceData|TestNewInstance_Sets" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/storage.go
git commit -m "feat(clean-room): rewrite session/storage.go from scratch"
```

## Wave 2: Instance Core

> **depends on wave 1:** `instance.go` imports `storage.go` types (`InstanceData`, `GitWorktreeData`, `DiffStatsData`) for serialization, and `activity.go`'s `Activity` type is referenced in the `Instance` struct. Wave 1 must be complete so these types exist with their exact signatures.

### Task 5: Rewrite instance.go — Struct, Constructors, Accessors, and Serialization

**Files:**
- Modify: `session/instance.go`
- Test: `session/instance_test.go` (existing), `session/instance_taskfile_test.go` (existing), `session/instance_wave_test.go` (existing), `session/instance_title_test.go` (existing)

**Step 1: write the failing test**

No new tests needed. Existing tests cover: `NewInstance` defaults (SoloAgent=false), `NewInstance` field propagation (TaskFile, AgentType, WaveNumber, TaskNumber), `ToInstanceData`/`FromInstanceData` round-trips (TaskFile, AgentType, ImplementationComplete, SoloAgent, wave fields), `buildTitleOpts` mapping for all agent types, and `FromInstanceData` paused-instance restoration.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestNewInstance|TestInstanceData|TestBuildTitleOpts" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `session/instance.go` and rewrite from scratch:

- **`Status` type** — `type Status int` with four constants: `Running` (0), `Ready` (1), `Loading` (2), `Paused` (3). Must use `iota` starting from `Running`.
- **Agent type constants** — `AgentTypePlanner = "planner"`, `AgentTypeCoder = "coder"`, `AgentTypeReviewer = "reviewer"`, `AgentTypeFixer = "fixer"`.
- **`Instance` struct** — all fields must match the current struct exactly (same names, types, export status, and ordering). The struct has ~40 fields organized in groups:
  - Identity: Title, Path, Branch, Status, Program, Height, Width, CreatedAt, UpdatedAt
  - Config: AutoYes, SkipPermissions
  - Plan/task metadata: TaskFile, Topic, AgentType, TaskNumber, WaveNumber, PeerCount, IsReviewer (deprecated), ImplementationComplete, SoloAgent, Exited, QueuedPrompt
  - Orchestration state: sharedWorktree (unexported), LoadingStage, LoadingTotal, LoadingMessage, Notified, LastActiveAt, PromptDetected, AwaitingWork, ReviewCycle, HasWorked
  - Resource monitoring: CPUPercent, MemMB, LastActivity (*Activity)
  - Cache: CachedContent, CachedContentSet
  - Internal: diffStats (*git.DiffStats), started (bool), tmuxSession (*tmux.TmuxSession), gitWorktree (*git.GitWorktree)
- **`ToInstanceData() InstanceData`** — converts Instance to its serializable form. Copies all persisted fields. Sets `UpdatedAt` to `time.Now()`. Conditionally includes worktree data (if `gitWorktree != nil`) and diff stats (if `diffStats != nil`).
- **`FromInstanceData(data InstanceData) (*Instance, error)`** — creates Instance from serialized data. Copies all fields. Creates `gitWorktree` via `git.NewGitWorktreeFromStorage()`. Creates `diffStats` from `DiffStatsData`. For paused instances: sets `started = true`, creates `tmux.NewTmuxSession()`. For non-paused: creates `TmuxSession`, sets agent type, checks `DoesSessionExist()` — if dead, marks `started = true`, `Exited = true`, `Status = Ready`; if alive, calls `Start(false)` to restore.
- **`InstanceOptions` struct** — Title, Path, Program, AutoYes, SkipPermissions, TaskFile, AgentType, TaskNumber, WaveNumber, PeerCount, ReviewCycle.
- **`NewInstance(opts InstanceOptions) (*Instance, error)`** — converts path to absolute via `filepath.Abs`, initializes Instance with defaults (Status=Ready, CreatedAt/UpdatedAt=now).
- **Accessors:** `RepoName() (string, error)`, `GetRepoPath() string`, `GetWorktreePath() string`, `SetStatus(status Status)` (handles notification logic, LastActiveAt, PromptDetected, AwaitingWork transitions), `setLoadingProgress(stage int, message string)`, `Started() bool`, `SetTitle(title string) error` (errors if started), `Paused() bool`, `TmuxAlive() bool`.

Imports: `fmt`, `path/filepath`, `time`, `github.com/kastheco/kasmos/session/git`, `github.com/kastheco/kasmos/session/tmux`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestNewInstance|TestInstanceData|TestBuildTitleOpts" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance.go
git commit -m "feat(clean-room): rewrite session/instance.go from scratch"
```

### Task 6: Rewrite instance_session.go — Pane I/O, Metadata Collection, and Resource Monitoring

**Files:**
- Modify: `session/instance_session.go`
- Test: `session/instance_session_async_test.go` (existing — 59 LOC), `session/instance_lifecycle_test.go` (existing — uses `SetTmuxSession`, `MarkStartedForTest`)

**Step 1: write the failing test**

No new tests needed. Existing `TestCollectMetadata_DoesNotMutateCachedPreviewState` verifies that `CollectMetadata()` returns fresh content in the metadata struct without mutating the instance's `CachedContent`/`CachedContentSet` fields. Lifecycle tests exercise `SetTmuxSession` and `MarkStartedForTest` helpers.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestCollectMetadata|TestStart" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `session/instance_session.go` and rewrite from scratch:

- **`Preview() (string, error)`** — returns empty string if not started or paused. Delegates to `tmuxSession.CapturePaneContent()`.
- **`HasUpdated() (updated bool, hasPrompt bool)`** — returns `(false, false)` if not started. Delegates to `tmuxSession.HasUpdated()`.
- **`NewEmbeddedTerminalForInstance(cols, rows int) (*EmbeddedTerminal, error)`** — errors if not started or tmuxSession nil. Gets sanitized session name, delegates to `NewEmbeddedTerminal()`.
- **`TapEnter()`** — no-op if not started or AutoYes disabled. Delegates to `tmuxSession.TapEnter()`, logs errors.
- **`Attach() (chan struct{}, error)`** — errors if not started. Delegates to `tmuxSession.Attach()`.
- **`SetPreviewSize(width, height int) error`** — errors if not started or paused. Delegates to `tmuxSession.SetDetachedSize()`.
- **`GetGitWorktree() (*git.GitWorktree, error)`** — errors if not started. Returns `gitWorktree`.
- **`SendPrompt(prompt string) error`** — errors if not started or tmuxSession nil. Sends keys via `tmuxSession.SendKeys()`, sleeps 100ms, then sends enter via `tmuxSession.TapEnter()`.
- **`PreviewFullHistory() (string, error)`** — returns empty string if not started or paused. Delegates to `tmuxSession.CapturePaneContentWithOptions("-", "-")`.
- **`SetTmuxSession(session *tmux.TmuxSession)`** — sets `tmuxSession` field. Test helper.
- **`MarkStartedForTest()`** — sets `started = true`. Test helper.
- **`SendKeys(keys string) error`** — errors if not started or paused. Delegates to `tmuxSession.SendKeys()`.
- **`InstanceMetadata` struct** — Content (string), ContentCaptured (bool), Updated (bool), HasPrompt (bool), DiffStats (*git.DiffStats), CPUPercent (float64), MemMB (float64), ResourceUsageValid (bool), TmuxAlive (bool), PermissionPrompt (*PermissionPrompt).
- **`CollectMetadata() InstanceMetadata`** — returns empty metadata if not started or paused. Single capture-pane call via `tmuxSession.HasUpdatedWithContent()`. Git diff stats via `gitWorktree.Diff()` (logs non-trivial errors, returns nil stats on error). Permission prompt detection via `ParsePermissionPrompt()` if content was captured. Resource usage via `collectResourceUsage()`. Session liveness via `TmuxAlive()`.
- **`SetDiffStats(stats *git.DiffStats)`** — sets `diffStats` field.
- **`UpdateDiffStats() error`** — returns nil (clears stats) if not started. Keeps previous stats if paused. Calls `gitWorktree.Diff()`, handles "base commit SHA not set" and "worktree path gone" errors silently.
- **`collectResourceUsage() (float64, float64, bool)`** — returns `(0, 0, false)` if not started or tmuxSession nil. Gets pane PID via `tmuxSession.GetPanePID()`. Finds child process via `pgrep -P <pid>`. Queries CPU% and RSS via `ps -o %cpu=,rss= -p <targetPid>`. Parses fields, converts RSS from KB to MB.
- **`UpdateResourceUsage()`** — delegates to `collectResourceUsage()`, sets `CPUPercent` and `MemMB` on success.
- **`GetDiffStats() *git.DiffStats`** — returns `diffStats`.
- **`SendPermissionResponse(choice tmux.PermissionChoice)`** — no-op if not started or tmuxSession nil. Delegates to `tmuxSession.SendPermissionResponse()`, logs errors.

Imports: `fmt`, `os/exec`, `strconv`, `strings`, `time`, `github.com/kastheco/kasmos/log`, `github.com/kastheco/kasmos/session/git`, `github.com/kastheco/kasmos/session/tmux`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestCollectMetadata|TestStart" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_session.go
git commit -m "feat(clean-room): rewrite session/instance_session.go from scratch"
```

## Wave 3: Instance Lifecycle

> **depends on wave 2:** `instance_lifecycle.go` calls methods on the `Instance` struct (`SetStatus`, `setLoadingProgress`, `transferPromptToCli`, `setTmuxTaskEnv`, `configureSessionTitle`, `Kill`) and accesses fields defined in `instance.go`. It also uses `SetTmuxSession` and test helpers from `instance_session.go`. Wave 2 must be complete so the Instance struct and its methods exist.

### Task 7: Rewrite instance_lifecycle.go — Start Variants, Kill, Pause, Resume, Restart, Adopt

**Files:**
- Modify: `session/instance_lifecycle.go`
- Test: `session/instance_lifecycle_test.go` (existing — 174 LOC, 7 test functions)

**Step 1: write the failing test**

No new tests needed. Existing tests cover: `Start` transfers QueuedPrompt for opencode, `Start` keeps QueuedPrompt for aider, `Restart` kills tmux and restarts, `Restart` works when tmux already dead, `Restart` errors when not started, `Restart` errors when paused, and `StartOnBranch` sets fields correctly. These tests use the mock `testPtyFactory` and `cmd_test.MockCmdExec` to avoid real tmux/git.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/... -run "TestStart|TestRestart|TestStartOn" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `session/instance_lifecycle.go` and rewrite from scratch:

- **`Start(firstTimeSetup bool) error`** — errors if Title empty. Sets loading total (8 for first-time, 6 for reload). Creates or reuses `tmuxSession` (via `tmux.NewTmuxSession`), sets agent type, wires task env, configures session title, wires progress callback, transfers prompt to CLI. For first-time: creates git worktree via `git.NewGitWorktree()`, sets branch, then sets up worktree via `gitWorktree.Setup()`, then starts tmux via `tmuxSession.Start()`. For reload: restores via `tmuxSession.Restore()`. Deferred cleanup on error: calls `Kill()`. On success: sets `started = true`, status to `Running`.
- **`StartOnMainBranch() error`** — errors if Title empty. Sets loading total to 5. Creates/reuses tmuxSession, configures it (agent type, task env, title, progress, prompt transfer). Starts tmux in `i.Path` (no worktree). Deferred cleanup on error. On success: `started = true`, status `Running`.
- **`StartOnBranch(branch string) error`** — errors if Title empty. Sets loading total to 8. Creates/reuses tmuxSession, configures it. Creates worktree via `git.NewGitWorktreeOnBranch()`, sets branch. Sets up worktree, starts tmux in worktree path. Deferred cleanup on error (includes worktree cleanup if tmux fails). On success: `started = true`, status `Running`.
- **`StartInSharedWorktree(worktree *git.GitWorktree, branch string) error`** — errors if Title empty. Sets loading total to 6. Assigns provided worktree and branch, sets `sharedWorktree = true`. Creates/reuses tmuxSession, configures it. Starts tmux in worktree path. On success: `started = true`, status `Running`.
- **`transferPromptToCli()`** — if `QueuedPrompt` is non-empty and `programSupportsCliPrompt(Program)` returns true, transfers prompt to tmuxSession via `SetInitialPrompt()` and clears `QueuedPrompt`.
- **`setTmuxTaskEnv()`** — if `TaskNumber > 0` and `tmuxSession != nil`, calls `tmuxSession.SetTaskEnv(TaskNumber, WaveNumber, PeerCount)`.
- **`buildTitleOpts(inst *Instance) opencodesession.TitleOpts`** — maps Instance metadata to `TitleOpts`: extracts plan display name from TaskFile via `taskstate.DisplayName()`, copies AgentType, WaveNumber, TaskNumber, InstanceTitle, ReviewCycle.
- **`configureSessionTitle()`** — no-op if tmuxSession nil or Program doesn't end with "opencode". Builds title opts, generates title via `opencodesession.BuildTitle()`, sets on tmuxSession via `SetSessionTitle()` and `SetTitleFunc()` (callback writes title to DB via `opencodesession.SetTitleDirect()`).
- **`Kill() error`** — returns nil if not started. Closes tmux session first (collects error). Then, if not shared worktree and gitWorktree exists: auto-commits dirty work (`gitWorktree.IsDirty()` → `CommitChanges()` with `[kas] auto-save` message), then calls `gitWorktree.Cleanup()`. Returns joined errors.
- **`StopTmux()`** — if tmuxSession non-nil, calls `Close()` (ignores error).
- **`Pause() error`** — errors if not started or already paused. For non-shared worktrees: checks dirty, commits changes with `[kas] update` message (returns early on commit failure). Detaches tmux via `DetachSafely()`. For non-shared worktrees: removes worktree via `Remove()`, prunes via `Prune()`. Sets status to `Paused`. Copies branch name to clipboard via `clipboard.WriteAll()`.
- **`AdoptOrphanTmuxSession(tmuxName string) error`** — creates `TmuxSession` via `tmux.NewTmuxSessionFromExisting()`, restores it, sets `started = true`, status `Ready`.
- **`Restart() error`** — errors if not started or paused. Best-effort closes existing tmux. Creates fresh tmux via `tmuxSession.NewReset()`, configures it (agent type, task env, title). Determines work dir (worktree path or `Path`). Starts tmux. Resets ephemeral state (Exited, PromptDetected, HasWorked, AwaitingWork, Notified, CachedContentSet, CachedContent). Sets status `Running`.
- **`Resume() error`** — errors if not started or not paused. Checks branch not checked out via `gitWorktree.IsBranchCheckedOut()`. Sets up worktree via `gitWorktree.Setup()`. If tmux session exists: restores (falls back to start on failure). If tmux session gone: starts new session. Sets status `Running`.

Imports: `errors`, `fmt`, `os`, `strings`, `time`, `github.com/kastheco/kasmos/config/taskstate`, `github.com/kastheco/kasmos/internal/opencodesession`, `github.com/kastheco/kasmos/log`, `github.com/kastheco/kasmos/session/git`, `github.com/kastheco/kasmos/session/tmux`, `github.com/atotto/clipboard`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run "TestStart|TestRestart|TestStartOn" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_lifecycle.go
git commit -m "feat(clean-room): rewrite session/instance_lifecycle.go from scratch"
```

### Task 8: Full Regression Suite and Cross-Package Compilation Check

**Files:**
- Modify: none (verification-only task)
- Test: all existing test files in `session/`

**Step 1: write the failing test**

No new tests — this task runs the full existing suite plus cross-package compilation.

**Step 2: run full session test suite**

```bash
go test ./session/... -v -count=1
```

expected: PASS — all tests across all test files pass

**Step 3: verify cross-package compilation**

```bash
go build ./...
```

expected: SUCCESS — all packages compile, confirming the public API is preserved. All callers in `app/`, `ui/`, `daemon/`, `main.go` compile without changes.

**Step 4: run full project test suite**

```bash
go test ./... -count=1
```

expected: PASS — no regressions in any package

**Step 5: commit**

No code changes to commit — this is a verification gate. If any test fails, fix the responsible file and amend the relevant earlier commit.
