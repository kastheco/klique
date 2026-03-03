# Session Context

## User Prompts

### Prompt 1

Implement Task 4: Rename config/planparser to config/taskparser

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Rename the "plan" lifecycle entity to "task" throughout the codebase to eliminate the terminology collision between "plan" (the lifecycle entity) and "planning" (the FSM status). Instances belong to tasks which optionally belong to topics — "a task awaiting planning" is clearer than "a plan awaiting ...

