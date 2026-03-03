# Session Context

## User Prompts

### Prompt 1

Implement docs/plans/02a-rewrite-session-core.md using the `kasmos-coder` skill. Execute all tasks sequentially.

Reviewer feedback from previous round:
## review round 1

### important

- `session/cli_prompt.go:1-11` — Task 3 not implemented: plan requires clean-room rewrite of cli_prompt.go but the file was not touched (zero diff from main). The plan's goal is "clean-room rewrite all files in session/" and Task 3 explicitly lists this file. Rewrite it per the plan spec.
- `session/notify.go...

