# Session Context

## User Prompts

### Prompt 1

Implement Task 3: Elaboration Prompt Builder

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Add an automatic elaboration phase between plan-ready and coder-start that expands terse task descriptions into detailed implementation instructions, reducing coder decision-making and improving output quality.
**Architecture:** When the user triggers "implement", kasmos spawns an elaborator agent before starting wave ...

