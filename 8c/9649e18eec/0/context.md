# Session Context

## User Prompts

### Prompt 1

Implement Task 1: Fix Planner Skill Wave-Splitting Guidance

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Prevent the planner from putting import-dependent tasks in the same wave, and give coder agents guidance when parallel tasks hit build failures from unresolved cross-task imports.
**Architecture:** Two-pronged fix: (1) update the kasmos-planner skill template to treat import/type dependencies as real wav...

