# Tmux Session Browser — Design

## Goal

TUI overlay to browse, kill, and adopt orphaned tmux sessions (`kas_`-prefixed sessions that exist in tmux but aren't tracked by any kasmos Instance).

## Aesthetic Identity

Minimal, data-dense — matches existing picker/context menu overlays. Rosé Pine Moon palette, rounded border, search-first interaction.

## Decisions

- **Adopt + Attach**: the browser offers both passive attach (fullscreen tmux peek) and full adopt (re-register as a kasmos Instance). Kill is also available.
- **Metadata list only**: no live pane preview. Static info per session: name, age, dimensions, attached status.
- **Dedicated keybind `T`**: opens from default state. First-class operation for crash recovery.
- **Arrow-key navigation**: no vim `j`/`k` in overlays. Letter keys always type into search filter.

## Data Model

### OrphanSession

```go
type OrphanSession struct {
    Name     string    // raw tmux name, e.g. "kas_auth-refactor-implement"
    Title    string    // human name: strip "kas_" prefix
    Created  time.Time // session creation time
    Windows  int       // window count
    Attached bool      // another client attached
    Width    int       // pane columns
    Height   int       // pane rows
}
```

### Discovery

```go
func DiscoverOrphans(cmdExec cmd.Executor, knownNames []string) ([]OrphanSession, error)
```

Runs `tmux ls -F '#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}'`. Filters for `kas_` prefix, subtracts `knownNames` (sanitized tmux names of all current Instances).

Called as a `tea.Cmd`, result delivered via `tmuxOrphansMsg`.

## Overlay Component

### Layout (80-col example)

```
╭─ tmux sessions ──────────────────────────────╮
│                                               │
│  ╭──────────────────────────────────────────╮ │
│  │ 🔍 Type to filter...                    │ │
│  ╰──────────────────────────────────────────╯ │
│                                               │
│  ▸ auth-refactor-implement    12m ago   80×24 │
│    db-migration-plan          3h ago    120×40 │
│    quick-fix-coder            1d ago  ● 80×24  │
│                                               │
│  ↑↓ navigate • k kill • a adopt • o attach   │
│  esc close                                    │
╰───────────────────────────────────────────────╯
```

- Title: `colorIris`, bold
- Border: `lipgloss.RoundedBorder()`, `colorIris`
- Search bar: `colorFoam` border when active
- Selected row: `colorBase` fg on `colorFoam` bg
- `●` indicator for sessions attached by another client
- Age: relative time in `colorMuted`
- Dimensions: `WxH` in `colorMuted`
- Hint bar: `colorMuted`

### State

```go
type TmuxBrowserOverlay struct {
    sessions    []TmuxBrowserItem
    filtered    []int           // indices into sessions
    selectedIdx int             // index into filtered
    searchQuery string
    width       int
}

type TmuxBrowserItem struct {
    Name     string
    Title    string
    Created  time.Time
    Windows  int
    Attached bool
    Width    int
    Height   int
}
```

### Actions

```go
type BrowserAction int
const (
    BrowserNone BrowserAction = iota
    BrowserDismiss
    BrowserKill
    BrowserAdopt
    BrowserAttach
)

func (t *TmuxBrowserOverlay) HandleKeyPress(msg tea.KeyMsg) BrowserAction
```

### Key Map

| Key | Search empty | Search active |
|-----|-------------|---------------|
| `↑` / `↓` | Navigate | Navigate |
| `k` | Kill selected | Types into search |
| `a` | Adopt selected | Types into search |
| `o` | Attach selected | Types into search |
| `Enter` | Attach selected | Attach selected |
| `Esc` | Dismiss | Clear search (if non-empty), dismiss (if empty) |
| `Backspace` | No-op | Delete last char |
| Other runes | Type into search | Type into search |

## App Integration

### New state

```go
stateTmuxBrowser  // in app.go state enum
```

### New field on `home`

```go
tmuxBrowser *overlay.TmuxBrowserOverlay
```

### Keybind `T`

Added to `keys/` global keymap. In default state, triggers `discoverTmuxOrphans()` which builds `knownNames` from `allInstances` and returns a `tea.Cmd` calling `tmux.DiscoverOrphans`.

### Message flow

1. `T` pressed → `discoverTmuxOrphans()` cmd dispatched
2. `tmuxOrphansMsg` received → if empty, toast "no orphaned tmux sessions found"; if non-empty, create overlay, enter `stateTmuxBrowser`
3. User interacts → `HandleKeyPress` returns action
4. Action dispatched:

| Action | Behavior |
|--------|----------|
| `BrowserDismiss` | Nil overlay, return to `stateDefault` |
| `BrowserKill` | Async cmd: `tmux kill-session -t <name>`. Remove from list. Auto-dismiss + toast if list empties. |
| `BrowserAttach` | Exit alt-screen, `tmux attach-session -t <name>`, re-enter on detach. Toast on return. |
| `BrowserAdopt` | Create new `Instance` with orphan's title. Construct `TmuxSession` with existing name, call `Restore()`. Add to `allInstances` + nav panel. Dismiss overlay. Select new instance. |

### Adopt details

- `Status: Ready`
- `Program: "unknown"` (irrecoverable)
- `Path: activeRepoPath` (best guess)
- No worktree created — orphan's tmux session already has its working directory
- User can rename afterward via existing rename flow

### Rendering

In `View()` when `stateTmuxBrowser`:
```go
content := m.tmuxBrowser.Render()
return overlay.PlaceOverlay(0, 0, content, base, true, true)
```

Centered with background fade.

### Guard

Add `stateTmuxBrowser` to the early-return state list in `handleMenuHighlighting`.

## Conventions Update

### CLAUDE.md

Add to Standards section:
```
- **Arrow-key navigation in overlays**: use ↑↓ for navigation, not j/k vim bindings.
  Letter keys should always type into search/filter when present.
```

### Existing overlay audit

Remove `j`/`k` navigation from `ContextMenu.HandleKeyPress` in `ui/overlay/contextMenu.go`. `PickerOverlay` already uses arrow-only (correct).

### Help screen

Add to "sessions" section in `app/help.go`:
```
keyStyle.Render("T") + descStyle.Render("             - browse orphaned tmux sessions"),
```
