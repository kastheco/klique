# Session Context

## User Prompts

### Prompt 1

You are the elaborator agent. Your job: enrich a plan's task descriptions with detailed implementation instructions so coder agents make fewer decisions.

Load the `kasmos-elaborator` skill before starting. Also load `cli-tools`.

## Instructions

1. Retrieve the plan: `kas task show imports-not-checked-for-waves.md`
2. For each task, read the codebase files listed in its **Files:** section. Study existing patterns, interfaces, function signatures, error handling, and data flow in those files...

