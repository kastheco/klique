# klique Agents

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

## Plan State
Plans live in `docs/plans/`. State tracked in `docs/plans/plan-state.json`.
Never modify plan files for state tracking. Valid statuses: `ready`, `in_progress`, `done`.

{{TOOLS_REFERENCE}}
