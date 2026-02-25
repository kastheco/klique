# Contextual Status Bar Design

## Goal

Add a full-width top status bar showing contextual info (repo name, branch, plan name, wave progress) that adapts based on the focused/selected item.

## Visual Treatment

- Single full-width bar at the very top of the screen, 1 line tall
- `ColorSurface` (#2a273f) background — subtle lift from `ColorBase`, no border needed
- Always visible — shows baseline info even when nothing is selected

## Content Layout

Left-aligned, sections separated by `ColorOverlay` dim separators (`│`).

### Baseline (always visible)

```
  kasmos │ repo-name │  main
```

- App name: `ColorIris`, bold
- Repo name: `ColorText`
- Branch: git branch nerd font glyph + branch name in `ColorFoam`
- Branch defaults to active repo's current branch

### Plan selected in sidebar

```
  kasmos │ repo-name │  plan/auth-refactor │ auth-refactor │ implementing
```

- Plan's branch replaces baseline branch
- Plan display name in `ColorText`
- Plan status in semantic color:
  - ready/planning: `ColorMuted`
  - implementing: `ColorFoam`
  - reviewing: `ColorRose`
  - done: `ColorFoam`

### Active wave orchestration

```
  kasmos │ repo-name │  plan/auth-refactor │ auth-refactor │ wave 2/4 ✓✓✓●○
```

- Wave N/M label in `ColorSubtle`
- Per-task glyphs:
  - `✓` complete (`ColorFoam`)
  - `●` running (`ColorIris`)
  - `✕` failed (`ColorLove`)
  - `○` pending (`ColorMuted`)

### Instance selected in list

```
  kasmos │ repo-name │  plan/auth-refactor
```

- Shows the selected instance's branch (or its plan's branch)
- No agent type or status — keep it navigational

## Data Model

```go
type StatusBarData struct {
    RepoName    string
    Branch      string
    PlanName    string      // empty = no plan context
    PlanStatus  string      // "ready", "implementing", "reviewing", "done"
    WaveLabel   string      // "wave 2/4" or empty
    TaskGlyphs  []TaskGlyph // per-task status for wave progress
}

type TaskGlyph int
const (
    TaskGlyphComplete TaskGlyph = iota
    TaskGlyphRunning
    TaskGlyphFailed
    TaskGlyphPending
)
```

## Architecture

- Pure rendering component: `ui/statusbar.go`
- Receives `StatusBarData` struct — no I/O, no goroutines
- App model computes data from existing state (sidebar selection, plan state, wave orchestrators, selected instance)
- `SetSize(width)` for terminal width, `SetData(StatusBarData)` for content updates

## Layout Integration

- `updateHandleWindowSizeEvent`: subtract 1 from `contentHeight` for the status bar height
- `View()`: render status bar first, then columns + menu below it
- Remove `PaddingTop(1)` from `colStyle` — the status bar provides that visual separation
