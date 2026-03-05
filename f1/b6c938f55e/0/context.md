# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Rewrite `GetConfigDir` to return project-local `.kasmos/` with legacy migration

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Centralize all kasmos config and state into the project-local `.kasmos/` directory (instead of `~/.config/kasmos/`) so that multiple OS users on the same repository (e.g. openfos via systemd) share config and state through the filesystem.
**Architecture:** `GetConfig...

