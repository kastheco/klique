# Session Context

## User Prompts

### Prompt 1

Implement Task 2: TUI and CLI Kill Actions Become Pause

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Make kill safe by preserving the git branch and changing all user-facing kill actions (TUI key, context menu, CLI) to pause the instance instead of destroying it, preventing accidental data loss.
**Architecture:** Currently `Kill()` calls `Cleanup()` which deletes both the worktree directory and the git bran...

