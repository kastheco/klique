# kasmos Agents

## Coder
Implementation agent. Writes code, fixes bugs, runs tests.
Follow TDD: write failing test first, implement, verify green.
Load superpowers skills: `test-driven-development`, `systematic-debugging`, `verification-before-completion`.
Load project skills: `tui-design` (TUI components), `tmux-orchestration` (tmux/worker code), `golang-pro` (Go patterns).

## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs (not line-based `git diff`).
Use `sg` (ast-grep) to verify patterns across the codebase.
Load superpowers skills: `requesting-code-review`, `receiving-code-review`.
Load project skills: `tui-design` (always for TUI/UX reviews), `tmux-orchestration` (when reviewing tmux/worker code).

## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Load superpowers skills: `brainstorming`, `writing-plans`.
Load project skills: `tui-design` (always for TUI work), `tmux-orchestration` (when task involves tmux/worker lifecycle).

## Task Store (CRITICAL)
Task state lives in the task store — a SQLite database (`~/.config/kasmos/kasmos.db`) or a remote HTTP API.
Use `kas task` CLI commands for all lifecycle operations:
- `kas task list` — list tasks and statuses
- `kas task show <file>` — read plan content
- `kas task create <name>` — create a new task
- `kas task register <file>` — register a plan file from disk
- `kas task update-content <file>` — update plan content
- `kas task transition <file> <event>` — FSM state transition
- `kas task set-status <file> <status> --force` — force override
Never modify task state directly. Unregistered plans are invisible in the kasmos sidebar.
Valid statuses: `ready` → `planning` → `implementing` → `reviewing` → `done`.

## CLI Tools

Read the `cli-tools` skill (SKILL.md) at session start. Read individual
resource files in `resources/` when using that specific tool.
