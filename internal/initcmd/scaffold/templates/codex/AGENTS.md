# kasmos Agents

## Coder
Implementation agent. Writes code, fixes bugs, runs tests.

## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs (not line-based `git diff`).
Use `sg` (ast-grep) to verify patterns across the codebase.
Load the `kasmos-reviewer` skill.

## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Load the `kasmos-planner` skill.

## Task State (CRITICAL)
Task state is stored in the **task store** (SQLite database or HTTP API), not in files on disk.
Never modify task state directly — use `kas task` CLI commands or sentinel files.
**You MUST register every plan** via `kas task register <plan>.md` immediately after writing it.
Unregistered plans are invisible in the kasmos sidebar.
Valid statuses: `ready` → `planning` → `implementing` → `reviewing` → `done`. Use `kas task` CLI for transitions.

## CLI Tools

Read the `cli-tools` skill (SKILL.md) at session start. Read individual
resource files in `resources/` when using that specific tool.
