# Session Context

## User Prompts

### Prompt 1

The plan cli-plan-lifecycle.md is missing ## Wave N headers required for kasmos wave orchestration. Retrieve the plan content with `kas task show cli-plan-lifecycle.md`, then annotate it by wrapping all tasks under ## Wave N sections. Every plan needs at least ## Wave 1 — even single-task trivial plans. Keep all existing task content intact; only add the ## Wave headers.

After annotating:
1. Store the updated plan via `kas task update-content cli-plan-lifecycle.md` (pipe the content)
2. Sign...

