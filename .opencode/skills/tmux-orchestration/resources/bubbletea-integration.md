# bubbletea Integration — Messages, Commands, Backend Interface

## Table of Contents
- [Messages](#messages)
- [Command Patterns](#command-patterns)
- [Backend Interface](#backend-interface)
- [TmuxBackend](#tmuxbackend)
- [Mode Selection](#mode-selection)
- [Poll Integration](#poll-integration)
- [State Machine Integration](#state-machine-integration)
- [Model Wiring](#model-wiring)

---

## Messages

Every tmux operation result flows through bubbletea's message system. Define typed
messages for each operation outcome.

### Message Catalog

```go
package worker

import (
    "time"
    "github.com/your/project/internal/tmux"
)

// ── Initialization ──

// parkingInitMsg signals the parking window was created.
type parkingInitMsg struct {
    windowID string
}

// parkingInitFailedMsg signals parking window creation failed.
type parkingInitFailedMsg struct {
    err error
}

// selfIdentifiedMsg carries the dashboard's own pane and window IDs.
type selfIdentifiedMsg struct {
    paneID   string
    windowID string
}

// ── Worker Lifecycle ──

// workerSpawnedMsg signals a worker pane was successfully created.
type workerSpawnedMsg struct {
    workerID string
    paneID   string
}

// workerSpawnFailedMsg signals worker pane creation failed.
type workerSpawnFailedMsg struct {
    workerID string
    err      error
}

// workerSwappedMsg signals a worker was swapped to visible.
type workerSwappedMsg struct {
    workerID       string
    paneID         string
    previousPaneID string
}

// swapFailedMsg signals a visibility swap failed.
type swapFailedMsg struct {
    err error
}

// workerFocusedMsg signals keyboard focus was transferred to a worker.
type workerFocusedMsg struct {
    paneID string
}

// focusFailedMsg signals focus transfer failed.
type focusFailedMsg struct {
    err error
}

// workerKilledMsg signals a worker pane was killed by user action.
type workerKilledMsg struct {
    workerID string
}

// ── Polling ──

// pollTickMsg triggers a status poll cycle.
type pollTickMsg time.Time

// paneStatusMsg carries fresh pane state from tmux.
type paneStatusMsg struct {
    panes []tmux.PaneInfo
}

// pollErrorMsg signals a poll cycle failed.
type pollErrorMsg struct {
    err error
}

// sessionLostMsg signals the tmux session is gone (catastrophic).
type sessionLostMsg struct {
    err error
}

// ── Content Capture ──

// outputCapturedMsg carries captured scrollback from a worker pane.
type outputCapturedMsg struct {
    workerID string
    content  string
}

// captureFailedMsg signals scrollback capture failed.
type captureFailedMsg struct {
    workerID string
    err      error
}

// previewCapturedMsg carries a preview snippet for the dashboard.
type previewCapturedMsg struct {
    paneID  string
    content string
}

// ── Recovery ──

// recoveryCompleteMsg carries the results of crash recovery.
type recoveryCompleteMsg struct {
    workers   map[string]RecoveredWorker
    parkingID string
    staleTags []string
}

// recoveryFailedMsg signals crash recovery failed.
type recoveryFailedMsg struct {
    err error
}

// noRecoveryNeededMsg signals no existing panes were found.
type noRecoveryNeededMsg struct{}

// ── Shutdown ──

// shutdownCompleteMsg signals all tmux resources are cleaned up.
type shutdownCompleteMsg struct{}

// detachCompleteMsg signals kasmos detached without killing workers.
type detachCompleteMsg struct{}
```

### Message Design Rules

1. **One message per outcome**: Success and failure are separate types. This makes
   `Update` switch cases clean and exhaustive.
2. **Carry enough context**: Include the workerID/paneID so `Update` knows which worker
   the message applies to without maintaining side channels.
3. **Errors in messages, not panics**: tmux failures become error messages, never panics.
   The TUI can display them gracefully.
4. **No methods on messages**: Messages are plain data structs. Logic lives in `Update`.

---

## Command Patterns

All tmux operations are `tea.Cmd` — functions that execute asynchronously and return
a `tea.Msg`. Never call tmux in `Update` directly.

### Pattern: Operation Cmd

```go
// Every tmux operation follows this shape:
func (b *TmuxBackend) someOperation(params...) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), timeout)
        defer cancel()

        result, err := b.tmux.SomeMethod(ctx, ...)
        if err != nil {
            return someOperationFailedMsg{err: err}
        }
        return someOperationSuccessMsg{result: result}
    }
}
```

### Pattern: Multi-Step Cmd

Some operations require multiple tmux calls in sequence (e.g., swap = park + show +
focus). These are fine as a single Cmd because the entire sequence is a single atomic
user operation:

```go
func (b *TmuxBackend) swapVisibleWorker(newWorkerID string) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        // Step 1: park
        if b.visiblePaneID != "" {
            err := b.tmux.JoinPane(ctx, ...)
            if err != nil && !tmux.IsNotFound(err) {
                return swapFailedMsg{err: err}
            }
        }

        // Step 2: show
        err := b.tmux.JoinPane(ctx, ...)
        if err != nil {
            return swapFailedMsg{err: err}
        }

        // Step 3: focus
        _ = b.tmux.SelectPane(ctx, b.dashboardPaneID)

        return workerSwappedMsg{...}
    }
}
```

### Pattern: Chained Cmds via Update

When operations depend on each other's results but should update the Model between
steps, chain through `Update`:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case workerSpawnedMsg:
        // Step 1 complete: worker spawned
        m.workers[msg.workerID].PaneID = msg.paneID
        m.backend.visiblePaneID = msg.paneID

        // Chain step 2: capture preview after a short delay
        return m, tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
            return capturePreviewMsg{workerID: msg.workerID}
        })

    case capturePreviewMsg:
        // Step 2: trigger preview capture
        return m, m.backend.captureLastNLines(m.workers[msg.workerID].PaneID, 20)
    }
}
```

### Pattern: Batch Init

Startup requires multiple concurrent tmux operations:

```go
func (b *TmuxBackend) Init() tea.Cmd {
    return tea.Batch(
        b.identifySelf(),       // display-message for pane/window IDs
        b.checkForRecovery(),   // look for KASMOS_* env vars
        pollTickCmd(),          // start the poll loop
    )
}
```

`tea.Batch` runs all Cmds concurrently. Results arrive as separate messages.

---

## Backend Interface

The `WorkerBackend` interface abstracts worker management. kasmos supports two
implementations: `SubprocessBackend` (headless, pipe-captured) and `TmuxBackend`
(PTY, pane-managed). They are **mutually exclusive** — a kasmos session uses one or
the other.

### Interface Definition

```go
package backend

import tea "github.com/charmbracelet/bubbletea"

// WorkerState represents the lifecycle state of a worker.
type WorkerState int

const (
    WorkerPending WorkerState = iota
    WorkerSpawning
    WorkerRunning
    WorkerExited
    WorkerKilled
)

// WorkerInfo carries the current state of a worker for display.
type WorkerInfo struct {
    ID             string
    State          WorkerState
    ExitCode       int
    SessionID      string // OpenCode/claude-code session for continuation
    ParentWorkerID string // for continuation chains
}

// SpawnOpts configures a new worker.
type SpawnOpts struct {
    WorkerID        string
    Agent           string   // role: planner, coder, reviewer, release
    Prompt          string
    Model           string   // model override
    Files           []string // file attachments
    ContinueSession string   // session ID for continuation
    Backend         string   // "opencode" or "claude-code"
}

// WorkerBackend abstracts worker lifecycle management.
type WorkerBackend interface {
    // Init returns the startup Cmd (setup parking window, discover panes, etc.)
    Init() tea.Cmd

    // Spawn creates a new worker with the given options.
    Spawn(opts SpawnOpts) tea.Cmd

    // Kill terminates a running worker.
    Kill(workerID string) tea.Cmd

    // SwapVisible shows a different worker in the visible position.
    // Only meaningful for TmuxBackend; SubprocessBackend is a no-op.
    SwapVisible(workerID string) tea.Cmd

    // FocusWorker transfers keyboard input to the visible worker.
    // Only meaningful for TmuxBackend; SubprocessBackend returns an error.
    FocusWorker() tea.Cmd

    // FocusDashboard returns keyboard input to the kasmos TUI.
    // Only meaningful for TmuxBackend; SubprocessBackend is a no-op.
    FocusDashboard() tea.Cmd

    // CaptureOutput grabs the worker's output for display or analysis.
    CaptureOutput(workerID string) tea.Cmd

    // Poll returns a Cmd that checks worker status and returns paneStatusMsg
    // or equivalent. Called on each tick.
    Poll() tea.Cmd

    // Shutdown cleans up all backend resources.
    Shutdown() tea.Cmd

    // Detach disconnects kasmos without killing workers (for reattach).
    // SubprocessBackend may not support this.
    Detach() tea.Cmd

    // Interactive returns true if this backend supports worker interaction
    // (focus transfer, PTY passthrough). TmuxBackend=true, Subprocess=false.
    Interactive() bool

    // Update processes backend-specific messages and returns updated backend state.
    // This is called from the main Model.Update for messages the backend owns.
    Update(msg tea.Msg) tea.Cmd
}
```

### Interactive() Discrimination

The `Interactive()` method is the key discriminator between backends. The TUI uses it
to conditionally show interactive features:

```go
func (m Model) View() string {
    // ...worker list, status bar...

    if m.backend.Interactive() {
        // Show focus transfer hint and interactive keybinds
        statusItems = append(statusItems, "enter: interact with worker")
        statusItems = append(statusItems, "Ctrl-b ←: return to dashboard")
    } else {
        // Show output viewport keybinds (scroll, search)
        statusItems = append(statusItems, "↑/↓: scroll output")
    }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if msg.String() == "enter" && m.backend.Interactive() {
            return m, m.backend.FocusWorker()
        }
    }
}
```

### Keybind Differences by Backend

| Action | SubprocessBackend | TmuxBackend |
|--------|------------------|-------------|
| Select worker | Switches output viewport | Swaps visible pane |
| Enter on worker | No-op (or expand output) | Focus worker pane (interact) |
| Kill worker | Send SIGTERM to process | `kill-pane` |
| View output | Stream from pipe buffer | `capture-pane` or live pane view |
| Continue session | Extract session ID from pipe | Extract from `capture-pane` |

---

## TmuxBackend

The `TmuxBackend` struct holds all tmux-specific state.

### Structure

```go
package backend

import (
    "sync"
    "github.com/your/project/internal/tmux"
)

type TmuxBackend struct {
    tmux tmux.TmuxClient

    // Identity (set during Init)
    dashboardPaneID string
    kasmosWindowID  string
    parkingWindowID string

    // Worker tracking
    mu             sync.RWMutex
    workerPanes    map[string]string // workerID -> paneID
    visiblePaneID  string            // currently visible worker pane (or "")
    visibleWorkerID string           // currently visible worker ID (or "")

    // Configuration
    splitSize      string // e.g., "50%"
    pollInterval   time.Duration
}

func NewTmuxBackend(client tmux.TmuxClient) *TmuxBackend {
    return &TmuxBackend{
        tmux:         client,
        workerPanes:  make(map[string]string),
        splitSize:    "50%",
        pollInterval: 1 * time.Second,
    }
}
```

### Init Sequence

```go
func (b *TmuxBackend) Init() tea.Cmd {
    return tea.Batch(
        b.identifySelf(),
        b.initParkingWindow(),
        pollTickCmd(b.pollInterval),
    )
}

func (b *TmuxBackend) identifySelf() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        paneID, err := b.tmux.DisplayMessage(ctx, "#{pane_id}")
        if err != nil {
            return initFailedMsg{err: fmt.Errorf("identify pane: %w", err)}
        }
        windowID, err := b.tmux.DisplayMessage(ctx, "#{window_id}")
        if err != nil {
            return initFailedMsg{err: fmt.Errorf("identify window: %w", err)}
        }
        return selfIdentifiedMsg{paneID: paneID, windowID: windowID}
    }
}

func (b *TmuxBackend) Interactive() bool { return true }
```

### Update Handler

The backend's `Update` processes messages it owns and returns follow-up Cmds:

```go
func (b *TmuxBackend) Update(msg tea.Msg) tea.Cmd {
    switch msg := msg.(type) {
    case selfIdentifiedMsg:
        b.dashboardPaneID = msg.paneID
        b.kasmosWindowID = msg.windowID
        // Tag ourselves
        return func() tea.Msg {
            ctx := context.Background()
            _ = b.tmux.SetEnvironment(ctx, "KASMOS_DASHBOARD", msg.paneID)
            return nil
        }

    case parkingInitMsg:
        b.parkingWindowID = msg.windowID
        return func() tea.Msg {
            ctx := context.Background()
            _ = b.tmux.SetEnvironment(ctx, "KASMOS_PARKING", msg.windowID)
            return nil
        }

    case workerSpawnedMsg:
        b.mu.Lock()
        b.workerPanes[msg.workerID] = msg.paneID
        b.visiblePaneID = msg.paneID
        b.visibleWorkerID = msg.workerID
        b.mu.Unlock()
        return nil

    case workerSwappedMsg:
        b.mu.Lock()
        b.visiblePaneID = msg.paneID
        b.visibleWorkerID = msg.workerID
        b.mu.Unlock()
        return nil

    case pollTickMsg:
        return tea.Batch(
            b.pollPaneStatus(),
            pollTickCmd(b.pollInterval), // re-arm the tick
        )
    }
    return nil
}
```

---

## Mode Selection

kasmos decides which backend to use at startup based on environment and flags.

### Detection Logic

```go
package backend

import "os"

// SelectBackend determines which WorkerBackend to use.
func SelectBackend(forceTmux, forceSubprocess bool) (WorkerBackend, error) {
    // Explicit flags take precedence
    if forceSubprocess {
        return NewSubprocessBackend(), nil
    }
    if forceTmux {
        if !inTmux() {
            return nil, fmt.Errorf("--tmux-mode requires running inside tmux")
        }
        client := &tmux.ExecClient{}
        return NewTmuxBackend(client), nil
    }

    // Auto-detect: use tmux if we're inside a tmux session
    if inTmux() {
        client := &tmux.ExecClient{}
        return NewTmuxBackend(client), nil
    }

    // Default: subprocess mode
    return NewSubprocessBackend(), nil
}

// inTmux checks if we're running inside a tmux session.
func inTmux() bool {
    return os.Getenv("TMUX") != ""
}
```

### CLI Flags

```go
// In main.go or cmd setup:
var (
    flagTmuxMode      bool // --tmux: force tmux backend
    flagSubprocessMode bool // --subprocess: force subprocess backend
    flagDaemon         bool // -d: headless mode (always subprocess)
)

func resolveBackend() (WorkerBackend, error) {
    if flagDaemon {
        // Daemon mode always uses subprocess (no panes to show)
        return NewSubprocessBackend(), nil
    }
    return SelectBackend(flagTmuxMode, flagSubprocessMode)
}
```

### Mode Announcement

On startup, kasmos should indicate which backend is active:

```go
func modeLabel(b WorkerBackend) string {
    if b.Interactive() {
        return "tmux (interactive)"
    }
    return "subprocess (headless)"
}
```

Display this in the TUI header or status bar so the user knows what mode they're in.

---

## Poll Integration

The poll loop is the heartbeat of the tmux backend — it runs on every tick and
detects worker state changes.

### Wiring Into Model.Update

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    // Always let the backend process its own messages first
    if backendCmd := m.backend.Update(msg); backendCmd != nil {
        cmds = append(cmds, backendCmd)
    }

    switch msg := msg.(type) {
    case paneStatusMsg:
        m, additionalCmds := m.handlePaneStatus(msg)
        cmds = append(cmds, additionalCmds...)

    case outputCapturedMsg:
        worker := m.workers[msg.workerID]
        worker.CapturedOutput = msg.content
        worker.SessionID = extractSessionID(msg.content)
        m.workers[msg.workerID] = worker

    case sessionLostMsg:
        // tmux session is gone — show error, offer to restart
        m.fatalError = msg.err
        m.showErrorOverlay = true

    case pollErrorMsg:
        // Non-fatal poll error — log and continue
        m.lastPollError = msg.err
    }

    return m, tea.Batch(cmds...)
}
```

### Poll Frequency Tuning

1 second is the default. Adjust based on use case:

| Scenario | Interval | Rationale |
|----------|----------|-----------|
| Normal operation | 1s | Responsive without excessive tmux calls |
| Many workers (10+) | 2s | Reduce tmux CLI overhead |
| Single critical worker | 500ms | Faster feedback for the user |
| Daemon mode | N/A | Subprocess backend uses process wait, not polling |

The interval is configurable via `TmuxBackend.pollInterval`.

---

## State Machine Integration

Worker state transitions in tmux mode need to account for the pane lifecycle:

```
                    ┌──────────┐
                    │ Pending  │  (task selected, not yet spawned)
                    └────┬─────┘
                         │ user presses spawn
                    ┌────▼─────┐
                    │ Spawning │  (split-window executing)
                    └────┬─────┘
                         │ workerSpawnedMsg
                    ┌────▼─────┐
              ┌─────│ Running  │──────┐
              │     └────┬─────┘      │
              │          │            │
       poll: dead   poll: missing   user: kill
       exitCode=N     pane gone    kill-pane
              │          │            │
        ┌─────▼──┐  ┌───▼────┐  ┌───▼────┐
        │ Exited │  │ Lost   │  │ Killed │
        │(code N)│  │(crash?)│  │(user)  │
        └───┬────┘  └────────┘  └───┬────┘
            │                       │
            │ user: continue        │ user: restart
            │ (capture → sessionID) │ (re-spawn with prompt)
            │                       │
            └──────────┬────────────┘
                  ┌────▼─────┐
                  │ Spawning │ (new worker, possibly --continue)
                  └──────────┘
```

The `Lost` state is tmux-specific — it means the pane disappeared from `list-panes`
without kasmos killing it. This could mean the user manually killed the pane, or
tmux had an issue. The TUI should surface this clearly.

---

## Model Wiring

### Top-Level Model Structure

```go
type Model struct {
    // Backend (one of TmuxBackend or SubprocessBackend)
    backend WorkerBackend

    // Worker state (backend-agnostic)
    workers       map[string]*Worker
    workerOrder   []string // display order
    selectedIdx   int

    // Pane mapping (tmux-specific, empty for subprocess)
    paneToWorker map[string]string // paneID -> workerID

    // UI state
    focused       panel
    width, height int
    ready         bool
    // ... other UI fields from charm-tui patterns

    // Sub-models
    table     table.Model
    viewport  viewport.Model
    help      help.Model
    spinner   spinner.Model
    // etc.
}
```

### Backend Message Routing

In `Update`, route messages through the backend FIRST, then handle them in the model:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    // 1. Backend gets first crack at the message
    if cmd := m.backend.Update(msg); cmd != nil {
        cmds = append(cmds, cmd)
    }

    // 2. Model handles the message for UI updates
    switch msg := msg.(type) {
    case workerSpawnedMsg:
        m.workers[msg.workerID].State = WorkerRunning
        m.workers[msg.workerID].PaneID = msg.paneID
        m.paneToWorker[msg.paneID] = msg.workerID
        m.updateTable()

    case workerSwappedMsg:
        m.visibleWorkerID = msg.workerID
        m.updateStatusBar()

    case paneStatusMsg:
        m, additionalCmds := m.handlePaneStatus(msg)
        cmds = append(cmds, additionalCmds...)

    case tea.KeyMsg:
        // Only process keys when dashboard has focus
        cmds = append(cmds, m.handleKey(msg))

    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.updateLayout()
    }

    // 3. Update sub-models
    // ... delegate to table, viewport, etc. as per charm-tui patterns

    return m, tea.Batch(cmds...)
}
```

### Key Handler with Backend Awareness

```go
func (m Model) handleKey(msg tea.KeyMsg) tea.Cmd {
    switch msg.String() {
    case "s":
        // Spawn — same for both backends
        return m.showSpawnDialog()

    case "k":
        // Kill selected worker
        wID := m.selectedWorkerID()
        return m.backend.Kill(wID)

    case "enter":
        if m.backend.Interactive() {
            // tmux: swap to selected worker and optionally focus
            wID := m.selectedWorkerID()
            return m.backend.SwapVisible(wID)
        }
        // subprocess: toggle expanded output view
        return m.toggleOutputExpand()

    case "i":
        if m.backend.Interactive() {
            // Transfer keyboard focus to worker
            return m.backend.FocusWorker()
        }

    case "c":
        // Continue completed worker
        wID := m.selectedWorkerID()
        return tea.Batch(
            m.backend.CaptureOutput(wID), // capture for session ID
            m.showContinueDialog(wID),
        )

    case "q":
        return m.backend.Shutdown()
    }
    return nil
}
```

This pattern ensures the TUI gracefully adapts to whichever backend is active,
with interactive features conditionally enabled based on `Interactive()`.
