# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Make waveAllCompleteMsg push non-blocking

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Eliminate the 1-2s UI freeze that occurs when the user confirms the "push branch and start review?" dialog after wave completion, caused by a synchronous `git push` in the bubbletea `Update` loop.
**Architecture:** The `waveAllCompleteMsg` handler in `app/app.go` calls `worktree.Push(false)` synchronousl...

### Prompt 2

Continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed.

