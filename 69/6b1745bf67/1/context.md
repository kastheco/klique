# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Migrate all test files to orchestration package types

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Delete the AGPL-tainted `app/wave_orchestrator.go` and `app/wave_prompt.go` and switch all `app/` code to import the clean-room `orchestration/` package, eliminating duplicate types and licensing risk.
**Architecture:** The `orchestration/` package already contains a complete clean-room rewri...

