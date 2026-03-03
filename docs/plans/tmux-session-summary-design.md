# Tmux Session Summary — Design

## Problem

The `T` keybind currently only shows **orphaned** `kas_` tmux sessions — sessions not tracked by any kasmos instance. This means you can't see the full picture of what's running without mentally combining the sidebar instance list with the tmux browser. There's also no persistent at-a-glance indicator of how many tmux sessions are active.

## Goals

1. **Unified `T` overlay:** Show *all* `kas_`-prefixed tmux sessions — both managed (tracked by kasmos instances) and orphaned. Managed sessions show enrichment data (plan name, agent type, status). Actions are contextual: adopt is only available for orphans.

2. **Persistent session count:** Display the total number of active `kas_` tmux sessions in the status bar so users can glance at the count without opening the overlay.

## Design

### Part 1: Unified session overlay

#### Data layer

New function `DiscoverAll()` in `session/tmux/tmux.go` that returns every `kas_`-prefixed session from `tmux ls`. Each entry carries a `Managed bool` field set by matching against the known instance names passed in.

```go
type SessionInfo struct {
    Name     string    // raw tmux name, e.g. "kas_auth-refactor-implement"
    Title    string    // human name with "kas_" stripped
    Created  time.Time
    Windows  int
    Attached bool
    Width    int
    Height   int
    Managed  bool      // true = tracked by a kasmos instance
    // Enrichment (populated by app layer when Managed=true):
    PlanFile  string
    AgentType string   // "planner"/"coder"/"reviewer"/""
    Status    string   // "running"/"ready"/"loading"/"paused"
}
```

The app layer enriches managed entries from `allInstances` before passing to the overlay. `DiscoverAll` replaces `DiscoverOrphans` at the call site; `DiscoverOrphans` can stay for backward compat or be removed.

#### Overlay changes

`TmuxBrowserOverlay` and `TmuxBrowserItem` gain the enrichment fields. Rendering changes:

- **Managed items** show a muted agent-type badge (e.g., `coder` or `planner`) and plan name when available. Rendered with a subtle visual distinction (e.g., muted prefix icon or label).
- **Orphaned items** render as today (title, age, dimensions).
- **Action hints** update based on selected item's `Managed` field:
  - Orphaned: `↑↓ navigate · k kill · a adopt · o attach · esc close`
  - Managed: `↑↓ navigate · k kill · o attach · esc close`
- `a` (adopt) is a no-op when a managed item is selected.
- Overlay title stays "tmux sessions".

#### App wiring

- `discoverTmuxOrphans()` → `discoverTmuxSessions()`. Same pattern: snapshot known names, run `DiscoverAll` in goroutine, return msg.
- `tmuxOrphansMsg` → `tmuxSessionsMsg` (carries `[]SessionInfo`).
- In `Update`, convert `SessionInfo` slice to `TmuxBrowserItem` slice, populating enrichment fields from `allInstances`.
- Zero-result toast: "no kas tmux sessions found".

### Part 2: Session count in status bar

#### Data flow

Add `TmuxSessionCount int` to `StatusBarData`.

In the metadata poll goroutine (runs every 500ms, already calls tmux subprocesses), add a `tmux ls` call that counts `kas_`-prefixed lines. Return the count in `metadataResultMsg` as a new `TmuxSessionCount int` field.

The app stores the latest count in `home.tmuxSessionCount` and feeds it into `computeStatusBarData()`.

#### Rendering

In `StatusBar.String()`, when `TmuxSessionCount > 0`, render `tmux:N` in `ColorMuted` with `ColorSurface` background. Placement: between the left group and the center branch, appended to the left group with a ` · ` separator. When count is 0, nothing renders.

Example: `kasmos · implementing · tmux:3     ⎇ feat/auth     myrepo`

### Keybind

`T` (shift+T) — unchanged. Lowercase `t` is already bound to `KeyFocusList`.

## Non-goals

- Showing non-`kas_` tmux sessions (user's personal sessions are not our business).
- Automatically refreshing the overlay while it's open (user can close and reopen).
- Killing managed instances from the overlay (that's what `X` abort does in the main view; `k` in the overlay only kills the tmux session, not the instance).
