# kasmos Agents

## Coder
Implementation agent. Writes code, fixes bugs, runs tests.
Load the `kasmos-coder` skill.

## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs (not line-based `git diff`).
Use `sg` (ast-grep) to verify patterns across the codebase.
Load the `kasmos-reviewer` skill.

## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Load the `kasmos-planner` skill.

## Plan State (CRITICAL)
Plans live in `docs/plans/`. State tracked in `docs/plans/plan-state.json`.
Never modify plan files for state tracking. **You MUST register every plan** by adding
an entry to `plan-state.json` with `"status": "ready"` immediately after writing the `.md` file.
Unregistered plans are invisible in the kasmos sidebar.
Valid statuses: `ready` → `in_progress` → `done`. Only kasmos transitions beyond `done`.

## CLI Tools

Read the `cli-tools` skill (SKILL.md) at session start. Read individual
resource files in `resources/` when using that specific tool.
