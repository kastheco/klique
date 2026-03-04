# Session Context

## User Prompts

### Prompt 1

Implement Task 3: `merge` and `start-over` commands

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Add high-level lifecycle commands (`create`, `start`, `push`, `merge`, `pr`, `start-over`) to `kas task` so users can manage the full plan lifecycle from the CLI without the TUI.
**Architecture:** All 6 commands are added as cobra subcommands under the existing `kas task` command tree in `cmd/task.go`. Each comm...

