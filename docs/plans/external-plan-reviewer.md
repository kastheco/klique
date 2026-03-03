# External Plan Reviewer Agent

## Context

Follow-up to `2026-02-27-plan-review.md` (planner self-review). This feature adds an
**external plan-reviewer agent** — a separate agent session that reviews the plan document
before implementation begins. The key use case: review a GPT-model planner's output with
Claude (or vice versa) for cross-model validation.

## Design Notes

### Why external review matters

Self-review (the prerequisite plan) catches structural issues but has a fundamental
limitation: the same model that wrote the plan reviews it. Blind spots persist. An
external reviewer using a different model/profile catches:

- Logical gaps the planner's model consistently misses
- Over-optimistic task sizing
- Architectural assumptions that don't match the codebase
- Missing edge cases in task descriptions

### Architecture sketch

1. **New agent profile: `plan_reviewer`** — configured separately from the implementation
   reviewer (`quality_review`). Users can assign a different model, harness, or temperature.
   Example: planner uses `gpt-4.1` via opencode, plan_reviewer uses `claude-opus-4-6` via claude.

2. **New FSM state or signal** — after `planner-finished`, kasmos optionally spawns a
   plan-reviewer agent instead of immediately showing the "start implementation?" dialog.
   Options:
   - New signal: `plan-review-approved-<planfile>` / `plan-review-changes-<planfile>`
   - Or reuse existing `review-approved` / `review-changes` with a plan-review context flag

3. **Plan-review prompt template** — similar to `review-prompt.md` but focused on plan
   quality rather than code quality. Checks: goal clarity, task decomposition, wave
   dependencies, TDD step completeness, file path accuracy, sizing realism.

4. **Feedback loop** — if the plan-reviewer requests changes, kasmos respawns the planner
   with the feedback (same pattern as `spawnCoderWithFeedback` but for the planner).

5. **Config toggle** — `plan_review.enabled` in kasmos config. When disabled (default),
   the flow is unchanged (self-review only). When enabled, the external reviewer is
   spawned automatically after the planner signals completion.

### Config example

```toml
[profiles.plan_reviewer]
enabled = true
harness = "claude"
model = "claude-opus-4-6"
# temperature, effort, etc. all configurable independently
```

### FSM impact

Current: `planning → (planner-finished) → ready → (implement-start) → implementing`

With external plan review:
`planning → (planner-finished) → plan_reviewing → (plan-review-approved) → ready → ...`
`planning → (planner-finished) → plan_reviewing → (plan-review-changes) → planning → ...`

This requires:
- New `StatusPlanReviewing` state
- New events: `PlanReviewApproved`, `PlanReviewChangesRequested`
- New signal prefixes: `plan-review-approved-`, `plan-review-changes-`
- Transition table additions

### Files likely affected

- `config/planfsm/fsm.go` — new state + events + transitions
- `config/planfsm/signals.go` — new sentinel prefixes
- `app/app_state.go` — `spawnPlanReviewer()`, plan-review signal handling
- `app/app.go` — signal dispatch in `metadataResultMsg` handler
- `config/config.go` — `plan_reviewer` profile support
- `internal/initcmd/wizard/` — wizard step for plan_reviewer agent config
- `internal/initcmd/scaffold/templates/` — plan-review prompt template
- `.opencode/skills/kasmos-planner/SKILL.md` — update signaling docs
- New: `.opencode/skills/kasmos-plan-reviewer/SKILL.md` — dedicated skill

## Notes

- Created as follow-up stub by plan-review planning session
- Prerequisite: `2026-02-27-plan-review.md` (planner self-review) must be implemented first
- This plan needs full design exploration and task decomposition before implementation
