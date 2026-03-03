# Session Context

## User Prompts

### Prompt 1

Implement Task 2: Increment ReviewCycle on FSM transition and thread to spawners

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Include review cycle numbers in instance titles, tmux session names, and opencode session labels (e.g. `-review-1`, `-fix-1`, `-review-2`) so each review/fix cycle gets unique identifiers, preventing tmux session clashes, stale caches, and opencode DB collisions.
**Architecture:** Ad...

