# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Ingest Content During CLI Registration

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Eliminate remaining disk reads of plan `.md` files in the TUI app layer, making the task store database the single source of truth for plan content. Three production callsites in `app/` still read plan content from `docs/plans/` on disk instead of the database — migrate them to use `m.taskStore.GetContent()...

