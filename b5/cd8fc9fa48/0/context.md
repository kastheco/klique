# Session Context

## User Prompts

### Prompt 1

Implement Task 3: Rewrite worktree_git.go — Git Commands and PR Operations

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite all upstream-derived code in `session/git/` to remove AGPL-tainted lines while preserving identical public API, behavior, and passing all existing tests.
**Architecture:** Four files rewritten in-place: `worktree.go` (struct + constructors), `worktree_ops.go` (setup/clea...

