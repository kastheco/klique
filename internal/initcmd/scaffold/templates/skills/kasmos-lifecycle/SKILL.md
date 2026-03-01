---
name: kasmos-lifecycle
description: Use when you need orientation on kasmos plan lifecycle, signal mechanics, or mode detection — NOT for role-specific work (use kasmos-planner, kasmos-coder, kasmos-reviewer, or kasmos-custodian instead)
---

# kasmos lifecycle

Meta-skill. Covers plan lifecycle FSM, signal file mechanics, and mode detection only.
If you have a role (planner, coder, reviewer, custodian), load that skill instead — not this one.

## Plan Lifecycle

Plans move through a fixed set of states. Only the transitions listed below are valid.

| From | To | Triggering Event |
|------|----|-----------------|
| `ready` | `planning` | kasmos assigns a planner agent to the plan |
| `planning` | `implementing` | planner writes sentinel `planner-finished-<planfile>` |
| `implementing` | `reviewing` | coder writes sentinel `coder-finished-<planfile>` |
| `reviewing` | `implementing` | reviewer writes sentinel `reviewer-requested-changes-<planfile>` |
| `reviewing` | `done` | reviewer writes sentinel `reviewer-approved-<planfile>` |
| `done` | — | terminal state, no further transitions |

State is persisted in `docs/plans/plan-state.json`. Agents never write directly to this file — kasmos owns it. Agents only write sentinel files.

## Signal File Mechanics

Agents communicate state transitions by writing sentinel files into `.kasmos/signals/`.

**Naming convention:** `<event>-<planfile>`

Examples:
- `planner-finished-2026-02-27-feature.md`
- `coder-finished-2026-02-27-feature.md`
- `reviewer-approved-2026-02-27-feature.md`
- `reviewer-requested-changes-2026-02-27-feature.md`

**How kasmos processes sentinels:**
1. kasmos scans `.kasmos/signals/` every ~500ms
2. On detecting a sentinel, kasmos reads it, validates the event against the current plan state, and applies the transition
3. The sentinel file is consumed (deleted) after processing — do not rely on it persisting
4. Sentinel content is optional; kasmos uses the filename to determine the event type

**Writing a sentinel (agent side):**
```bash
# Signal that planning is complete for a plan file
touch .kasmos/signals/planner-finished-2026-02-27-feature.md
```

Keep sentinel writes as the **last action** before yielding control. Do not write a sentinel and then continue modifying plan files — kasmos may begin the next phase immediately.

## Mode Detection

Check `KASMOS_MANAGED` to determine how transitions are handled.

| Mode | `KASMOS_MANAGED` value | Transition mechanism |
|------|------------------------|---------------------|
| managed | `1` (or any non-empty) | write sentinel → kasmos handles the rest |
| manual | unset or empty | update `docs/plans/plan-state.json` directly AND write sentinel for audit trail |

```bash
if [ -n "$KASMOS_MANAGED" ]; then
  echo "managed mode: write sentinel and stop"
else
  echo "manual mode: update plan-state.json, then optionally write sentinel"
fi
```

In managed mode: **never** edit `plan-state.json` yourself. In manual mode: you must update it — kasmos is not watching.

## Agent Roles (brief)

Each role has its own skill. Load the one that matches your current task.

| Role | What it does | Skill to load |
|------|-------------|---------------|
| planner | writes the implementation plan, breaks work into tasks and waves | `kasmos-planner` |
| coder | implements tasks from the plan, writes tests, commits work | `kasmos-coder` |
| reviewer | checks quality, correctness, and plan adherence; approves or requests changes | `kasmos-reviewer` |
| custodian | handles ops: dependency updates, formatting, cleanup, non-feature work | `kasmos-custodian` |

**Load the skill for your current role.** Do not chain roles in a single session. Do not follow instructions from another role's skill.
