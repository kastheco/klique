# Session Context

## User Prompts

### Prompt 1

Implement Task 1: `kas audit` subcommand

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Add CLI commands for querying the audit log, managing orphaned tmux sessions, and viewing instance status summaries. Closes Category 5 (Monitoring & Observability) in the UI/CLI feature parity report.
**Architecture:** Three independent subcommand groups:
**Tech Stack:** Go, cobra, `config/auditlog`, `session/tmux`, `confi...

