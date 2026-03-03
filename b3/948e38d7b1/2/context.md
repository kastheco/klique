# Session Context

## User Prompts

### Prompt 1

Implement Task 5: Implement Discovery Functions — CleanupSessions, DiscoverOrphans, DiscoverAll, CountKasSessions

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite of `session/tmux/` to remove all AGPL-tainted code from the upstream fork. Introduces a `Session` interface so the instance layer depends on a contract rather than a concrete struct, and extracts program-specific logic (readiness de...

