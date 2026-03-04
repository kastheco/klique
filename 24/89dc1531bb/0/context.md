# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Add project skills section to `skills list`

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Fix `kas skills list` bug where symlinked skill directories are silently skipped due to `DirEntry.IsDir()` returning false for symlinks, and add project-level skill listing.
**Architecture:** The `skills list` command in `skills.go` uses `os.ReadDir` + `entry.IsDir()` which filters out symlinks-to-dire...

