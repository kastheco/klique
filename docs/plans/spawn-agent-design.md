# Spawn Agent Design

## Goal

Restore `s` keybind for spawning ad-hoc agent sessions for manual work. These instances sit outside the plan lifecycle — no automatic push prompts, no review spawning, no wave orchestration — but are fully managed (kill, abort, checkout, PR, resume all work).

## Architecture

A form overlay (`huh`-backed, same pattern as `n` new-plan) collects name + optional branch/path overrides. On submit, an `Instance` is created with no `PlanFile` or `AgentType`, auto-opting out of lifecycle logic. The worktree/branch either auto-generates from the name (default) or uses user-supplied overrides.

## Keybind

- `s` → `KeySpawnAgent` (new constant, replaces dead `KeyFocusSidebar`)
- `KeyFocusSidebar` removed — arrow keys already handle sidebar focus
- `KeyNew` dead code at line 988 of `app_input.go` removed

## Form Overlay

New `NewSpawnFormOverlay(title, width)` constructor on `FormOverlay`. Adds `branchVal` and `pathVal` fields alongside existing `nameVal`/`descVal`. Three visible fields:

| Field | Required | Placeholder | Purpose |
|-------|----------|-------------|---------|
| name | yes | — | Session title, used for branch name if no override |
| branch | no | `kas/<name>` | Existing or new branch to use |
| path | no | `.worktrees/<branch>_<ts>` | Working directory override |

Accessors: `Branch() string`, `WorkPath() string` added to `FormOverlay`.

Tab/↑↓ cycles through all fields. Enter submits (requires non-empty name). Esc cancels.

## App State

New `stateSpawnAgent` enum value. Handler reads form fields on submit:

- **No overrides**: calls `instance.Start(true)` — standard auto-worktree + auto-branch
- **Branch override**: constructs `GitWorktree` with the specified branch, calls `Setup()` (handles both new and existing branches), then `StartInSharedWorktree()`
- **Path override**: sets `instance.Path` before start, skips worktree creation (runs in-place like `StartOnMainBranch`)

## Instance Properties

```go
session.InstanceOptions{
    Title:   name,
    Path:    m.activeRepoPath, // or path override
    Program: m.program,
    // No PlanFile, No AgentType — pure ad-hoc
}
```

## Lifecycle Opt-Out

All lifecycle gates already check `PlanFile != ""`:
- `shouldPromptPushAfterCoderExit` → returns false
- `spawnReviewer` → never triggered (no plan to review)
- Wave orchestration → no `WaveNumber`/`TaskNumber`

No new flags needed.

## Rendering

Ad-hoc instances render with their title + status icon + branch. No plan badges, no wave/task labels, no lifecycle indicators. This is already the default for instances with no `PlanFile`.

## Management

All existing keybinds work unchanged:
- `k` kill, `K` abort, `c` checkout, `P` PR, `r` resume, `delete` dismiss
