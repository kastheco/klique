---
name: writing-plans
description: Use when you have a spec or requirements for a multi-step task, before touching code
---

# Writing Plans

## Overview

Write comprehensive implementation plans assuming the engineer has zero context for our codebase and questionable taste. Document everything they need to know: which files to touch for each task, code, testing, docs they might need to check, how to test it. Give them the whole plan as right-sized tasks. DRY. YAGNI. TDD. Frequent commits.

Assume they are a skilled developer, but know almost nothing about our toolset or problem domain. Assume they don't know good test design very well.

**Announce at start:** "I'm using the writing-plans skill to create the implementation plan."

**Context:** This should be run in a dedicated worktree (created by brainstorming skill).

**Save plans to:** `docs/plans/YYYY-MM-DD-<feature-name>.md`

## Feature Sizing — Do This First

Before writing ANY tasks, classify the feature by estimated total implementation effort. This determines task count, wave structure, and review overhead.

| Size | Estimated effort | Max tasks | Waves | Review model |
|------|-----------------|-----------|-------|--------------|
| **Trivial** | < 30 min | 1 | 1 (## Wave 1 required) | Self-review only |
| **Small** | 30 min – 2 hours | 2–3 | 1 (## Wave 1 required) | Single review after all tasks |
| **Medium** | 2–6 hours | 3–6 | 1–2 waves | Review per wave |
| **Large** | 6+ hours | 6–12 | 2–4 waves | Review per wave |

**Sizing rules:**
- **Every task should represent 15–45 minutes of cohesive work.** A task that takes < 10 minutes to implement is too small — merge it into an adjacent task.
- **Never split tightly coupled work across tasks.** If Task B can't be tested without Task A's output, they belong in one task.
- **A "task" is a commit-worthy unit** — it should leave the codebase in a compilable, testable state.
- **Waves exist for dependency ordering**, not for grouping small tasks. If all tasks are independent, don't use waves — use a flat list.
- **One-line changes, dead code removal, render wiring, and config updates are NOT standalone tasks.** Bundle them into the task they serve.

**Include the size classification in the plan header** so executors know the expected overhead:

```markdown
**Size:** Small (estimated ~1 hour, 3 tasks, no waves)
```

## Task Granularity

Within each task, steps follow TDD:

**Each step is one action:**
- "Write the failing test" - step
- "Run it to make sure it fails" - step
- "Implement the minimal code to make the test pass" - step
- "Run the tests and make sure they pass" - step
- "Commit" - step

But **the task itself should be a meaningful chunk of work** — not a single function or a single case-statement addition. A good task touches 1–3 files in a cohesive way and produces a testable behavioral change.

**Anti-patterns (do NOT create tasks like these):**
- "Add one case to a switch statement" — merge into the task that needs it
- "Remove dead code" — do it in the task that replaces it
- "Update help text" — do it in the task that adds the feature
- "Run the full test suite" — that's a step, not a task

## Wave Structure

Waves create review checkpoints and dependency barriers. They have real cost (agent handoffs, review cycles), so use them only when justified.

**When to use waves:**
- Tasks in Wave 2 genuinely depend on Wave 1 being complete and reviewed
- Different subsystems need sequential integration (e.g., data layer before UI)
- Risk isolation: risky foundation work should be reviewed before building on it

**Minimum wave requirement (CRITICAL):**
**Every plan must have at least `## Wave 1`.** kasmos uses wave headers for orchestration — a plan without any `## Wave N` section cannot be implemented through kasmos. Even a single-task trivial plan must wrap its task under `## Wave 1`.

**When NOT to use multiple waves:**
- All tasks are independent (use a single ## Wave 1 with all tasks)
- Feature is small (< 3 tasks)
- The "dependency" is just file-level imports (the compiler catches that)

**Every wave boundary must state its justification:**
```markdown
## Wave 2: App Integration
> **Depends on Wave 1:** Form overlay and key constants must exist before wiring the handler.
```

## Plan Document Header

**Every plan MUST start with this header:**

```markdown
# [Feature Name] Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** [One sentence describing what this builds]

**Architecture:** [2-3 sentences about approach]

**Tech Stack:** [Key technologies/libraries]

**Size:** [Trivial/Small/Medium/Large] (estimated ~[time], [N] tasks, [N] waves or "no waves")

---
```

## Task Structure

````markdown
### Task N: [Component Name]

**Files:**
- Create: `exact/path/to/file.py`
- Modify: `exact/path/to/existing.py:123-145`
- Test: `tests/exact/path/to/test.py`

**Step 1: Write the failing test**

```python
def test_specific_behavior():
    result = function(input)
    assert result == expected
```

**Step 2: Run test to verify it fails**

Run: `pytest tests/path/test.py::test_name -v`
Expected: FAIL with "function not defined"

**Step 3: Write minimal implementation**

```python
def function(input):
    return expected
```

**Step 4: Run test to verify it passes**

Run: `pytest tests/path/test.py::test_name -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tests/path/test.py src/path/file.py
git commit -m "feat: add specific feature"
```
````

## Remember
- Exact file paths always
- Complete code in plan (not "add validation")
- Exact commands with expected output
- Reference relevant skills with @ syntax
- DRY, YAGNI, TDD, frequent commits
- **Right-size tasks to 15–45 min each — never micro-tasks**
- **Justify every wave boundary with a real dependency**

## After Saving the Plan

Do all three of these steps immediately after writing the plan file. Do not skip any of them.

**Step 1: Call `TodoWrite` to populate the session task list.**

Create one todo per `### Task N:` in the plan, all with status `pending` and appropriate priority:

```
TodoWrite([
  { content: "Task 1: [Component Name]", status: "pending", priority: "high" },
  { content: "Task 2: [Component Name]", status: "pending", priority: "high" },
  ...
])
```

**Step 2: Register the plan.**

Check whether you're running under kasmos orchestration:

```bash
echo "${KASMOS_MANAGED:-}"
```

**If `KASMOS_MANAGED=1` (running inside kasmos):** Write a sentinel file. kasmos monitors
`docs/plans/.signals/` and will register the plan automatically.

```bash
touch docs/plans/.signals/planner-finished-<date>-<name>.md
```

The filename must match the plan filename exactly. **Do not edit `plan-state.json` directly.**

**If `KASMOS_MANAGED` is unset (raw terminal):** Register the plan in `plan-state.json`
directly. Read the file first, then add an entry with `"status": "ready"`:

```json
"<date>-<name>.md": { "status": "ready" }
```

**Step 3: Commit the plan.**

```bash
git add docs/plans/<date>-<name>.md docs/plans/plan-state.json
git commit -m "plan: <feature name>"
```

Do not commit sentinel files — they are consumed and deleted by kasmos.

## Execution Handoff

Check the execution context before offering choices:

```bash
echo "${KASMOS_MANAGED:-}"
```

**If `KASMOS_MANAGED=1` (running inside kasmos):**

Your job is done. The sentinel file written in Step 2 above signals kasmos that planning is
complete. kasmos will automatically prompt the user to begin implementation. **Do not offer
execution choices — just announce completion and stop:**

> "Plan complete and saved to `docs/plans/<filename>.md`. kasmos will prompt you to start implementation."

**If `KASMOS_MANAGED` is unset (raw terminal):**

Offer execution choice:

**"Plan complete and saved to `docs/plans/<filename>.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between waves, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?"**

**If Subagent-Driven chosen:**
- **REQUIRED SUB-SKILL:** Use superpowers:subagent-driven-development
- Stay in this session
- Fresh subagent per task + review per wave

**If Parallel Session chosen:**
- Guide them to open new session in worktree
- **REQUIRED SUB-SKILL:** New session uses superpowers:executing-plans
