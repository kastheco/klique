# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Add `resolveRepoRoot()`, fix `resolvePlansDir()`, and test

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Make `kas plan` CLI commands work correctly when run from git worktrees, so coder/fixer agents can manage plan state without being on the main branch.
**Architecture:** The root cause is `resolvePlansDir()` in `cmd/plan.go` which uses `os.Getwd()` to find `docs/plans/`. When agents run i...

