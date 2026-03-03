# Details Tab Design — Replace Lazygit with Info Pane

## Overview

Replace the third tab in `TabbedWindow` (`Git` → embedded lazygit) with an `Info` tab that displays contextual metadata about the selected instance and its associated plan. The lazygit integration is removed entirely — all tmux subprocess, PTY, and override config code is deleted.

## Decisions

- **Read-only** — no inline actions, no interactivity beyond scrolling
- **Ad-hoc instances show instance metadata** — title, program, branch, path, created, status
- **Tab name: "Info"** — concise, works for both plan-bound and ad-hoc instances
- **Approach: full replacement** — lazygit code deleted, not hidden behind a flag

## Tab Header

```
┌──────────────┐┌──────────────┐┌──────────────┐
│   Agent     ││   Diff      ││  Info       │
└──────────────┘└──────────────┘└──────────────┘
```

Icon: `` (nf-cod-info).

## Content Layout — Plan-Bound Instance

```
  plan
  ─────────────────────────────
  name              my-feature
  description       add dark mode toggle
  status            implementing
  topic             ui
  branch            plan/my-feature
  created           2026-02-25

  instance
  ─────────────────────────────
  title             my-feature-coder
  role              coder
  program           claude
  wave              2/3
  task              4 of 6

  wave progress
  ─────────────────────────────
  task 1            ✓ complete
  task 2            ✓ complete
  task 3            ● running
  task 4            ○ pending
```

Visual rules:
- Section headers: **foam** (`ColorFoam`), bold
- Dividers: thin `─` in `ColorOverlay`
- Labels: `ColorMuted`, values: `ColorText`
- Status values get semantic coloring: `implementing` → iris, `done` → foam, `reviewing` → gold, `ready` → muted
- Wave task glyphs reuse `✓ ● ○` vocabulary from status bar

## Content Layout — Ad-Hoc Instance

```
  instance
  ─────────────────────────────
  title             fix-login-bug
  program           opencode
  branch            kas/fix-login-bug
  path              /home/kas/dev/myapp
  created           2026-02-25 14:30
  status            running
```

No plan section — just instance metadata.

## Content Layout — No Instance Selected

Muted centered text: `no instance selected`

## New File: `ui/info_pane.go`

~150 lines. Pure renderer, no subprocess, no mutex.

```go
type InfoData struct {
    // Instance fields
    Title    string
    Program  string
    Branch   string
    Path     string
    Created  time.Time
    Status   string

    // Plan fields (empty for ad-hoc)
    PlanName        string
    PlanDescription string
    PlanStatus      string
    PlanTopic       string
    PlanBranch      string
    PlanCreated     time.Time

    // Wave fields (zero values = no wave)
    AgentType   string
    WaveNumber  int
    TotalWaves  int
    TaskNumber  int
    TotalTasks  int
    WaveTasks   []WaveTaskInfo
}

type WaveTaskInfo struct {
    Number int
    State  string // "complete", "running", "failed", "pending"
}

type InfoPane struct {
    width, height int
    data          InfoData
    viewport      viewport.Model
}
```

Public API:
- `NewInfoPane() *InfoPane`
- `SetSize(width, height int)`
- `SetData(data InfoData)`
- `ScrollUp()` / `ScrollDown()`
- `String() string`

Uses `viewport.Model` from bubbles for scroll support when content exceeds visible height.

## Deletion Scope

### Files deleted
- `ui/git_pane.go` — entire file

### Code removed from existing files
- `TabbedWindow.git *GitPane` field, `gitContent string`
- `GitTab` constant → renamed to `InfoTab`
- `NewTabbedWindow(preview, diff, git)` → `NewTabbedWindow(preview, diff, info)`
- `SetGitContent()`, `GetGitPane()`, `IsInGitTab()` → replaced with info equivalents
- `spawnGitTab()`, `killGitTab()`, `enterGitFocusMode()` in `app_state.go`
- `gitTabTickMsg` and its 30fps ticker in `app.go`
- Git tab focus-mode key forwarding in `app_input.go`
- Lazygit spawn/kill on Tab cycling in `app_input.go`
- `kas_lazygit_*` session cleanup in `session/tmux`
- Help text referencing lazygit/git tab

### Focus ring
- `slotGit` (slot 3) stays — just routes to info pane instead of git pane
- No focus-mode entry from info tab (read-only, no PTY)
- Key forwarding for git focus removed

## Documentation Updates

All references to lazygit and git tab across documentation, help text, comments, and README must be updated to reflect the info tab. Specifically:
- `CLAUDE.md` — remove any lazygit references if present
- `app/help.go` — update keybind descriptions (tab cycle text, remove `g` lazygit keybind)
- Code comments referencing lazygit throughout `app_input.go`, `app_state.go`, `tabbed_window.go`
- README or any user-facing docs mentioning the git tab
