# Session Context

## User Prompts

### Prompt 1

Implement Task 3: Rewrite cli_prompt.go and notify.go — Small Utilities

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite all files in `session/` (excluding the already-rewritten `session/tmux/` and `session/git/` subdirectories) to remove AGPL-tainted lines. The rewrite preserves the identical public API so all callers (`app/`, `ui/`, `daemon/`, `main.go`) compile without changes, while repla...

