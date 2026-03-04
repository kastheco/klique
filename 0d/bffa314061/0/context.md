# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Update skills, docs, and remove legacy artifacts

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Remove every `docs/plans` reference from the codebase and ensure agent/skill files teach the task store CLI (`kas task`) as the single source of truth — no agent should ever look for, read from, or write to a `docs/plans/` directory.
**Architecture:** Two parallel cleanup tracks: (1) update all sk...

