---
name: kasmos-cli
description: Use when an agent needs exact `kas` command syntax, flags, lifecycle transitions, or troubleshooting for task/instance/skills/setup/check/serve workflows.
---

# kasmos-cli

Use this skill as the shared source for exact `kas` CLI syntax, lifecycle transitions, and task/instance workflows.

<HARD-GATE>
### command surface scope

The commands below are derived from current Cobra handlers and help output in this repo. Do not document legacy `kas plan ...` forms; use `kas task ...` equivalents.

| group | command | command syntax | key flags |
|-------|---------|----------------|-----------|
| task | `kas task list` | `kas task list [--status <ready|planning|implementing|reviewing|done|cancelled>]` | `--status` |
| task | `kas task register` | `kas task register <plan-file> [--branch <name>] [--topic <topic>] [--description <text>]` | `--branch`, `--topic`, `--description` |
| task | `kas task create` | `kas task create <name> [--description <text>] [--branch <name>] [--topic <topic>] [--content <markdown>]` | `--description`, `--branch`, `--topic`, `--content` |
| task | `kas task set-status` | `kas task set-status <plan-file> <status> --force` | `--force` |
| task | `kas task transition` | `kas task transition <plan-file> <event>` | none |
| task | `kas task show` | `kas task show <plan-file>` | none |
| task | `kas task update-content` | `kas task update-content <plan-file> [--file <path>]` | `--file` |
| task | `kas task start` | `kas task start <plan-file>` | none |
| task | `kas task implement` | `kas task implement <plan-file> [--wave <n>]` | `--wave` |
| task | `kas task push` | `kas task push <plan-file> [--message <text>]` | `--message` |
| task | `kas task pr` | `kas task pr <plan-file> [--title <text>]` | `--title` |
| task | `kas task merge` | `kas task merge <plan-file>` | none |
| task | `kas task start-over` | `kas task start-over <plan-file>` | none |
| task | `kas task link-clickup` | `kas task link-clickup [--project <name>]` | `--project` |
| instance | `kas instance list` | `kas instance list [--format text|json] [--status running|ready|loading|paused]` | `--format`, `--status` |
| instance | `kas instance kill` | `kas instance kill <title>` | none |
| instance | `kas instance pause` | `kas instance pause <title>` | none |
| instance | `kas instance resume` | `kas instance resume <title>` | none |
| instance | `kas instance send` | `kas instance send <title> <prompt>` | none |
| instance | `kas instance status` | `kas instance status` | none |
| skills | `kas skills list` | `kas skills list` | none |
| skills | `kas skills sync` | `kas skills sync` | none |
| setup | `kas setup` | `kas setup [--force] [--clean]` | `--force`, `--clean` |
| setup | `kas init` | `kas init [--force] [--clean]` | `--force`, `--clean` |
| check | `kas check` | `kas check [-v|--verbose]` | `-v`, `--verbose` |
| serve | `kas serve` | `kas serve [--bind <addr>] [--db <path>] [--port <n>]` | `--bind`, `--db`, `--port` |
</HARD-GATE>

## task lifecycle commands

### `kas task list`
- Purpose: list task entries, optionally filtered by status.
- Output format is one line per task: `STATUS` + `FILE` + `BRANCH`.
- `--status` accepts `ready`, `planning`, `implementing`, `reviewing`, `done`, `cancelled`.

### `kas task create`
- Creates an entry in the task store, not necessarily local file.
- Default branch is `plan/<name>`.
- Optional fields: `--description`, `--branch`, `--topic`, `--content`.

### `kas task register`
- Registers a local plan file into the task store and sets status to `ready`.
- `<plan-file>` is resolved relative to current working dir.
- `--branch` defaults to `plan/<slug>` where slug is file basename without `.md`.
- If no `--description`, first `# heading` from file becomes description.

### `kas task set-status`
- Force-overrides status and bypasses FSM.
- Requires `--force`; otherwise it exits with `--force required to override task status (this bypasses the FSM)`.

### `kas task transition`
- Applies FSM event names only (no free-form status).
- Valid events: `plan_start`, `planner_finished`, `implement_start`, `implement_finished`, `review_approved`, `review_changes`, `request_review`, `start_over`, `reimplement`, `cancel`, `reopen`.

### `kas task show`
- Prints stored task content.
- Fails with `task not found: <file>` if entry missing.
- Fails with `no content stored for <file>` if content is empty/whitespace.

### `kas task update-content`
- Replaces task content in store.
- Reads from stdin by default and from `--file` when provided.
- If neither stdin content nor `--file` path resolves, the command waits for stdin and can appear to hang.

### `kas task start`
- Transitions task to `implementing` via FSM when needed.
- Sets up dedicated task branch and worktree.
- Prints `started: <plan-file> → implementing` and the worktree path.

### `kas task implement`
- Writes implementation signal file for branch/worktree orchestration.
- `--wave` must be `>= 1`.
- Invalid wave number returns `wave number must be >= 1, got <n>`.

### `kas task push`
- Commits dirty changes in task worktree and pushes branch.
- `--message` defaults to `update from kas`.

### `kas task pr`
- Push + opens a pull request via GitHub CLI.
- Uses description by default when `--title` is missing; falls back to file stem.

### `kas task merge`
- Merges task branch into current branch and transitions FSM to done.
- If task is not in `reviewing`, it transitions via `implement_finished` or forces to `reviewing` before `review_approved`.

### `kas task start-over`
- Resets branch from `HEAD` (`git.ResetTaskBranch`).
- Uses FSM `start_over` when valid; otherwise force-sets status to `planning`.

### `kas task link-clickup`
- Scans stored plan content for `**Source:** ClickUp <ID>` lines.
- Updates missing ClickUp IDs in store entries.
- Optional `--project` overrides derived project name.

## instance lifecycle commands

### actions and state rules

`validateStatusForAction` logic:

- `kill`: allowed from any state.
- `pause`: not allowed when already `paused` (`instance is already paused`).
- `resume`: allowed only from `paused` (`can only resume paused instances (current status: <status>)`).
- `send`: not allowed when paused (`cannot send prompt to a paused instance`).

| command | behavior |
|---------|----------|
| `kas instance list` | list all instances; defaults `text` format |
| `kas instance list --format json` | outputs JSON with `title`, `status`, `branch`, `program`, optional `task_file` |
| `kas instance list --status paused` | filters list by lowercase status |
| `kas instance kill <title>` | refuses dirty worktrees (shows changed-file summary), otherwise removes tmux session and worktree and marks paused (branch preserved); allowed from any state |
| `kas instance pause <title>` | refuses dirty worktrees (shows changed-file summary), otherwise removes tmux session and worktree and marks paused; not allowed if already paused |
| `kas instance resume <title>` | recreates worktree from preserved branch, restarts tmux session |
| `kas instance send <title> <prompt>` | sends prompt into resumed tmux session |
| `kas instance status` | shows aggregated counts (running, ready, paused, killed) |

## skills commands

### `kas skills list`
- Shows **personal** (`~/.agents/skills`) and **project** (`./.agents/skills`) entries.
- Symlinks are printed with `-> <target> (external)` when the listed entry is a symlink.
- This is the list used by harness sync + skill loading checks.

### `kas skills sync`
- Syncs personal skills from `~/.agents/skills/` into harness global directories.
- Global targets are Claude and OpenCode skill directories (not project `.agents/skills`).
- Output includes `OK`, `SKIP (not installed)`, and `FAILED` statuses.

## setup commands

### `kas setup` / `kas init`
- Alias pair: `setup` and `init` are interchangeable.
- `--force`: overwrite project scaffold files.
- `--clean`: ignore existing config and start from defaults.
- Runs interactive initialization flow for harness detection, roles, hooks, and project scaffolding.

## check command

### `kas check`
- Health audit for global + project skill sync.
- `-v/--verbose` prints per-skill detail rows.
- Prints `Health: X/Y OK (N%)`.
- Exits non-zero when `N < 100` (even after useful output) because it returns an internal unhealthy sentinel.

## serve command

### `kas serve`
- Starts task store HTTP server (`taskstore.NewHandler`).
- Defaults:
  - `--bind`: `0.0.0.0`
  - `--db`: resolved default sqlite path (usually under `.kasmos/taskstore.db`)
  - `--port`: `7433`
- Logs `task store listening on http://<bind>:<port> (db: <path>)` and supports graceful SIGINT/SIGTERM shutdown.

## workflows

### workflow A: task file bootstrap + register/show/update

```bash
kas task create my-feature --description "Add search indexing support" --topic infra --branch plan/my-feature
kas task show my-feature.md
kas task update-content my-feature.md --file /tmp/my-feature.md
kas task show my-feature.md

# alternate path from disk import
kas task register plans/my-feature.md --topic infra --branch plan/my-feature
kas task show my-feature.md
kas task update-content my-feature.md --file plans/my-feature.md
```

### workflow B: branch/worktree lifecycle

```bash
kas task start my-feature.md
kas task push my-feature.md --message "checkpoint from implementing"
kas task pr my-feature.md --title "My feature"
kas task merge my-feature.md
kas task start-over my-feature.md
```

### workflow C: FSM-first flow vs override flow

```bash
# normal progression through FSM events
kas task transition my-feature.md plan_start
kas task transition my-feature.md planner_finished
kas task transition my-feature.md implement_start
kas task transition my-feature.md implement_finished
kas task transition my-feature.md request_review

# explicit override (bypass FSM) only with force
kas task set-status my-feature.md cancelled --force
```

### wave orchestration

- `kas task implement <plan-file> --wave <n>` writes an implementation signal consumed by downstream automation.
- Use FSM transitions for normal lifecycle movement, only reserve `set-status --force` for manual recovery scenarios.

## edge cases and troubleshooting

- `kas task set-status <plan-file> <status>` without `--force` fails by design.
- `kas task implement <plan-file> --wave 0` fails: `wave number must be >= 1, got 0`.
- `kas task update-content <plan-file>` reads `/dev/stdin` unless `--file` is passed.
- `kas task update-content <plan-file>` with neither `--file` nor piped stdin blocks waiting for stdin.
- `kas task show <missing-file>` errors as `task not found`.
- `kas task show <file-with-empty-content>` errors `no content stored`.
- `kas check` can print full health tables and still exit code `1` when health is below `100%`.
- `kas instance send` returns `cannot send prompt to a paused instance` for paused targets.
- `kas instance resume` requires preserved worktree metadata (`repo path` + `branch`), else `instance "<title>" has no stored worktree metadata; cannot resume`.

## anti-patterns

- Using legacy `kas plan ...` command forms (or docs that suggest it) instead of `kas task ...`.
- Editing task store files directly when `kas task create/register/update-content/show/transition` already owns state transitions.
- Using `kas task set-status` for normal state movement (replace with `kas task transition` and valid events).
- Assuming `kas skills sync` syncs project-level `./.agents/skills` into harness globals; it only syncs personal `~/.agents/skills` to harness global dirs.

## verification checks

- Detect missing skill before implementation:

```bash
kas skills list | rg 'kasmos-cli'
```

- Verify skill is visible after adding this file:

```bash
kas skills list | rg 'kasmos-cli'
```
