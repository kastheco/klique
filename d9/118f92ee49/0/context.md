# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Create orchestration package with engine and prompt builders

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Extract the wave orchestration state machine and prompt builders from `app/` into a standalone `orchestration/` package, and consolidate the remaining TUI wiring from 4 scattered files into a single `app/wave_integration.go` — making orchestration logic independently testable and the T...

