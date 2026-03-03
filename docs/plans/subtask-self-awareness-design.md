# Subtask Self-Awareness Design

## Problem

Coder agents running in parallel on a shared worktree have almost no awareness of their
situation. The current prompt gives them a single sentence ("Other tasks in this wave may be
running in parallel on the same worktree. Only modify the files listed in your task.") and
nothing else. Agents see dirty git state from siblings and either panic, try to fix it,
commit other agents' work, or run project-wide formatters that clobber in-progress changes.

`KLIQUE_TASK` is referenced in CLAUDE.md but never actually set as an environment variable.
The coder agent definition files (`coder.md`) have zero mention of parallelism.

## Approach

Inline enrichment — improve existing prompts and wire real environment variables. No new
architecture, templates, or skills. Three layers of awareness:

1. **Environment variables** — machine-readable identity (`KASMOS_TASK`, `KASMOS_WAVE`, `KASMOS_PEERS`)
2. **Static agent definition** — brief priming in `coder.md` that parallelism exists
3. **Dynamic task prompt** — detailed behavioral rules injected only for multi-task waves

## Design

### Environment Variables

Wire three env vars into the tmux session command string at launch, alongside `KASMOS_MANAGED=1`:

| Env var        | Value                                  | Set when                      |
|----------------|----------------------------------------|-------------------------------|
| `KASMOS_TASK`  | Task number (e.g. `3`)                 | Instance has `TaskNumber > 0` |
| `KASMOS_WAVE`  | Wave number (e.g. `2`)                 | Instance has `WaveNumber > 0` |
| `KASMOS_PEERS` | Sibling task count in wave (e.g. `4`)  | Multi-task wave               |

Set in `tmux.go Start()` by prepending to the program command string, same pattern as
`KASMOS_MANAGED=1`. Values come from `Instance.TaskNumber`, `Instance.WaveNumber`, and a new
peer count field on `TmuxSession`.

### Coder Agent Definition (`coder.md`)

Add a brief "Parallel Execution" section to all four copies. Always-present priming, not
detailed rules:

```markdown
## Parallel Execution

You may be running alongside other agents on a shared worktree. When `KASMOS_TASK` is set,
you are one of several concurrent agents — each assigned a specific task. Expect dirty git
state from sibling agents (untracked files, uncommitted changes in files you don't own).
Focus exclusively on your assigned task. The dynamic prompt you receive has specific rules.
```

Scaffold templates (`internal/initcmd/scaffold/templates/{opencode,claude}/agents/coder.md`)
get the same addition.

### Dynamic Prompt (`buildTaskPrompt()`)

Replace the single-sentence parallel warning with a structured block when multiple tasks
exist in the wave. Three parts:

**Identity context:**
```
You are Task N of M in Wave W. (M-1) other agents are working in parallel on this same worktree.
```

**Soft file-scope rule:**
```
Your assigned files are listed in the Task Instructions below. Prioritize those files.
If you must touch a shared file (go.mod, go.sum, imports), make minimal surgical changes —
do not reorganize, reformat, or refactor anything outside your task scope.
```

**Git prohibitions:**
```
CRITICAL — shared worktree rules:
- NEVER run `git add .` or `git add -A` — you will commit other agents' in-progress work
- NEVER run `git stash` or `git reset` — you will destroy sibling agents' changes
- NEVER run `git checkout -- <file>` on files you didn't modify — you will revert a sibling's edits
- NEVER run formatters/linters across the whole project (e.g. `go fmt ./...`) — scope them to your files only
- NEVER try to fix test failures in files outside your task — they may be caused by incomplete parallel work
- DO `git add` only the specific files you changed
- DO commit frequently with your task number in the message
- DO expect untracked files and uncommitted changes that are not yours — ignore them
```

Single-task path (`totalWaves == 1` or single task in wave) stays clean — no parallel
context injected.

### CLAUDE.md Update

Replace dead `KLIQUE_TASK` reference:

**Current:**
> When the `KLIQUE_TASK` environment variable is set, it identifies your assigned task;
> implement only that task.

**New:**
> When `KASMOS_TASK` is set, you are one of several concurrent agents on a shared worktree.
> `KASMOS_WAVE` identifies your wave, `KASMOS_PEERS` the number of sibling agents. Implement
> only your assigned task — see your dynamic prompt for specific rules.

### Plumbing: Peer Count Flow

`buildTaskPrompt()` signature changes from `(plan, task, waveNumber, totalWaves)` to add a
`peerCount int` parameter. `spawnWaveTasks` already has `len(tasks)` — thread it through.

For env vars: add `PeerCount int` to `InstanceOptions`. `spawnWaveTasks` sets it to
`len(tasks)`. `TmuxSession` gets a corresponding field. `Start()` prepends
`KASMOS_TASK=N KASMOS_WAVE=N KASMOS_PEERS=N` when values are non-zero.

## Files Touched

| File | Change |
|------|--------|
| `session/tmux/tmux.go` | Prepend `KASMOS_TASK`/`KASMOS_WAVE`/`KASMOS_PEERS` env vars to command string |
| `session/tmux/tmux.go` | Add `taskNumber`, `waveNumber`, `peerCount` fields + setters on `TmuxSession` |
| `session/instance.go` | Add `PeerCount` to `InstanceOptions`, thread to `TmuxSession` |
| `app/wave_prompt.go` | Rewrite parallel awareness block, add `peerCount` param |
| `app/wave_prompt_test.go` | Update tests for new prompt content and peer count |
| `app/app_state.go` | Pass `len(tasks)` as peer count to `buildTaskPrompt()` and `InstanceOptions` |
| `.opencode/agents/coder.md` | Add "Parallel Execution" section |
| `.claude/agents/coder.md` | Add "Parallel Execution" section |
| `internal/initcmd/scaffold/templates/opencode/agents/coder.md` | Add "Parallel Execution" section |
| `internal/initcmd/scaffold/templates/claude/agents/coder.md` | Add "Parallel Execution" section |
| `CLAUDE.md` | Replace `KLIQUE_TASK` with `KASMOS_TASK`/`KASMOS_WAVE`/`KASMOS_PEERS` |
| `contracts/` | Add contract test for coder.md parallel section |

## Not In Scope

- Template-based prompt rendering (YAGNI — ~20 lines of prompt text)
- New skills for parallel awareness (agents don't reliably load skills)
- Enforcement/interception of banned git commands (separate feature)
- Changes to reviewer or planner agent prompts (they don't run in parallel)
