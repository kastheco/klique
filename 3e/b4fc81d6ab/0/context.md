# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Add `kas task show` CLI subcommand

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Stop agents from reading plan content via disk files at `docs/plans/`. Add a `kas task show` CLI command so agents can retrieve plan content from the database, and update all prompt builders, review templates, and skill documentation to reference the CLI instead of disk paths.
**Architecture:** Three parallel c...

