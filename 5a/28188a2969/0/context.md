# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Fix Stale Date-Prefix References in Planner Scaffold Templates

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Add a persistent `--repo` flag to all `kas plan` subcommands so they can target any repo without relying on CWD, and fix stale date-prefix references in planner scaffold templates.
**Architecture:** Add a `--repo` flag to the `plan` parent command that propagates to all subcommands. ...

