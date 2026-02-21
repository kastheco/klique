# Pane Orchestration — Lifecycle, Parking, Polling, Tagging

## Table of Contents
- [Spawning](#spawning)
- [Parking Window](#parking-window)
- [Visibility Swapping](#visibility-swapping)
- [Status Polling](#status-polling)
- [Pane Tagging](#pane-tagging)
- [Content Capture](#content-capture)
- [Crash Recovery](#crash-recovery)
- [Focus Management](#focus-management)
- [Graceful Shutdown](#graceful-shutdown)

---

## Spawning

Creating a worker pane means splitting the current tmux window to place a new pane
beside the kasmos dashboard.

### Spawn Sequence

```go
// SpawnWorker creates a new worker pane running the given command.
func (b *TmuxBackend) spawnWorker(workerID string, cmd []string) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        // 1. If another worker is already visible, park it first
        if b.visiblePaneID != "" {
            err := b.tmux.JoinPane(ctx, tmux.JoinOpts{
                Source:   b.visiblePaneID,
                Target:   b.parkingWindowID,
                Detached: true, // don't follow focus
            })
            if err != nil && !tmux.IsNotFound(err) {
                return workerSpawnFailedMsg{workerID: workerID, err: err}
            }
        }

        // 2. Split dashboard pane to create worker pane
        paneID, err := b.tmux.SplitWindow(ctx, tmux.SplitOpts{
            Target:     b.dashboardPaneID,
            Horizontal: true,
            Size:       "50%",
            Command:    cmd,
        })
        if err != nil {
            return workerSpawnFailedMsg{workerID: workerID, err: err}
        }

        // 3. Tag the pane for crash recovery
        tagKey := fmt.Sprintf("KASMOS_PANE_%s", workerID)
        _ = b.tmux.SetEnvironment(ctx, tagKey, paneID)

        // 4. Focus dashboard so user stays in control
        _ = b.tmux.SelectPane(ctx, b.dashboardPaneID)

        return workerSpawnedMsg{
            workerID: workerID,
            paneID:   paneID,
        }
    }
}
```

### Key Decisions

- **Park before split**: If there's already a visible worker, park it first. Only one
  worker pane visible at a time.
- **`-P -F '#{pane_id}'`**: Captures the new pane's ID from stdout. This ID is the
  stable handle for all future operations on this pane.
- **Size `50%`**: Default split. The dashboard gets half, the worker gets half. This can
  be made configurable.
- **Focus back to dashboard**: After splitting, `select-pane` returns focus to the
  dashboard so the user stays in bubbletea's input loop. The user explicitly switches
  focus to the worker pane when they want to interact.

### Command Construction for Workers

```go
// buildWorkerCommand constructs the opencode/claude-code command for a worker.
func buildWorkerCommand(role, prompt string, opts WorkerOpts) []string {
    cmd := []string{"opencode", "run"}

    if opts.Agent != "" {
        cmd = append(cmd, "--agent", opts.Agent)
    }
    if opts.Model != "" {
        cmd = append(cmd, "--model", opts.Model)
    }
    for _, f := range opts.Files {
        cmd = append(cmd, "--file", f)
    }
    if opts.ContinueSession != "" {
        cmd = append(cmd, "--continue", "-s", opts.ContinueSession)
    }

    cmd = append(cmd, prompt)
    return cmd
}

// For claude-code workers:
func buildClaudeCodeCommand(prompt string, opts WorkerOpts) []string {
    cmd := []string{"claude", "--dangerously-skip-permissions"}

    if opts.Model != "" {
        cmd = append(cmd, "--model", opts.Model)
    }
    if opts.ContinueSession != "" {
        cmd = append(cmd, "--continue", opts.ContinueSession)
    }

    cmd = append(cmd, "-p", prompt)
    return cmd
}
```

### Wrapping Worker Command for Exit Retention

tmux panes close when their process exits unless configured otherwise. For workers, we
want the pane to stay alive after exit so we can capture output and show exit status.

```go
// wrapCommandForRetention wraps a command so the pane stays alive after exit.
// Uses tmux's remain-on-exit option or a shell wrapper.
func (b *TmuxBackend) wrapForRetention(cmd []string) []string {
    // Option A: Use tmux set-option remain-on-exit (set per-pane after spawn)
    // This is cleaner but requires a second tmux call after spawn.

    // Option B: Shell wrapper that preserves exit code
    // sh -c '<cmd>; EXIT=$?; echo "\n[exited: $EXIT]"; exec cat'
    // The trailing `exec cat` keeps the pane alive indefinitely.
    // The echo provides a visual marker in the scrollback.
    shellCmd := strings.Join(cmd, " ")
    return []string{
        "sh", "-c",
        fmt.Sprintf(`%s; EXIT=$?; echo ""; echo "[exited with code: $EXIT]"; exec cat`, shellCmd),
    }
}

// Option A implementation (cleaner):
func (b *TmuxBackend) setRemainOnExit(ctx context.Context, paneID string) error {
    _, err := b.tmux.run(ctx, "set-option", "-t", paneID, "remain-on-exit", "on")
    return err
}
```

**Recommendation**: Use `remain-on-exit` (Option A) when targeting tmux 2.6+ where
per-pane options are supported. Fall back to the shell wrapper for broader compatibility.

---

## Parking Window

The parking window is a hidden tmux window that holds all non-visible worker panes.
Workers parked here are still alive — their processes keep running, their PTYs keep
buffering. They're just not rendered on screen.

### Creation

```go
// initParkingWindow creates the hidden window for parking worker panes.
// Called once during TmuxBackend initialization.
func (b *TmuxBackend) initParkingWindow() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        windowID, err := b.tmux.NewWindow(ctx, tmux.NewWindowOpts{
            Detached: true,       // don't switch to it
            Name:     "kasmos-parking",
        })
        if err != nil {
            return parkingInitFailedMsg{err: err}
        }

        return parkingInitMsg{windowID: windowID}
    }
}
```

### Why a Hidden Window?

- **`new-window -d`** creates the window without switching to it. It exists but the user
  never sees it unless they manually `select-window`.
- **`join-pane -d -s <pane> -t <parking_window>`** moves a pane there without following
  focus. The pane is "parked."
- **`join-pane -s <parking>:<pane> -t <kasmos_window>`** brings it back.

Alternative considered: Using `break-pane` + `join-pane` instead of just `join-pane`.
`join-pane` alone handles both directions, so there's no need for `break-pane`.

### Parking Window Lifecycle

```
TmuxBackend.Init()
    │
    ├── display-message -p '#{pane_id}'     → dashboardPaneID
    ├── display-message -p '#{window_id}'   → kasmosWindowID
    └── new-window -d -n kasmos-parking     → parkingWindowID

Worker spawn:
    ├── if visiblePaneID != "":
    │       join-pane -d -s <visible> -t <parking>   (park current)
    └── split-window -h -t <dashboard> <cmd>          (show new)

Worker swap:
    ├── join-pane -d -s <visible> -t <parking>        (park current)
    ├── join-pane -h -s <new_pane> -t <kasmos_window> (show new)
    └── select-pane -t <dashboard> OR <new_pane>      (focus choice)

Shutdown:
    └── kill all worker panes, then kill parking window
```

---

## Visibility Swapping

The swap operation is the critical UX path. It must be fast (< 100ms) and reliable.

### Swap Implementation

```go
// swapVisibleWorker parks the current visible worker and shows a different one.
func (b *TmuxBackend) swapVisibleWorker(newWorkerID string) tea.Cmd {
    return func() tea.Msg {
        newPaneID, ok := b.workerPanes[newWorkerID]
        if !ok {
            return swapFailedMsg{err: fmt.Errorf("worker %s has no pane", newWorkerID)}
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        // Step 1: Park current visible worker (if any)
        if b.visiblePaneID != "" && b.visiblePaneID != newPaneID {
            err := b.tmux.JoinPane(ctx, tmux.JoinOpts{
                Source:   b.visiblePaneID,
                Target:   b.parkingWindowID,
                Detached: true,
            })
            if err != nil && !tmux.IsNotFound(err) {
                return swapFailedMsg{err: fmt.Errorf("park current: %w", err)}
            }
        }

        // Step 2: Show new worker beside dashboard
        err := b.tmux.JoinPane(ctx, tmux.JoinOpts{
            Source:     newPaneID,
            Target:     b.kasmosWindowID,
            Horizontal: true,
            Size:       "50%",
        })
        if err != nil {
            return swapFailedMsg{err: fmt.Errorf("show worker: %w", err)}
        }

        // Step 3: Return focus to dashboard
        _ = b.tmux.SelectPane(ctx, b.dashboardPaneID)

        return workerSwappedMsg{
            workerID:      newWorkerID,
            paneID:        newPaneID,
            previousPaneID: b.visiblePaneID,
        }
    }
}
```

### Edge Cases in Swapping

| Scenario | Handling |
|----------|----------|
| No current visible worker | Skip park step, just show new |
| Current visible pane already dead | Park silently fails (IsNotFound), continue showing new |
| New pane is already visible | No-op, return success |
| New pane was killed while parked | Show fails, return error msg to TUI |
| Terminal too small for split | `IsNoSpace` error, inform user to resize |
| Swap to same worker | Detect `visiblePaneID == newPaneID`, skip |

### Focus Transfer to Worker

When the user wants to *interact* with the worker (type into it), they need tmux focus
transferred to the worker pane:

```go
// focusWorkerPane transfers keyboard input to the visible worker pane.
// The user returns to the dashboard via tmux prefix (Ctrl-b) + arrow/select.
func (b *TmuxBackend) focusWorkerPane() tea.Cmd {
    return func() tea.Msg {
        if b.visiblePaneID == "" {
            return focusFailedMsg{err: fmt.Errorf("no visible worker")}
        }
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()

        err := b.tmux.SelectPane(ctx, b.visiblePaneID)
        if err != nil {
            return focusFailedMsg{err: err}
        }
        return workerFocusedMsg{paneID: b.visiblePaneID}
    }
}
```

**Important**: Once focus transfers to the worker pane, bubbletea stops receiving
keystrokes. The user returns to the dashboard via:
- tmux prefix key (`Ctrl-b`) then arrow/pane-select
- Or a kasmos-specific tmux keybinding (configurable)

This should be clearly communicated in the TUI's status bar.

---

## Status Polling

tmux has no event push mechanism for pane lifecycle changes. Poll with `list-panes` on
a timer.

### Poll Loop Design

```go
const pollInterval = 1 * time.Second

// pollTick is the bubbletea tick message for tmux status polling.
type pollTickMsg time.Time

func pollTickCmd() tea.Cmd {
    return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
        return pollTickMsg(t)
    })
}

// pollPaneStatus queries tmux for current pane states.
func (b *TmuxBackend) pollPaneStatus() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()

        // Poll all panes in the tmux session
        panes, err := b.tmux.ListPanes(ctx, "")
        if err != nil {
            // Session gone — catastrophic, all workers lost
            if tmux.IsSessionGone(err) {
                return sessionLostMsg{err: err}
            }
            return pollErrorMsg{err: err}
        }

        return paneStatusMsg{panes: panes}
    }
}
```

### Processing Poll Results

```go
func (m Model) handlePaneStatus(msg paneStatusMsg) (Model, tea.Cmd) {
    knownPanes := m.backend.KnownPaneIDs() // set of pane IDs we're tracking

    for _, pane := range msg.panes {
        workerID, tracked := m.paneToWorker[pane.PaneID]
        if !tracked {
            continue // not our pane
        }

        worker := m.workers[workerID]

        if pane.Dead && worker.State == WorkerRunning {
            // Worker just exited — state transition
            worker.State = WorkerExited
            worker.ExitCode = pane.DeadStatus
            worker.ExitedAt = time.Now()
            m.workers[workerID] = worker

            // Optionally capture scrollback for session ID extraction
            // (returned as a Cmd, not done inline)
        }
    }

    // Check for panes that disappeared entirely (killed externally)
    for paneID, workerID := range m.paneToWorker {
        found := false
        for _, pane := range msg.panes {
            if pane.PaneID == paneID {
                found = true
                break
            }
        }
        if !found {
            worker := m.workers[workerID]
            if worker.State == WorkerRunning {
                worker.State = WorkerKilled
                worker.ExitedAt = time.Now()
                m.workers[workerID] = worker
            }
        }
    }

    return m, nil
}
```

### Poll Scope

Poll `list-panes` without a specific target (or target the session) to get ALL panes
across all windows (including the parking window). This catches:

- Visible worker pane state changes
- Parked worker pane state changes (worker finished while parked)
- Externally killed panes (user manually ran `tmux kill-pane`)

To list all panes in the session:
```
list-panes -s -F '#{pane_id} #{pane_pid} #{pane_dead} #{pane_dead_status}'
```

The `-s` flag lists panes across ALL windows in the session, not just the current window.
This is essential — without it you'd miss panes in the parking window.

### Differentiation: Dead vs. Missing

| Pane in list-panes? | pane_dead=1? | Meaning |
|---------------------|-------------|---------|
| Yes | No | Running normally |
| Yes | Yes | Process exited, pane retained (remain-on-exit) |
| No | — | Pane completely gone (killed, or remain-on-exit off) |

When a pane disappears from `list-panes`, it means the pane was destroyed — either the
user killed it manually, tmux cleaned it up after process exit (no remain-on-exit), or
something went wrong. Handle this as a `WorkerKilled` state.

---

## Pane Tagging

Environment variables tag tmux panes with kasmos worker IDs. This enables crash recovery.

### Tag Convention

```
Key:   KASMOS_PANE_<worker_id>
Value: <pane_id>

Example:
  KASMOS_PANE_abc123 = %42
  KASMOS_PANE_def456 = %43
```

Tags are set in the tmux **session environment** (not shell environment). They persist
as long as the tmux session lives — surviving kasmos crashes, TUI restarts, detach/attach
cycles.

### Tag Operations

```go
// tagPane associates a worker ID with its tmux pane ID in the session environment.
func (b *TmuxBackend) tagPane(ctx context.Context, workerID, paneID string) error {
    key := fmt.Sprintf("KASMOS_PANE_%s", workerID)
    return b.tmux.SetEnvironment(ctx, key, paneID)
}

// untagPane removes the association when a worker is fully cleaned up.
func (b *TmuxBackend) untagPane(ctx context.Context, workerID string) error {
    key := fmt.Sprintf("KASMOS_PANE_%s", workerID)
    return b.tmux.UnsetEnvironment(ctx, key)
}

// discoverTaggedPanes reads all KASMOS_PANE_* env vars and returns worker->pane mapping.
func (b *TmuxBackend) discoverTaggedPanes(ctx context.Context) (map[string]string, error) {
    env, err := b.tmux.ShowEnvironment(ctx)
    if err != nil {
        return nil, err
    }

    result := make(map[string]string)
    const prefix = "KASMOS_PANE_"
    for key, value := range env {
        if strings.HasPrefix(key, prefix) {
            workerID := key[len(prefix):]
            result[workerID] = value
        }
    }
    return result, nil
}
```

### Additional Metadata Tags

Beyond pane-to-worker mapping, you can store other kasmos metadata in tmux env vars:

```
KASMOS_SESSION_ID=<kasmos_session_uuid>   # Which kasmos session owns these panes
KASMOS_PARKING=<parking_window_id>        # Parking window ID for rediscovery
KASMOS_DASHBOARD=<dashboard_pane_id>      # Dashboard pane for layout recovery
```

These enable a restarting kasmos to fully reconstruct its state from the tmux session.

---

## Content Capture

`capture-pane` grabs the full scrollback buffer from a pane. Used for:

1. **Session ID extraction**: After a worker exits, capture output and regex for the
   OpenCode/claude-code session ID for continuations.
2. **Post-mortem analysis**: Feed captured output to the AI failure analysis helper.
3. **Output preview**: Show the last N lines of a parked worker in the dashboard viewport.

### Capture Implementation

```go
// captureWorkerOutput grabs the full scrollback from a worker pane.
func (b *TmuxBackend) captureWorkerOutput(workerID string) tea.Cmd {
    return func() tea.Msg {
        paneID, ok := b.workerPanes[workerID]
        if !ok {
            return captureFailedMsg{workerID: workerID, err: fmt.Errorf("no pane for worker")}
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        content, err := b.tmux.CapturePane(ctx, paneID)
        if err != nil {
            return captureFailedMsg{workerID: workerID, err: err}
        }

        return outputCapturedMsg{
            workerID: workerID,
            content:  content,
        }
    }
}
```

### Session ID Extraction

```go
import "regexp"

// OpenCode session IDs look like: session_01HXYZ...
var openCodeSessionRe = regexp.MustCompile(`session_[0-9A-Za-z]{20,}`)

// Claude Code session IDs may differ — adjust pattern as needed
var claudeCodeSessionRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// extractSessionID finds the session ID in captured pane output.
func extractSessionID(content string) string {
    // Try OpenCode pattern first
    if match := openCodeSessionRe.FindString(content); match != "" {
        return match
    }
    // Try Claude Code pattern
    if match := claudeCodeSessionRe.FindString(content); match != "" {
        return match
    }
    return ""
}
```

### Capture Timing

Capture is best done **immediately after detecting pane death** in the poll loop:

```go
// In handlePaneStatus, when a worker exits:
if pane.Dead && worker.State == WorkerRunning {
    worker.State = WorkerExited
    worker.ExitCode = pane.DeadStatus
    m.workers[workerID] = worker

    // Trigger scrollback capture for session ID extraction
    cmds = append(cmds, b.captureWorkerOutput(workerID))
}
```

With `remain-on-exit` enabled, the dead pane retains its scrollback until explicitly
killed. Without it, capture must happen before the pane disappears.

### Output Preview for Dashboard

For the bubbletea dashboard to show a preview of a worker's output (even when parked):

```go
// captureLastNLines grabs the tail of a pane's scrollback for preview.
func (b *TmuxBackend) captureLastNLines(paneID string, n int) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()

        // -S -<n> starts capture from n lines before the end
        content, err := b.tmux.run(ctx, "capture-pane", "-p", "-t", paneID,
            "-S", fmt.Sprintf("-%d", n))
        if err != nil {
            return previewFailedMsg{paneID: paneID, err: err}
        }
        return previewCapturedMsg{paneID: paneID, content: content}
    }
}
```

---

## Crash Recovery

kasmos crashes. tmux keeps the panes alive. When kasmos restarts, it needs to rediscover
and reclaim its panes.

### Recovery Sequence

```go
// recoverFromCrash attempts to find existing kasmos panes in the tmux session.
func (b *TmuxBackend) recoverFromCrash() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        // 1. Discover tagged panes
        workerPanes, err := b.discoverTaggedPanes(ctx)
        if err != nil {
            return recoveryFailedMsg{err: err}
        }

        if len(workerPanes) == 0 {
            return noRecoveryNeededMsg{}
        }

        // 2. Get actual pane list to validate tags
        allPanes, err := b.tmux.ListPanes(ctx, "")
        if err != nil {
            return recoveryFailedMsg{err: err}
        }

        activePaneIDs := make(map[string]tmux.PaneInfo)
        for _, p := range allPanes {
            activePaneIDs[p.PaneID] = p
        }

        // 3. Reconcile: tag exists but pane doesn't = stale tag
        recovered := make(map[string]RecoveredWorker)
        var staleTags []string

        for workerID, paneID := range workerPanes {
            paneInfo, exists := activePaneIDs[paneID]
            if !exists {
                staleTags = append(staleTags, workerID)
                continue
            }

            recovered[workerID] = RecoveredWorker{
                WorkerID: workerID,
                PaneID:   paneID,
                Dead:     paneInfo.Dead,
                ExitCode: paneInfo.DeadStatus,
            }
        }

        // 4. Clean up stale tags
        for _, workerID := range staleTags {
            _ = b.untagPane(ctx, workerID)
        }

        // 5. Discover parking window
        parkingID := ""
        if env, err := b.tmux.ShowEnvironment(ctx); err == nil {
            parkingID = env["KASMOS_PARKING"]
        }

        return recoveryCompleteMsg{
            workers:    recovered,
            parkingID:  parkingID,
            staleTags:  staleTags,
        }
    }
}

type RecoveredWorker struct {
    WorkerID string
    PaneID   string
    Dead     bool
    ExitCode int
}
```

### Recovery at Startup

During `Init()`, the TmuxBackend should:

1. Identify itself: `display-message -p '#{pane_id}'` and `#{window_id}'`
2. Check for existing KASMOS_* environment variables
3. If tags exist, run the recovery sequence
4. If no tags, fresh start — create parking window

```go
func (b *TmuxBackend) Init() tea.Cmd {
    return tea.Batch(
        b.identifySelf(),     // get dashboard pane/window IDs
        b.checkForRecovery(), // look for existing tagged panes
    )
}
```

---

## Focus Management

tmux's `select-pane` moves keyboard focus between panes. In the kasmos context:

### Focus States

| Focus State | Who Receives Keystrokes | How User Got Here |
|------------|------------------------|-------------------|
| Dashboard | bubbletea (kasmos TUI) | Default. After spawn, swap, or returning from worker |
| Worker | The worker process PTY | User pressed "enter worker" key in dashboard |

### Returning to Dashboard

When focus is on a worker pane, the user needs an escape hatch back to the dashboard.
Options:

1. **tmux prefix key** (`Ctrl-b` then arrow or pane number) — always works, no kasmos
   involvement
2. **Custom tmux keybinding** — bind a key in tmux.conf to `select-pane -t <dashboard>`.
   kasmos can set this up during init:

```go
// bindReturnKey configures a tmux keybinding to return focus to the dashboard.
// Example: Ctrl-b + d selects the dashboard pane
func (b *TmuxBackend) bindReturnKey(ctx context.Context) error {
    _, err := b.tmux.run(ctx, "bind-key", "-n", "M-d",
        "select-pane", "-t", b.dashboardPaneID)
    return err
}
```

3. **Status bar hint**: The bubbletea status bar should show how to return:
   `"Focus: worker | Ctrl-b ← to return"` or `"Alt-d: dashboard"`

---

## Graceful Shutdown

When kasmos exits (user quits or SIGTERM), clean up tmux resources.

### Shutdown Sequence

```go
// shutdown cleans up all kasmos tmux resources.
func (b *TmuxBackend) shutdown() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        // 1. Kill all worker panes
        for workerID, paneID := range b.workerPanes {
            _ = b.tmux.KillPane(ctx, paneID)
            _ = b.untagPane(ctx, workerID)
        }

        // 2. Kill parking window
        if b.parkingWindowID != "" {
            _, _ = b.tmux.run(ctx, "kill-window", "-t", b.parkingWindowID)
        }

        // 3. Clean up kasmos-specific env vars
        _ = b.tmux.UnsetEnvironment(ctx, "KASMOS_SESSION_ID")
        _ = b.tmux.UnsetEnvironment(ctx, "KASMOS_PARKING")
        _ = b.tmux.UnsetEnvironment(ctx, "KASMOS_DASHBOARD")

        return shutdownCompleteMsg{}
    }
}
```

### Detach vs. Kill Shutdown Modes

For long-running orchestration where workers should survive kasmos exit:

```go
// detach leaves workers running in tmux but removes kasmos TUI.
// Workers can be reclaimed by a future kasmos --attach.
func (b *TmuxBackend) detach() tea.Cmd {
    return func() tea.Msg {
        // Park the visible worker (if any) so it doesn't depend on the dashboard pane
        if b.visiblePaneID != "" {
            ctx := context.Background()
            _ = b.tmux.JoinPane(ctx, tmux.JoinOpts{
                Source:   b.visiblePaneID,
                Target:   b.parkingWindowID,
                Detached: true,
            })
        }

        // Don't kill anything — just exit. Tags remain for recovery.
        return detachCompleteMsg{}
    }
}
```

The kasmos session persistence file should record whether the last exit was "clean"
(kill) or "detach" so the next startup knows whether to attempt recovery.
