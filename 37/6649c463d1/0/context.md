# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Delete Old Files and Define Session Interface + Program Adapters

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite of `session/tmux/` to remove all AGPL-tainted code from the upstream fork. Introduces a `Session` interface so the instance layer depends on a contract rather than a concrete struct, and extracts program-specific logic (readiness detection, prompt injection) into...

### Prompt 2

continue

