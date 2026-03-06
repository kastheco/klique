---
name: kasmos-coder-lite
description: Load when you are the coder agent in low-context mode — minimal execution rules for plan task implementation.
---

# kasmos-coder-lite

You are the coder agent. Implement only your assigned task with minimal context usage.

## execution rules

1. implement only the task identified by `KASMOS_TASK`. do not touch sibling tasks.
2. write a failing test first. no production code without a failing test.
3. run tests scoped to your package before committing:
   `go test ./path/to/package/... -run TestName -v`
4. verify: build must pass, tests must be green, no regressions in your package.
5. commit immediately after verification:
   `git add <specific-files> && git commit -m "feat(task-N): description"`

## shared worktree rules

sibling agents are modifying this worktree concurrently. violations destroy sibling work.

**never:**
- `git add .` or `git add -A` — stages sibling in-progress files
- `git stash` — destroys sibling uncommitted changes
- `git reset` — destroys sibling uncommitted changes
- `git checkout -- <file>` on files you did not touch — reverts sibling edits
- project-wide formatters (`go fmt ./...`) — scope formatters to your changed files only

**always:**
- `git add <specific-files>` — only files you modified
- include task number in every commit message: `feat(task-6): description`
- ignore untracked files and dirty state not caused by your work

## when stuck

- **2 failed fix attempts:** stop. document the failure in a commit message and commit partial work.
- **missing type or function from a sibling's package:** do not stub or mock it. commit whatever
  you have with a note (e.g., `partial: blocked on task N types`) and stop.
- **test failures in files outside your scope:** ignore them — they belong to sibling agents.

## signaling

you are running in managed mode (`KASMOS_MANAGED=1`).

after committing your implementation: **stop.**

do not write sentinel files. do not implement other tasks. kasmos detects completion
automatically when your agent returns to its input prompt and handles all lifecycle
transitions via the wave orchestrator.
