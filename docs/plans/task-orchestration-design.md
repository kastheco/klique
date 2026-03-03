# Task-Driven Orchestration with Role-Based Agent Switching

## Goal

klique gains awareness of superpowers implementation plans. It parses plan files, displays tasks grouped by phase in the instance list, launches instances pre-configured for the right agent profile, and auto-switches agents as tasks progress through the implement-verify-review lifecycle. Ad-hoc (non-task) instance creation is fully preserved.

## Architecture

klique reads `docs/plans/*.md` from the active repo, parsing `## Phase N:` and `### Task N:` blocks into a structured task graph. Task state (phase, associations, verify history) is tracked in `~/.klique/state.json` — the plan file stays read-only. A new task-grouped section in the instance list shows phases as collapsible headers with tasks and their child instances nested underneath. Agent profiles in config map lifecycle phases to `{program, flags}` pairs; klique swaps agents in the same tmux session when tasks transition phases.

## Components

### 1. Plan Parser

**Package**: `plan/`

Reads superpowers plan files and extracts structured data. Regex/line-scanner based — no markdown AST library.

**Parsing rules:**
- `## Phase N: <name>` starts a new phase group
- `### Task N: <name>` starts a new task within the current phase
- `**Files:**` starts file list (create/modify/test/reference entries)
- `**Step N:**` starts a step within the current task
- `---` is a task separator (boundary is the next `###` or `##`)

**Plan discovery:** Globs `<repoPath>/docs/plans/*.md`, selects the most recently modified file as the active plan. User can switch plans via keybind. No plans = no task groups in the list.

**Reload:** Re-reads on 500ms metadata tick if file mtime changed. New tasks get `planned` status. Removed tasks stay in state with a stale badge.

**Data model:**

```go
type Plan struct {
    FilePath string
    Goal     string
    Phases   []Phase
}

type Phase struct {
    Number int
    Name   string
    Tasks  []PlanTask
}

type PlanTask struct {
    Number    int
    Name      string
    Files     []FileRef // {Action: create|modify|test|ref, Path, Lines}
    Steps     []Step    // {Number, Description, Command, Expected}
    CommitMsg string
    RawText   string    // full task text for prompt injection
}
```

### 2. State Management & Task Lifecycle

**Storage:** New `plans` key in `~/.klique/state.json`:

```json
{
  "plans": {
    "/path/to/plan.md": {
      "active": true,
      "tasks": {
        "1": {
          "phase": "implementing",
          "instance_title": "Task 1: Plan Parser",
          "started_at": "2026-02-20T10:00:00Z",
          "verify_attempts": 2,
          "last_verify_result": "3 issues found"
        }
      }
    }
  }
}
```

**Phases:**

| Phase | Agent | Description |
|-------|-------|-------------|
| `planned` | none | Parsed from plan, waiting to launch |
| `implementing` | implementer | Agent implements using TDD. Loops with verify until clean. |
| `verifying` | same/kas:verify | Tiered verification. Issues found = back to implementing. Clean = eligible for spec_review. |
| `spec_review` | spec-reviewer | Reviews implementation against task spec. Pass = quality_review. Fail = back to implementing. |
| `quality_review` | quality-reviewer | Reviews code quality. Pass = done. Fail = back to implementing. |
| `done` | none | Task complete. |

**Transitions:**

```
planned ──(launch)──> implementing ──(verify pass)──> spec_review ──(pass)──> quality_review ──(pass)──> done
                           ^    ^                          |                        |
                           |    └──(verify fail)───────────┘                        |
                           |    └──────────────────(fail)───────────────────────────┘
                           └──(verify finds issues)──┘
```

**Implementing + verify loop:**
1. Implementer agent works (TDD: failing test -> implement -> pass -> commit)
2. Agent goes idle (detected via existing `HasUpdated()` polling)
3. klique auto-triggers `/kas:verify` (injected into session)
4. Verify passes clean -> transition to `spec_review`, swap agent
5. Verify finds issues -> inject issues as prompt, stay in `implementing`

Auto-verify configurable: `auto_verify: true` in config, or manual confirmation at each step.

**Dependencies:** Implicit from plan structure. Tasks within a phase are sequential (Task N+1 blocked until Task N is done). Phases are sequential (Phase 2 blocked until all Phase 1 tasks done). Blocked state is computed, not stored.

**Instance association:** Tasks track `instance_title`. Used for bidirectional linkage in the TUI and lifecycle event handling.

**Instance kill behavior:**
- Task in implementing/verifying: prompt "Revert to planned, or mark done?"
- Task in review: revert to implementing (worktree preserved)
- Task done: no state change

### 3. Agent Profiles & Role-Based Switching

**Config schema** (new fields in `~/.klique/config.json`):

```json
{
  "profiles": {
    "implementer": { "program": "claude", "flags": [] },
    "spec-reviewer": { "program": "claude", "flags": ["--model", "sonnet"] },
    "quality-reviewer": { "program": "claude", "flags": ["--model", "sonnet"] }
  },
  "phase_roles": {
    "implementing": "implementer",
    "spec_review": "spec-reviewer",
    "quality_review": "quality-reviewer"
  }
}
```

**Profile struct:**

```go
type AgentProfile struct {
    Program string   `json:"program"`
    Flags   []string `json:"flags"`
}
```

**Role resolution:**
1. Look up task phase in `phase_roles` -> profile name
2. Look up profile name in `profiles` -> `AgentProfile`
3. Fallback: `default_program` if either lookup fails

**Switching mechanism:**
1. SIGTERM to current agent process. SIGKILL after 3 seconds.
2. Worktree and branch preserved. Uncommitted work stays.
3. Spawn new agent via `tmux send-keys` in the existing tmux session.
4. After agent starts (trust prompts cleared), inject task text + phase instructions via send-keys.

**Fallback:** No profiles configured = all phases use `default_program`. Purely additive, no behavioral change to existing workflow.

### 4. TUI Integration — Task-Grouped Instance List

Tasks integrate into the existing instance list, not a separate tab. The tabbed window (preview/diff/git) stays unchanged.

**Layout:**

```
┌─sidebar─┐┌─instance list──────────────────┐┌─tabbed window──────────────┐
│          ││ v Phase 1: Foundation           ││ Preview | Diff | Git       │
│          ││   ok T1: Plan Parser      done  ││                            │
│          ││   >> T2: State Mgmt   implement ││  > Analyzing state.go...   │
│          ││     └ kas/task-2 * Running      ││  > Writing test for...     │
│          ││   >> T3: Config      implement  ││                            │
│          ││     └ kas/task-3 * Running      ││                            │
│          ││   > Phase 2: TUI (blocked)      ││                            │
│          ││   > Phase 3: Switching (blocked)││                            │
│          ││ ────────────────────────────── ││                            │
│          ││   Fix unrelated bug  * Running  ││                            │
└──────────┘└──────────────────────────────────┘└────────────────────────────┘
```

**Visual indicators:**
- `ok` = done (dimmed green)
- `>>` = active (bright, color varies by phase)
- `--` = blocked (dimmed)
- (blank) = planned, launchable

**Navigation:**
- `Up`/`Down` or `j`/`k` — navigate between items
- `Left`/`Right` or `e` — expand/collapse phase groups and tasks
- `Enter`/`l` — launch task (on planned task) or attach instance (on instance)
- `t` — transition to next phase
- `b` — send back to implementing
- `s` — switch active plan file

**Selecting a task vs instance:**
- Select a task row: tabbed window shows task detail pane (files, phase, verify history, actions)
- Select an instance row: standard preview/diff/git

**Ad-hoc instances:** The `n` keybind, `-p` flag, and all existing instance management work exactly as before. Ad-hoc instances appear below a separator after all task groups. If no plans exist, the entire list is ad-hoc — identical to stock klique.

### 5. Error Handling

| Scenario | Behavior |
|----------|----------|
| No `docs/plans/` directory | No task groups. Ad-hoc instances only. |
| Unparseable plan file | Toast with error. Task groups hidden for that plan. |
| Agent fails to start | Toast. Phase reverts. Worktree preserved. |
| Agent ignores SIGTERM | SIGKILL after 3 seconds. |
| Plan file deleted while tasks running | Instances keep running. Task groups show stale badge. |
| Plan file modified | Re-parsed on 500ms tick. New tasks = planned. Removed tasks = stale badge. |
| Profile references missing program | Toast. Fallback to `default_program`. |
| Multiple klique instances on same repo | File locking. Atomic writes. Last-write-wins. |

### 6. Testing Strategy

| Component | Approach |
|-----------|----------|
| Plan parser | Table-driven. Markdown strings in, structs out. Malformed plans, edge cases. |
| State persistence | Round-trip serialize/deserialize. Backward compat with state files lacking `plans`. |
| Phase transitions | State machine table tests. All valid + invalid transitions. |
| Dependency resolution | Given plan state, assert blocked/available per task. |
| Profile resolution | Config permutations: full, partial, empty -> correct fallback. |
| Agent switching | Mock tmux. Assert SIGTERM/SIGKILL sequence, spawn, prompt injection. |
| TUI rendering | Snapshot tests via `teatest`. Various states. |

### Non-Functional Requirements

- **Non-blocking**: All plan I/O in `tea.Cmd` goroutines. Results as `tea.Msg`.
- **Zero-config baseline**: No plans, no profiles = identical to stock klique.
- **Switch speed**: Agent swap within 5 seconds. SIGKILL at 3 seconds.
- **Concurrent safety**: Atomic state writes (temp + rename). Mutex in StateManager.
