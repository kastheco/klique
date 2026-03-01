# AGPL Escape: Clean-Room Rewrite of Upstream Code

**Goal:** Eliminate all AGPL-licensed upstream code (9,074 lines across 56 files) by rewriting
every non-kas-authored line from functional specification, enabling relicensing under a
license of our choosing.

**Architecture:** Each inherited file is processed via git-blame triage: upstream-authored lines
are deleted, then reimplemented from behavioral specification using surrounding kas-authored
code as context. 100% upstream files are rewritten wholesale. Mixed files get surgical
line-level deletion followed by gap-filling. Non-Go assets (LICENSE, install script, Makefile)
are replaced entirely. Final step isolates git history to remove upstream commits.

**Tech Stack:** Go 1.24+, bubbletea v1.3, lipgloss v1.1, go-git/v5, creack/pty, charmbracelet/x/vt,
tmux CLI, git CLI

**Size:** Large (estimated ~10-14 hours, 11 tasks, 4 waves)

---

## Methodology

Every task follows the same rewrite protocol:

1. **Identify** — `git blame <file>` to mark upstream vs kas lines
2. **Specify** — document what each upstream block *does* (behavior, not expression)
3. **Test** — write new tests exercising the specified behavior (different structure,
   names, and approach than originals — even if the original tests were upstream)
4. **Delete** — remove all upstream-authored lines from both impl and test files
5. **Rewrite** — implement from the behavioral spec to pass the new tests
6. **Verify** — `go test ./...` passes, `go build ./...` succeeds

**What is safe to keep:**
- All kas-authored lines (git blame confirms authorship)
- Function signatures and type names (APIs/interfaces are not copyrightable expression)
- Third-party library imports (bubbletea, lipgloss, go-git are independent works)
- go.mod/go.sum (module path already `github.com/kastheco/kasmos`, deps are third-party)

**What must be rewritten:**
- Every line attributed to Jayant Shrivastava, Mufeez Amjad, Fabian Urbanek, or other
  upstream contributors in `git blame`
- Test files that are 100% upstream (new tests must use different structure)
- Non-Go files: LICENSE.md, install.sh, CONTRIBUTING.md, Makefile, clean scripts

---

## Upstream Code Inventory

### Tier 1: 100% upstream (full rewrite, 1,085 lines)

| File | Lines | Function |
|------|-------|----------|
| `session/activity.go` | 156 | ANSI-strip + regex activity parser for agent output |
| `session/activity_test.go` | 230 | Table tests for Claude/Aider/Codex activity patterns |
| `ui/overlay/toast_test.go` | 226 | Toast manager lifecycle + animation phase tests |
| `ui/gradient.go` | 99 | Hex parse, lerp, truecolor gradient text renderer |
| `ui/gradient_test.go` | 98 | Gradient edge cases (empty, single char, newlines) |
| `session/git/util.go` | 67 | Branch name sanitizer, gh CLI detection |
| `session/git/util_test.go` | 73 | sanitizeBranchName table tests |
| `session/git/worktree_branch.go` | 43 | Branch ref cleanup for worktree recycling |
| `session/notify.go` | 38 | Cross-platform desktop notification (macOS/Linux) |
| `session/tmux/pty.go` | 26 | PTY factory interface + creack/pty adapter |
| `daemon/daemon_windows.go` | 15 | Windows CREATE_NEW_PROCESS_GROUP proc attrs |
| `daemon/daemon_unix.go` | 14 | Unix Setsid proc attrs for daemon detach |

### Tier 2: >60% upstream (heavy rewrite, 4,734 lines upstream)

| File | Upstream/Total | Function |
|------|---------------|----------|
| `ui/diff.go` | 369/382 (96%) | Diff pane: file chunk parser, syntax-colored +/- renderer |
| `session/terminal.go` | 234/247 (94%) | EmbeddedTerminal: tmux attach PTY → vt emulator → screen buffer |
| `config/state.go` | 155/157 (98%) | InstanceStorage + AppState interfaces, paths |
| `daemon/daemon.go` | 163/167 (97%) | Daemon loop: iterate sessions, auto-accept, launch/kill |
| `session/git/worktree_git.go` | 165/169 (97%) | runGitCommand, PushChanges, GeneratePRBody |
| `ui/overlay/overlay.go` | 220/247 (89%) | PlaceOverlay: ANSI-aware overlay compositor |
| `ui/overlay/contextMenu.go` | 215/253 (84%) | Context menu: items, search filter, arrow nav, render |
| `session/tmux/tmux_attach.go` | 179/210 (85%) | Attach/DetachSafely, outer mouse save/restore |
| `ui/overlay/toast.go` | 313/378 (82%) | Toast manager: queue, animation phases, tick lifecycle |
| `ui/overlay/textInput.go` | 125/157 (79%) | Text input overlay: single/multiline, tab cycle |
| `ui/overlay/pickerOverlay.go` | 163/206 (79%) | Picker overlay: filterable list selection |
| `session/storage.go` | 134/174 (77%) | InstanceData/GitWorktreeData JSON serialization |
| `session/git/diff.go` | 46/61 (75%) | DiffStats struct, git diff --stat parser |
| `session/git/worktree.go` | 96/135 (71%) | GitWorktree struct, constructor, worktree dir resolution |
| `ui/preview_test.go` | 326/459 (71%) | Preview pane rendering tests with mock instances |
| `log/log.go` | 70/87 (80%) | File logger init, global log file, Close() |
| `main.go` | 152/220 (69%) | CLI flags, cobra root command, Run() entry |
| `cmd/cmd.go` | 32/46 (69%) | Executor interface for exec.Cmd (testability shim) |
| `app/help.go` | 136/198 (68%) | Help text interface, keybinding descriptions per state |
| `session/git/worktree_ops.go` | 213/317 (67%) | Setup, setupFromExistingBranch, syncBranchWithRemote |
| `session/instance_lifecycle.go` | 273/440 (62%) | Start, StartOnMainBranch, Resume, Kill, Cleanup |
| `session/instance.go` | 234/381 (61%) | Instance struct, Status enum, AgentType, constructors |
| `session/instance_session.go` | 172/277 (62%) | Preview, HasUpdated, NewEmbeddedTerminalForInstance |
| `ui/overlay/confirmationOverlay.go` | 82/83 (98%) | Yes/No dialog: arrow nav, enter confirm, render |
| `ui/overlay/textOverlay.go` | 54/55 (98%) | Read-only text display overlay with scroll |
| `session/tmux/tmux_unix.go` | 77/78 (98%) | SIGWINCH monitor for resize during attach |
| `session/tmux/tmux_windows.go` | 56/57 (98%) | Windows resize polling during attach |
| `app/folder_picker.go` | 61/62 (98%) | Native OS folder picker via zenity/osascript |

### Tier 3: 20-60% upstream (surgical deletion + gap-fill, 3,255 lines upstream)

| File | Upstream/Total | Function |
|------|---------------|----------|
| `ui/tabbed_window.go` | 260/443 (58%) | Tab bar: custom border, tab switching, content render |
| `config/config.go` | 156/275 (56%) | GetConfigDir (XDG + legacy migration), DefaultConfig |
| `config/config_test.go` | 154/335 (45%) | Config loading, migration, default command tests |
| `cmd/cmd_test/testutils.go` | 18/31 (58%) | MockCmdExec with RunFunc/OutputFunc overrides |
| `keys/keys.go` | 133/267 (49%) | KeyName enum, GlobalKeyStringsMap, bindings |
| `ui/menu.go` | 187/408 (45%) | Bottom menu bar: keybind hints, action groups, styles |
| `app/app_input.go` | 715/1625 (44%) | handleMenuHighlighting, handleMouse, handleRightClick |
| `session/tmux/tmux.go` | 266/628 (42%) | TmuxSession struct, NewSession, pane operations |
| `ui/preview.go` | 237/625 (37%) | PreviewPane: spring anim, tab content routing, render |
| `session/tmux/tmux_io.go` | 83/247 (33%) | Permission detection, TapRight, keystroke injection |
| `app/app_test.go` | 454/1450 (31%) | newTestHome, spawn/kill/lifecycle integration tests |

### Tier 4: <20% upstream (minimal, 811 lines upstream)

| File | Upstream/Total | Function |
|------|---------------|----------|
| `app/app.go` | 384/2060 (18%) | Run(), model init, Update loop scaffolding |
| `session/tmux/tmux_test.go` | 136/743 (18%) | MockPtyFactory, cleanup regex tests |
| `app/app_actions.go` | 112/932 (12%) | executeContextAction base structure |
| `app/app_state.go` | 175/1944 (9%) | mergeTopicStatus/mergePlanStatus origin lines |
| `ui/consts.go` | 4/61 (6%) | 4 style constant lines |

### Non-Go files

| File | Upstream % | Action |
|------|-----------|--------|
| `LICENSE.md` | 100% | Replace with chosen license |
| `CONTRIBUTING.md` | 92% | Rewrite or remove |
| `install.sh` | 97% | Rewrite (shell one-liner installer) |
| `Makefile` | 76% | Rewrite (already have Justfile) |
| `clean.sh` | 40% | Rewrite |
| `clean_hard.sh` | 50% | Rewrite |
| `README.md` | 21% | Remove upstream paragraphs |
| `assets/screenshot.png` | 100% | Replace with current screenshot |

---

## Wave 1: Standalone Utilities

All files here have zero cross-dependencies and can be worked in parallel. These are
the smallest, simplest rewrites — mostly pure functions and thin interfaces.

### Task 1: Pure Function Utilities

Rewrite standalone pure functions and trivial interfaces that have no internal dependencies.

**Files:**
- Rewrite: `ui/gradient.go` (99 lines, 100% upstream)
- Rewrite: `ui/gradient_test.go` (98 lines, 100% upstream)
- Rewrite: `session/notify.go` (38 lines, 100% upstream)
- Rewrite: `session/tmux/pty.go` (26 lines, 100% upstream)
- Rewrite: `daemon/daemon_unix.go` (14 lines, 100% upstream)
- Rewrite: `daemon/daemon_windows.go` (15 lines, 100% upstream)
- Rewrite: `log/log.go` (70/87 upstream)
- Rewrite: `cmd/cmd.go` (32/46 upstream)
- Rewrite: `cmd/cmd_test/testutils.go` (18/31 upstream)

**Step 1: write the failing test**

New tests for gradient (different structure than upstream):

```go
// ui/gradient_test.go — all new
func TestParseHex_Valid(t *testing.T) {
    r, g, b := parseHex("#FF8800")
    assert.Equal(t, uint8(0xFF), r)
    assert.Equal(t, uint8(0x88), g)
    assert.Equal(t, uint8(0x00), b)
}

func TestGradientText_Boundaries(t *testing.T) {
    assert.Equal(t, "", GradientText("", "#000000", "#FFFFFF"))
    result := GradientText("X", "#FF0000", "#0000FF")
    assert.Contains(t, result, "X")
    assert.Contains(t, result, "\033[")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run TestParseHex_Valid -v
go test ./ui/... -run TestGradientText_Boundaries -v
```

expected: FAIL (functions deleted)

**Step 3: write minimal implementation**

For each file:
1. Run `git blame <file>` to identify upstream lines
2. Delete all upstream-authored lines
3. Rewrite from behavioral spec:
   - `gradient.go`: hex→RGB parser, byte lerp, ANSI truecolor string builder
   - `notify.go`: fire-and-forget `notify-send` (Linux) / `osascript` (macOS)
   - `pty.go`: `PtyFactory` interface with `Start(*exec.Cmd)` and `Close()`, thin wrapper over `creack/pty`
   - `daemon_unix.go`: return `&syscall.SysProcAttr{Setsid: true}`
   - `daemon_windows.go`: return `&syscall.SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS}`
   - `log.go`: open temp file, set `log.SetOutput`, expose `Initialize()` + `Close()`
   - `cmd/cmd.go`: `Executor` interface with `Run`/`Output`, concrete `Exec` struct
   - `cmd_test/testutils.go`: `MockCmdExec` with injectable `RunFunc`/`OutputFunc`

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run TestParseHex -v
go test ./ui/... -run TestGradientText -v
go test ./cmd/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add ui/gradient.go ui/gradient_test.go session/notify.go session/tmux/pty.go \
  daemon/daemon_unix.go daemon/daemon_windows.go log/log.go cmd/cmd.go cmd/cmd_test/testutils.go
git commit -m "feat: rewrite standalone utilities from behavioral spec"
```

### Task 2: Session Utilities

Rewrite activity parser, git branch helpers, and their tests.

**Files:**
- Rewrite: `session/activity.go` (156 lines, 100% upstream)
- Rewrite: `session/activity_test.go` (230 lines, 100% upstream)
- Rewrite: `session/git/util.go` (67 lines, 100% upstream)
- Rewrite: `session/git/util_test.go` (73 lines, 100% upstream)
- Rewrite: `session/git/worktree_branch.go` (43 lines, 100% upstream)

**Step 1: write the failing test**

New activity parser tests (testify table-driven, different layout than upstream):

```go
func TestParseActivity(t *testing.T) {
    tests := []struct {
        name    string
        content string
        agent   string
        action  string
        detail  string
    }{
        {"claude edit", "\x1b[36m⠙\x1b[0m Editing main.go\n", "claude", "editing", "main.go"},
        {"opencode bash", "⠙ Running bash command\n", "opencode", "running", "bash command"},
        {"no match", "just some text\n", "claude", "", ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            a := ParseActivity(tt.content, tt.agent)
            if tt.action == "" {
                assert.Nil(t, a)
            } else {
                require.NotNil(t, a)
                assert.Equal(t, tt.action, a.Action)
            }
        })
    }
}
```

New branch sanitizer tests:

```go
func TestSanitizeBranchName_Comprehensive(t *testing.T) {
    cases := map[string]string{
        "simple":            "simple",
        "With Spaces":       "with-spaces",
        "UPPER_case":        "upper_case",
        "special!@#chars":   "special-chars",
        "multi---dashes":    "multi-dashes",
    }
    for input, expected := range cases {
        assert.Equal(t, expected, sanitizeBranchName(input), "input: %s", input)
    }
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/... -run TestParseActivity -v
go test ./session/git/... -run TestSanitizeBranchName_Comprehensive -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `activity.go`: strip ANSI codes with regex, match spinner patterns per agent
  (Claude: `⠙ Editing/Writing/Reading/Running/Searching <detail>`,
  OpenCode/Codex: similar patterns), return `Activity{Action, Detail, Timestamp}`
- `git/util.go`: lowercase → strip unsafe chars → collapse dashes → trim edges;
  `checkGHCLI` runs `gh auth status` and checks exit code
- `git/worktree_branch.go`: remove branch ref + worktree-specific refs from storer

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run TestParseActivity -v
go test ./session/git/... -run TestSanitizeBranchName -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add session/activity.go session/activity_test.go \
  session/git/util.go session/git/util_test.go session/git/worktree_branch.go
git commit -m "feat: rewrite session utilities from behavioral spec"
```

### Task 3: Config + Storage Layer

Rewrite config directory resolution, state interfaces, and storage serialization.

**Files:**
- Modify: `config/config.go` (156/275 upstream — delete upstream lines, rewrite)
- Modify: `config/config_test.go` (154/335 upstream — delete upstream tests, write new)
- Rewrite: `config/state.go` (155/157 upstream)
- Modify: `session/storage.go` (134/174 upstream)

**Step 1: write the failing test**

```go
func TestGetConfigDir_XDGCompliant(t *testing.T) {
    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)
    dir, err := GetConfigDir()
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(tmpHome, ".config", "kasmos"), dir)
}

func TestGetConfigDir_MigratesLegacy(t *testing.T) {
    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)
    legacyDir := filepath.Join(tmpHome, ".klique")
    require.NoError(t, os.MkdirAll(legacyDir, 0o755))
    dir, err := GetConfigDir()
    require.NoError(t, err)
    assert.Contains(t, dir, "kasmos")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/... -run TestGetConfigDir -v
```

expected: FAIL (upstream functions deleted)

**Step 3: write minimal implementation**

Behavioral specs:
- `config.go`: `GetConfigDir()` → `~/.config/kasmos/`, migrate from `~/.klique` or
  `~/.hivemind` if they exist. `DefaultConfig()` returns sensible defaults.
  `aliasRegex` resolves shell aliases for agent commands.
- `state.go`: `InstanceStorage` interface (Save/Load/Delete), `AppState` interface
  (Get/Set last repo, window size). Path constants for state files.
- `storage.go`: `InstanceData`/`GitWorktreeData`/`DiffStatsData` structs with JSON tags,
  `ToData()`/`FromData()` conversion methods.

**Step 4: run test to verify it passes**

```bash
go test ./config/... -v
go test ./session/... -run TestStorage -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add config/config.go config/config_test.go config/state.go session/storage.go
git commit -m "feat: rewrite config and storage layer from behavioral spec"
```

## Wave 2: Core Infrastructure

> **depends on wave 1:** uses Executor interface (cmd.go), PtyFactory (pty.go),
> logger (log.go), config paths (config.go), branch utils (git/util.go)

### Task 4: Git Worktree Operations

Rewrite the git worktree management layer — the core of how sessions get isolated branches.

**Files:**
- Modify: `session/git/worktree.go` (96/135 upstream)
- Rewrite: `session/git/worktree_git.go` (165/169 upstream)
- Modify: `session/git/worktree_ops.go` (213/317 upstream)
- Modify: `session/git/diff.go` (46/61 upstream)

**Step 1: write the failing test**

```go
func TestGitWorktree_DiffStats(t *testing.T) {
    // Setup: init a repo, create worktree, make a change, verify DiffStats
    repo := setupTestRepo(t)
    wt := NewGitWorktree(repo.Path, "test-session", "test-branch", "")
    require.NoError(t, wt.Setup())
    // ... write a file in worktree, verify DiffStats returns non-empty
    stats := wt.Diff()
    assert.False(t, stats.IsEmpty())
}

func TestGitWorktree_PushChanges(t *testing.T) {
    // Verify commit + push flow with mock executor
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/git/... -run TestGitWorktree -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `worktree.go`: `GitWorktree` struct (repoPath, worktreePath, sessionName, branchName,
  baseCommitSHA), constructor, `getWorktreeDirectory()` for `.worktrees/` path resolution
- `worktree_git.go`: `runGitCommand()` exec wrapper, `PushChanges()` (add all → commit →
  push origin), `GeneratePRBody()` (diff summary + commit log → markdown)
- `worktree_ops.go`: `Setup()` (create branch, `git worktree add`), `setupFromExistingBranch()`
  (checkout existing remote branch into worktree), `syncBranchWithRemote()` (fast-forward)
- `diff.go`: `DiffStats{Added, Removed, Files}`, parse `git diff --stat` output

**Step 4: run test to verify it passes**

```bash
go test ./session/git/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add session/git/worktree.go session/git/worktree_git.go \
  session/git/worktree_ops.go session/git/diff.go
git commit -m "feat: rewrite git worktree operations from behavioral spec"
```

### Task 5: Tmux Session Layer

Rewrite tmux session management — the other core plumbing alongside git worktrees.

**Files:**
- Modify: `session/tmux/tmux.go` (266/628 upstream)
- Modify: `session/tmux/tmux_io.go` (83/247 upstream)
- Modify: `session/tmux/tmux_attach.go` (179/210 upstream)
- Rewrite: `session/tmux/tmux_unix.go` (77/78 upstream)
- Rewrite: `session/tmux/tmux_windows.go` (56/57 upstream)
- Modify: `session/tmux/tmux_test.go` (136/743 upstream)

**Step 1: write the failing test**

```go
func TestTmuxSession_CleanupRegex(t *testing.T) {
    // Verify the cleanup regex matches kas_, klique_, hivemind_ prefixes
    re := cleanupSessionsRe
    assert.True(t, re.MatchString("kas_abc123:"))
    assert.True(t, re.MatchString("klique_old:"))
    assert.True(t, re.MatchString("hivemind_legacy:"))
    assert.False(t, re.MatchString("unrelated_session:"))
}

func TestTmuxSession_PermissionDetection(t *testing.T) {
    // Verify opencode permission prompt detection patterns
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/tmux/... -run TestTmuxSession -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `tmux.go`: `TmuxSession` struct (name, pane, program, pty factory, started, etc.),
  `NewSession()`, `SendKeys()`, `CapturePane()`, `Kill()`, `IsAlive()`,
  ANSI stripping regex, cleanup regex for kas/klique/hivemind prefixes
- `tmux_io.go`: `PermissionChoice` enum, `TapRight()`, permission prompt detection for
  opencode's Allow/Reject dialogs, keystroke injection helpers
- `tmux_attach.go`: `Attach()` — opens PTY to `tmux attach-session`, returns done channel.
  `restoreOuterMouse()` re-enables mouse in outer tmux. `DetachSafely()`.
- `tmux_unix.go`: `monitorWindowSize()` — listen for SIGWINCH, relay to tmux pane
- `tmux_windows.go`: `monitorWindowSize()` — poll-based resize for Windows

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_io.go session/tmux/tmux_attach.go \
  session/tmux/tmux_unix.go session/tmux/tmux_windows.go session/tmux/tmux_test.go
git commit -m "feat: rewrite tmux session layer from behavioral spec"
```

### Task 6: Session Model + Daemon

Rewrite the instance model, lifecycle management, and daemon — the glue that ties
git worktrees and tmux sessions into a managed agent instance.

**Files:**
- Modify: `session/instance.go` (234/381 upstream)
- Modify: `session/instance_lifecycle.go` (273/440 upstream)
- Modify: `session/instance_session.go` (172/277 upstream)
- Rewrite: `session/terminal.go` (234/247 upstream)
- Rewrite: `daemon/daemon.go` (163/167 upstream)

**Step 1: write the failing test**

```go
func TestInstance_StatusTransitions(t *testing.T) {
    // Verify Status enum values and string representations
    assert.Equal(t, "running", StatusRunning.String())
    assert.Equal(t, "paused", StatusPaused.String())
}

func TestEmbeddedTerminal_Creation(t *testing.T) {
    // Verify terminal creation returns valid screen dimensions
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/... -run TestInstance -v
go test ./session/... -run TestEmbeddedTerminal -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `instance.go`: `Instance` struct (Title, Status, AgentType, GitWorktree, TmuxSession,
  prompt, plan metadata), `Status` enum (Running/Paused/Error/Stopped), constructors
- `instance_lifecycle.go`: `Start(firstTimeSetup)` — creates worktree + tmux session,
  sends prompt. `StartOnMainBranch()` — no worktree, runs in repo root (planners).
  `Resume()`, `Kill()`, `Cleanup()` — state machine transitions.
- `instance_session.go`: `Preview()` — capture tmux pane. `HasUpdated()` — check for new
  output since last read. `NewEmbeddedTerminalForInstance()` — attach PTY, connect vt emulator.
- `terminal.go`: `EmbeddedTerminal` — dedicated tmux attach PTY, reads into
  `charmbracelet/x/vt` emulator, renders from screen buffer. `Update()`, `View()`, `Resize()`.
- `daemon.go`: `RunDaemon()` — load config, iterate sessions, auto-accept prompts.
  `LaunchDaemon()` — fork process with platform-specific detach attrs.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -v
go test ./daemon/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance.go session/instance_lifecycle.go session/instance_session.go \
  session/terminal.go daemon/daemon.go
git commit -m "feat: rewrite session model and daemon from behavioral spec"
```

## Wave 3: UI Components

> **depends on wave 2:** overlays render instance data, preview pane uses session.Preview(),
> diff pane uses git.DiffStats, menu shows keybindings from keys package

### Task 7: Overlay System

Rewrite the overlay rendering primitives — the foundation for all dialogs and menus.

**Files:**
- Rewrite: `ui/overlay/overlay.go` (220/247 upstream)
- Modify: `ui/overlay/toast.go` (313/378 upstream)
- Rewrite: `ui/overlay/toast_test.go` (226 lines, 100% upstream)
- Rewrite: `ui/overlay/confirmationOverlay.go` (82/83 upstream)
- Rewrite: `ui/overlay/textOverlay.go` (54/55 upstream)
- Modify: `ui/overlay/textInput.go` (125/157 upstream)
- Modify: `ui/overlay/contextMenu.go` (215/253 upstream)
- Modify: `ui/overlay/pickerOverlay.go` (163/206 upstream)

**Step 1: write the failing test**

```go
func TestPlaceOverlay_CentersContent(t *testing.T) {
    bg := strings.Repeat(strings.Repeat(" ", 40)+"\n", 20)
    fg := "hello"
    result := PlaceOverlay(20, 10, fg, bg)
    assert.Contains(t, result, "hello")
}

func TestToastManager_Lifecycle(t *testing.T) {
    tm := NewToastManager()
    id := tm.Show("test", ToastInfo, 0)
    assert.NotEmpty(t, id)
    assert.Len(t, tm.Active(), 1)
    tm.Dismiss(id)
    assert.Len(t, tm.Active(), 0)
}

func TestConfirmationOverlay_YesNo(t *testing.T) {
    co := NewConfirmationOverlay("delete?")
    // Simulate pressing right (switch to No) then Enter
    _, action := co.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
    assert.Equal(t, "", action)
    _, action = co.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "no", action)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `overlay.go`: `PlaceOverlay(x, y, fg, bg)` — split bg into lines, splice fg at coords,
  handle ANSI color codes (regex replacement for proper overlay compositing).
  Derived from lipgloss PR #102 approach but rewritten.
- `toast.go`: `ToastManager` with `Show()`/`Dismiss()`/`Active()`, animation phases
  (SlideIn → Visible → SlideOut → Gone), tick-driven lifecycle
- `confirmationOverlay.go`: yes/no selection, arrow key toggle, enter confirm
- `textOverlay.go`: scrollable read-only text display, esc/q to close
- `textInput.go`: single/multiline text input, tab focus cycle
- `contextMenu.go`: item list + search filter, arrow nav, enter select, esc cancel
- `pickerOverlay.go`: filterable item picker with search + arrow selection

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/overlay.go ui/overlay/toast.go ui/overlay/toast_test.go \
  ui/overlay/confirmationOverlay.go ui/overlay/textOverlay.go \
  ui/overlay/textInput.go ui/overlay/contextMenu.go ui/overlay/pickerOverlay.go
git commit -m "feat: rewrite overlay system from behavioral spec"
```

### Task 8: Panes, Menu, and Keys

Rewrite the main UI panes (diff, preview, tabs), the bottom menu bar, and keybinding
definitions.

**Files:**
- Modify: `ui/diff.go` (369/382 upstream)
- Modify: `ui/preview.go` (237/625 upstream)
- Modify: `ui/preview_test.go` (326/459 upstream)
- Modify: `ui/menu.go` (187/408 upstream)
- Modify: `ui/tabbed_window.go` (260/443 upstream)
- Modify: `keys/keys.go` (133/267 upstream)

**Step 1: write the failing test**

```go
func TestDiffPane_ParsesUnifiedDiff(t *testing.T) {
    raw := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,4 @@\n line1\n+added\n line2\n"
    dp := NewDiffPane()
    dp.SetContent(raw, 80, 24)
    view := dp.View()
    assert.Contains(t, view, "added")
}

func TestTabbedWindow_SwitchTabs(t *testing.T) {
    tw := NewTabbedWindow([]Tab{{Name: "info"}, {Name: "agent"}, {Name: "diff"}})
    assert.Equal(t, 0, tw.ActiveTab())
    tw.NextTab()
    assert.Equal(t, 1, tw.ActiveTab())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run TestDiffPane -v
go test ./ui/... -run TestTabbedWindow -v
```

expected: FAIL

**Step 3: write minimal implementation**

Behavioral specs:
- `diff.go`: `DiffPane` — parse unified diff into `fileChunk` slices, render with
  green +lines / red -lines, file headers, line numbers. Scroll support.
- `preview.go`: `PreviewPane` — routes content to active tab (agent output, diff, info).
  Spring load-in animation via harmonica. `TickSpring()`, `View()`.
- `menu.go`: Bottom bar rendering — key hint sections separated by `•`, action groups
  (navigation, session control, plan actions). Context-aware hint swapping.
- `tabbed_window.go`: `TabbedWindow` with custom lipgloss border, `Tab` struct,
  `NextTab()`/`PrevTab()`, `SetActiveTab()`, content routing by index.
- `keys/keys.go`: `KeyName` enum, `GlobalKeyStringsMap` mapping string→keybinding,
  context-dependent binding groups.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -v
go test ./keys/... -v
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add ui/diff.go ui/preview.go ui/preview_test.go ui/menu.go \
  ui/tabbed_window.go keys/keys.go
git commit -m "feat: rewrite UI panes, menu, and keybindings from behavioral spec"
```

## Wave 4: App Layer + License

> **depends on waves 1-3:** app layer orchestrates all session, UI, and config subsystems

### Task 9: App Core

Rewrite the remaining upstream lines in the app package — the top-level orchestration
that ties everything together.

**Files:**
- Modify: `app/app.go` (384/2060 upstream — 18%, surgical)
- Modify: `app/app_input.go` (715/1625 upstream — 44%)
- Modify: `app/app_state.go` (175/1944 upstream — 9%, surgical)
- Modify: `app/app_actions.go` (112/932 upstream — 12%, surgical)
- Modify: `app/app_test.go` (454/1450 upstream — 31%)
- Modify: `app/help.go` (136/198 upstream)
- Rewrite: `app/folder_picker.go` (61/62 upstream)
- Modify: `main.go` (152/220 upstream)

**Step 1: write the failing test**

```go
func TestHome_InitializesCorrectly(t *testing.T) {
    h := newTestHome()
    assert.NotNil(t, h)
    assert.Equal(t, stateDefault, h.state)
}

func TestFolderPicker_ReturnsPath(t *testing.T) {
    // Mock test — verify folderPickedMsg is handled
    h := newTestHome()
    msg := folderPickedMsg{path: "/tmp/test", err: nil}
    model, _ := h.Update(msg)
    assert.NotNil(t, model)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -v
```

expected: FAIL

**Step 3: write minimal implementation**

For `app.go`, `app_state.go`, `app_actions.go` (all <20% upstream): use `git blame` to
identify specific upstream lines, delete them, fill gaps from context. These files are
overwhelmingly kas-authored — the upstream lines are mostly original scaffolding that
has been extended beyond recognition.

For `app_input.go` (44% upstream): the upstream lines are original key handlers and mouse
event processing. Delete upstream blocks, rewrite input routing from the current keybinding
map and handler signatures.

For `app_test.go` (31% upstream): delete upstream test functions, write new tests covering
the same lifecycle scenarios with different structure.

For `help.go` (68% upstream): rewrite the help text interface and per-state descriptions.

For `folder_picker.go` (98% upstream): rewrite zenity/osascript folder picker invocation.

For `main.go` (69% upstream): rewrite cobra command setup, flag parsing, `Run()` call.

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v
go test ./... -count=1
go build ./...
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_input.go app/app_state.go app/app_actions.go \
  app/app_test.go app/help.go app/folder_picker.go main.go
git commit -m "feat: rewrite app layer from behavioral spec"
```

### Task 10: Remaining Mixed Files

Clean up the remaining files with minor upstream contamination — keybinding constants
and consts.

**Files:**
- Modify: `ui/consts.go` (4/61 upstream — 6%)

**Step 1: write the failing test**

Not applicable — `ui/consts.go` contains only constant declarations (banner art, block
glyphs). No testable logic. The 4 upstream lines are style constants that can be trivially
replaced with different values.

**Step 2: run test to verify it fails**

Skipped — constants file, no behavioral test needed.

**Step 3: write minimal implementation**

Use `git blame ui/consts.go` to identify the 4 upstream lines. Delete them and replace
with equivalent constant values in different form (rewritten expressions, not copy).

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./ui/... -v
```

expected: PASS (no behavior change, just expression change)

**Step 5: commit**

```bash
git add ui/consts.go
git commit -m "feat: replace remaining upstream constants"
```

### Task 11: License Swap + Non-Go Cleanup

Replace all non-Go upstream files with originals. This is the legal capstone — after this
task, zero upstream-copyrighted content remains in the repository.

**Files:**
- Replace: `LICENSE.md` (100% upstream AGPL → chosen license)
- Rewrite: `install.sh` (97% upstream — new installer)
- Remove: `CONTRIBUTING.md` (92% upstream — rewrite or delete)
- Remove: `Makefile` (76% upstream — already have Justfile)
- Rewrite: `clean.sh` (40% upstream)
- Rewrite: `clean_hard.sh` (50% upstream)
- Modify: `README.md` (21% upstream — remove upstream paragraphs)
- Replace: `assets/screenshot.png` (100% upstream)
- Remove: `CLA.md` (100% upstream)

**Step 1: write the failing test**

Not applicable — non-Go assets, no programmatic tests.

**Step 2: run test to verify it fails**

Skipped — non-code files.

**Step 3: write minimal implementation**

1. Replace `LICENSE.md` with chosen license text (MIT, Apache-2.0, or custom)
2. Delete `CONTRIBUTING.md` (or write new contribution guidelines)
3. Delete `Makefile` (Justfile already covers all recipes)
4. Delete `CLA.md` (upstream CLA doesn't apply)
5. Rewrite `install.sh` — simple curl-pipe-bash installer for GitHub releases
6. Rewrite `clean.sh` / `clean_hard.sh` — `rm -rf ~/.config/kasmos` etc.
7. Edit `README.md` — remove any paragraphs with upstream authorship per `git blame`
8. Replace `assets/screenshot.png` with current kasmos screenshot

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./... -count=1
```

expected: PASS (no Go changes)

**Step 5: commit**

```bash
git add LICENSE.md install.sh clean.sh clean_hard.sh README.md assets/
git rm -f CONTRIBUTING.md Makefile CLA.md
git commit -m "feat: replace license and non-Go upstream assets"
```

---

## Post-Completion: Git History Isolation

After all 11 tasks are complete and the codebase contains zero upstream code, the final
step is isolating git history. This is a manual operation (not a kasmos task):

**Option A: Squash-and-restart (recommended)**

```bash
# Create a fresh orphan branch with current tree
git checkout --orphan fresh-main
git add -A
git commit -m "kasmos: initial commit (clean-room rewrite)"
git branch -D main
git branch -m fresh-main main
git push origin main --force
```

This removes all upstream commits from history. Tags and releases remain on GitHub but
the branch history starts fresh.

**Option B: `git filter-repo` to remove upstream-authored commits**

More surgical but complex. Preserves your commit history while removing upstream commits.
Requires careful testing.

**Option C: Keep history as-is**

If the codebase has zero upstream code, the history itself isn't a copyright concern —
the commits show what *was changed*, not a copy of the original work. The clean-room
rewrite tasks create a clear paper trail showing deliberate replacement. This option is
lowest risk of data loss but provides the weakest legal separation.

---

## Verification Checklist

After all waves complete:

- [ ] `git blame --line-porcelain <file> | rg '^author ' | rg -v 'author kas' | rg -c '.'`
  returns 0 for every tracked `.go` file
- [ ] `go build ./...` succeeds
- [ ] `go test ./... -count=1` passes
- [ ] `LICENSE.md` contains chosen license (not AGPL)
- [ ] No file in `git ls-tree -r HEAD` has upstream authorship per `git blame`
- [ ] README contains no upstream-authored paragraphs
