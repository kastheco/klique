---
name: tmux-orchestration
description: >
  Use tmux as a process/pane orchestration layer underneath a bubbletea TUI. This
  skill covers spawning and managing worker panes in tmux from Go, the hidden-window
  parking pattern for single-visible-pane UX, poll-based status detection via tmux
  format strings, environment-based pane tagging for crash-resilient rediscovery,
  content capture from pane scrollback, and integrating all of this into bubbletea's
  Elm architecture via tea.Cmd. Trigger when the user mentions tmux pane management
  from Go, worker pane orchestration, tmux-backed process management, or is implementing
  the kasmos TmuxBackend. Also trigger when the user needs to programmatically control
  tmux from a bubbletea application — spawning panes, swapping visibility, polling
  status, capturing output, or surviving crashes via pane rediscovery. This skill is
  complementary to the charm-tui skill: charm-tui handles TUI design/styling, this
  skill handles the tmux substrate beneath it.
---

# tmux Orchestration Skill

Use tmux as a **process and pane orchestration layer** underneath a bubbletea TUI. This
is not about building a TUI inside tmux — it's about driving tmux programmatically from
Go to manage worker processes that need real PTYs, while a bubbletea dashboard retains
orchestration control.

## How to Use This Skill

1. **Always** read this SKILL.md first for the mental model, architecture, and core
   patterns
2. Read `references/tmux-cli-wrapper.md` for the Go wrapper layer — `os/exec` patterns,
   error handling, format string parsing, and the `TmuxClient` interface
3. Read `references/pane-orchestration.md` for the pane lifecycle — spawn, park, swap,
   focus, kill, poll, tag, capture — the full operational playbook
4. Read `references/bubbletea-integration.md` for wiring tmux operations into bubbletea's
   Elm architecture — message types, Cmd patterns, the `WorkerBackend` interface, and
   the mutual exclusivity design with subprocess mode

For implementing the full TmuxBackend, read ALL reference files. For isolated tmux
operations (just spawning a pane, just polling), read the relevant one.

**This skill pairs with `charm-tui`**: charm-tui handles TUI design, styling, component
composition, and UX. This skill handles what's *underneath* — the tmux operations that
give workers real terminals. Use both skills together when building the kasmos TmuxBackend.

## Mental Model

```
┌─────────────────────────────────────────────────────────────┐
│ tmux session                                                │
│                                                             │
│  ┌─────────────────────────┬──────────────────────────┐     │
│  │ kasmos dashboard (pane) │ active worker pane        │     │
│  │                         │                           │     │
│  │  bubbletea TUI renders  │  opencode/claude-code     │     │
│  │  here — worker table,   │  runs here with full PTY  │     │
│  │  output preview, status │  (colors, cursor, input)  │     │
│  │                         │                           │     │
│  └─────────────────────────┴──────────────────────────┘     │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ kasmos-parking (hidden window)                       │    │
│  │                                                      │    │
│  │  worker-2 (parked)  worker-3 (parked)  worker-N ... │    │
│  │  still alive, just  their PTYs keep    swap in when │    │
│  │  not visible        running            user selects │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**Key insight**: Only ONE worker pane is visible at a time beside the dashboard. All
others are parked in a hidden tmux window. Swapping is `break-pane` + `join-pane` — under
100ms, instant to the user. Workers stay alive regardless of visibility because tmux owns
the PTYs.

## Architecture Principles

### 1. tmux Is the Process Substrate, Not the UI

The bubbletea TUI owns the user experience — layout, styling, keybinds, navigation.
tmux provides what bubbletea cannot: real PTY allocation for worker processes. The user
interacts with kasmos primarily through the bubbletea dashboard; tmux panes are the
"window" into a selected worker.

### 2. All tmux Operations via `os/exec`

Every tmux interaction is a `tmux` CLI invocation via Go's `os/exec.Command`. No tmux
Go library, no socket protocol, no C bindings. The CLI is the stable, documented, tested
interface. Wrap it cleanly.

### 3. Never Call tmux in Update

bubbletea's `Update` function must be non-blocking and side-effect-free (returns new
Model + Cmd). All tmux operations go in `tea.Cmd` functions that execute asynchronously
and return result messages. This is the same discipline as any bubbletea side effect.

### 4. Poll, Don't Watch

tmux has no event subscription mechanism for pane lifecycle. Instead, poll with
`list-panes` on the existing bubbletea tick timer (1s interval). Parse format strings
for pane state. This is simple, reliable, and adds zero architectural complexity.

### 5. Tag for Crash Resilience

tmux session-level environment variables (`set-environment`) tag panes with kasmos
worker IDs. If kasmos crashes and restarts, it reads these tags to rediscover its
panes. This is the crash resilience story — tmux owns the PTYs, kasmos just needs to
find them again.

### 6. One Visible Worker at a Time

The "parking window" pattern: a hidden tmux window holds all non-visible worker panes.
Swapping workers means parking the current one and joining the new one. This keeps the
layout predictable (dashboard left, worker right) and avoids the complexity of
multi-pane arrangements.

## Core Operations Summary

| Operation | tmux Command | Purpose |
|-----------|-------------|---------|
| Spawn worker | `split-window -h -t <target> -P -F '#{pane_id}' <cmd>` | Create worker pane beside dashboard |
| Kill worker | `kill-pane -t <pane_id>` | Terminate worker process |
| Focus worker | `select-pane -t <pane_id>` | Transfer keyboard input |
| Create parking | `new-window -d -n kasmos-parking -P -F '#{window_id}'` | Hidden holding area |
| Park worker | `join-pane -d -s <pane_id> -t <parking_window>` | Hide without killing |
| Show worker | `join-pane -s <parking>:<pane> -t <kasmos_window> -h [-l 50%]` | Bring to visible split |
| Poll status | `list-panes -t <target> -F '#{pane_id} #{pane_pid} #{pane_dead} #{pane_dead_status}'` | Detect exits/deaths |
| Tag pane | `set-environment KASMOS_PANE_<id> <worker_id>` | Crash-resilient identity |
| Read tags | `show-environment -t <session>` | Rediscover panes |
| Self-identify | `display-message -p '#{pane_id}'` | Capture dashboard's own pane ID |
| Capture output | `capture-pane -p -t <pane_id> -S -` | Grab scrollback content |
| Version check | `tmux -V` | Advisory minimum (2.6+) |

## Swap Sequence (The Critical Path)

This is the most important operation — swapping which worker is visible:

```
1. Park current visible worker:
   join-pane -d -s %<current_pane_id> -t @<parking_window_id>

2. Show new worker from parking:
   join-pane -h -s %<new_pane_id> -t @<kasmos_window_id>

3. Focus the new worker pane (if user wants input there):
   select-pane -t %<new_pane_id>

   OR focus dashboard (if user wants to keep navigating):
   select-pane -t %<dashboard_pane_id>
```

The `-d` flag on step 1 prevents focus from following the pane into parking. The whole
sequence completes in < 100ms.

## Error Handling Philosophy

tmux commands can fail for many reasons: pane already dead, session gone, window doesn't
exist. The wrapper layer must:

- **Capture stderr** from every `os/exec` call and wrap it into Go errors
- **Distinguish "expected" failures** (pane already dead when killing) from unexpected
  ones (tmux not found, permission denied)
- **Never panic** on tmux failures — return error messages through bubbletea's Msg system
- **Log tmux commands** at debug level for troubleshooting (the exact command string and
  stderr output)

## Minimum tmux Version

The skill targets **tmux 2.6+** for broad compatibility. Key capabilities by version:

| Feature | Minimum Version | Notes |
|---------|----------------|-------|
| `#{pane_dead}` format | 2.0+ | Core polling |
| `#{pane_dead_status}` | 2.4+ | Exit code capture |
| `set-environment` | 1.0+ | Pane tagging |
| `join-pane` / `break-pane` | 1.0+ | Parking pattern |
| `capture-pane -p` | 1.8+ | Scrollback capture |
| `-P -F` on `split-window` | 2.6+ | Pane ID capture on create |

The version check is advisory — warn the user, don't hard-fail. Older tmux may work
with reduced functionality.

## File Organization

When implementing tmux orchestration in a Go project:

```
internal/
├── tmux/
│   ├── client.go          # TmuxClient interface + exec-based implementation
│   ├── client_test.go     # Tests with mock/stub tmux
│   ├── parser.go          # Format string parsing (pane status, env vars)
│   ├── parser_test.go     # Parser unit tests (pure functions, easy to test)
│   └── errors.go          # Typed errors (PaneNotFound, SessionGone, etc.)
├── backend/
│   ├── backend.go         # WorkerBackend interface
│   ├── subprocess.go      # SubprocessBackend (headless, pipe-captured)
│   ├── tmux.go            # TmuxBackend (PTY, pane-managed)
│   └── tmux_test.go       # TmuxBackend tests with mock TmuxClient
└── worker/
    ├── worker.go          # Worker model (state machine, metadata)
    └── messages.go        # Worker-related tea.Msg types
```

Separating `tmux/client.go` from `backend/tmux.go` means the raw tmux operations are
testable independently of the bubbletea integration. The `TmuxClient` interface makes
the whole layer mockable.

## Quick Reference

| Pattern | Reference |
|---------|-----------|
| TmuxClient interface definition | `tmux-cli-wrapper.md` § Interface Design |
| os/exec command construction | `tmux-cli-wrapper.md` § Command Execution |
| stderr capture + error wrapping | `tmux-cli-wrapper.md` § Error Handling |
| Format string parsing | `tmux-cli-wrapper.md` § Parsing |
| Pane spawn with ID capture | `pane-orchestration.md` § Spawning |
| Hidden window parking pattern | `pane-orchestration.md` § Parking Window |
| Swap choreography (park → show) | `pane-orchestration.md` § Visibility Swapping |
| Poll loop with list-panes | `pane-orchestration.md` § Status Polling |
| Environment-based tagging | `pane-orchestration.md` § Pane Tagging |
| Scrollback capture for extraction | `pane-orchestration.md` § Content Capture |
| Crash rediscovery sequence | `pane-orchestration.md` § Crash Recovery |
| tea.Cmd wrappers for tmux ops | `bubbletea-integration.md` § Command Patterns |
| Message types for tmux events | `bubbletea-integration.md` § Messages |
| WorkerBackend interface | `bubbletea-integration.md` § Backend Interface |
| TmuxBackend implementation | `bubbletea-integration.md` § TmuxBackend |
| Interactive() discrimination | `bubbletea-integration.md` § Mode Selection |
| Tick-based polling wiring | `bubbletea-integration.md` § Poll Integration |
