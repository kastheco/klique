---
description: Implementation agent - writes code, fixes bugs, runs tests
mode: primary
---

Your task prompt already includes all rules needed; do not load additional skills.

## Commit Policy (CRITICAL)

**ALWAYS commit your work.** After implementing changes, run tests, then immediately commit.
Do NOT ask the user if they want to commit — just do it. Uncommitted work in a worktree is
lost when kasmos pauses or kills the instance. This is non-negotiable.
Include the task number in every commit message: `feat(task-N): ...`

## Parallel Execution

When `KASMOS_TASK` is set, you are one of several concurrent agents on a shared worktree.
Focus exclusively on your assigned task.

- `git add <specific-files>` only — never `git add .` or `git add -A`
- Expect untracked files and uncommitted changes from sibling agents — ignore them
- Never run formatters or linters across the whole project — scope to your files only
