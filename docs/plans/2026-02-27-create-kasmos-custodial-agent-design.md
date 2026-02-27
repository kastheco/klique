# Custodial Agent Design

## Problem

kasmos has workflow gaps that require manual intervention — plans stuck in wrong states,
stale worktrees accumulating, branches with no cleanup path, no way to trigger a specific
wave from outside the TUI's normal flow. These are operational/janitorial tasks that don't
fit the coder, planner, or reviewer roles.

## Solution

A hybrid custodial agent + slash commands + CLI subcommands system:

1. **`kq plan` CLI subcommands** — safe state mutations through the existing FSM and planstate packages
2. **Custodial agent** (`.opencode/agents/custodial.md`) — interactive janitor persona for guided ops
3. **Slash commands** — standalone one-shot operations usable from any agent mode

## Architecture

### Approach: Hybrid Smart

- Operations that touch plan FSM state → Go CLI (`kq plan`) wrapping `planfsm`/`planstate` with flock
- Operations that are pure git/shell → agent runs directly (branch cleanup, PR creation, worktree removal)
- Wave triggering → CLI writes a signal file, TUI picks it up on next metadata tick

### `kq plan` CLI Subcommands

New cobra command group under `cmd/`. Reuses `config/planfsm` and `config/planstate`.

**`kq plan list [--status <status>]`**
Dumps all plans with status, branch, topic. Supports status filter. Plain text output,
one plan per line, parseable by agents.

**`kq plan set-status <plan-file> <status> --force`**
Directly sets a plan's status field, bypassing the FSM transition table. Requires `--force`.
Validates status is one of: `ready`, `planning`, `implementing`, `reviewing`, `done`, `cancelled`.
Writes through `planstate` with flock.

**`kq plan transition <plan-file> <event>`**
Applies a named FSM event (`plan_start`, `implement_start`, `review_approved`, `cancel`,
`reopen`, etc.). Respects the transition table. Prints resulting status on success, or
error with current status + valid events on failure.

**`kq plan implement <plan-file> [--wave N]`**
Transitions plan to `implementing` via FSM, then writes signal file
`docs/plans/.signals/implement-wave-<N>-<date>-<name>.md`. Default wave is 1.
TUI signal scanner picks it up and spawns the wave orchestrator.

### Signal Design

New signal type: `implement-wave-<N>-<date>-<name>.md`

Empty file (matches existing signal conventions). TUI signal scanner parses wave number
from filename, creates `WaveOrchestrator`, fast-forwards to wave N, spawns tasks.

Edge cases:
- Plan already has running orchestrator → toast "wave already running for '<plan>'"
- Wave N doesn't exist → toast "plan has N waves, requested wave M"
- Plan has no `## Wave` headers → fall back to single-coder implementation

### Custodial Agent

Part of the scaffolding system — template at `templates/opencode/agents/custodial.md`
and `templates/claude/agents/custodial.md`.

Configuration:
- Model: `anthropic/claude-sonnet-4-6`
- Temperature: 0.1
- Reasoning effort: low
- Text verbosity: low
- Permissions: full bash/edit/write/read/glob/grep (same as coder)

Static block in `opencode.jsonc` template (not wizard-configurable, always present,
like the `chat` block).

Persona: operational janitor. Confirms before acting, reports what changed, refuses
feature work. Uses `kq plan` for state mutations, raw git/gh for everything else.

Also update local `.opencode/opencode.jsonc` and create `.opencode/agents/custodial.md`
directly so the agent is available without re-running scaffold.

### Slash Commands

**`/kas.reset-plan <plan-file> <status>`**
Force-override plan status. Calls `kq plan set-status --force`. Shows before/after.

**`/kas.finish-branch [plan-file]`**
Merge plan's branch to main or create PR. Infer plan from current branch if omitted.
Verify commits ahead of main, offer merge vs PR, execute, update plan status to done.

**`/kas.cleanup [--dry-run]`**
Three-pass: (1) worktrees for done/cancelled plans, (2) orphan local branches,
(3) plan entries with no .md file. Default dry-run, `--execute` to delete.

**`/kas.implement <plan-file> [--wave N]`**
Set plan to implementing, write wave signal file. Default wave 1.

**`/kas.triage`**
Scan non-done/cancelled plans, show status + branch + last commit + worktree existence.
Group by status. Agent asks what to do with each group.

## Operations Summary

| Operation | Mechanism | Safe? |
|-----------|-----------|-------|
| Reset plan status | `kq plan set-status --force` | flock-protected |
| FSM transition | `kq plan transition` | FSM-validated + flock |
| Trigger wave | `kq plan implement` + signal file | FSM + TUI signal scan |
| Finish branch | git merge / gh pr create | direct git ops |
| Clean worktrees | git worktree remove | direct git ops |
| Clean branches | git branch -d | direct git ops |
| Triage plans | read plan-state.json + agent interaction | read-only scan |
