# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Fix Go user-facing strings referencing "plan store" and stale plan terminology

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Replace all stale `kas plan` CLI references with `kas task` in skill files, agent prompt templates, README, and Go user-facing strings — so agents actually use the correct command name.
**Architecture:** Mechanical text replacement across three file categories: (1) sk...

