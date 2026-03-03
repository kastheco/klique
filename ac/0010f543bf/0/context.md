# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Kill, Pause, Resume, and Send Commands

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Add a `kas instance` CLI command tree that enables headless instance monitoring and lifecycle management — listing, killing, pausing, resuming, and prompting agent sessions without launching the TUI.
**Architecture:** A new `cmd/instance.go` file follows the established pattern from `cmd/task.go`: testable ...

