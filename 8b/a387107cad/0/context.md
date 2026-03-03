# Session Context

## User Prompts

### Prompt 1

Implement Task 8: Rewrite tabbed_window.go — Tab Container

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite all non-overlay panel files in `ui/` to eliminate AGPL-tainted code. The rewrite preserves the identical public API and visual output so all callers (`app/`, `cmd/`) compile without changes, while replacing every line of implementation. Existing test files are the regression suite.
**Ar...

