---
name: executing-plans
description: Use when you have a written implementation plan to execute in a separate session with review checkpoints
---

# Executing Plans

## Overview

Load plan, review critically, execute tasks in batches, report for review between batches.

**Core principle:** Batch execution with checkpoints for architect review.

**Announce at start:** "I'm using the executing-plans skill to implement this plan."

## The Process

### Step 1: Load and Review Plan
1. Read plan file
2. Review critically - identify any questions or concerns about the plan
3. If concerns: Raise them with your human partner before starting
4. If no concerns: **Create TodoWrite** with one todo per task (all `pending`), then proceed

```
TodoWrite([
  { content: "Task 1: [Component Name]", status: "pending", priority: "high" },
  { content: "Task 2: [Component Name]", status: "pending", priority: "high" },
  ...
])
```

### Step 2: Execute Batch
**Default: First 3 tasks**

For each task:
1. Mark as in_progress in TodoWrite
2. Follow each step exactly (plan has bite-sized steps)
3. Run verifications as specified
4. Mark as completed in TodoWrite

### Step 3: Report
When batch complete:
- Show what was implemented
- Show verification output
- Say: "Ready for feedback."

### Step 4: Continue
Based on feedback:
- Apply changes if needed
- Execute next batch
- Repeat until complete

### Step 5: Complete Development

After all tasks complete and verified, **signal that implementation is done.**

Check whether you're running under kasmos orchestration:

```bash
echo "${KASMOS_MANAGED:-}"
```

**If `KASMOS_MANAGED=1`:** Write a sentinel file — kasmos transitions the plan automatically.

```bash
touch docs/plans/.signals/implement-finished-<date>-<name>.md
```

**If `KASMOS_MANAGED` is unset:** Update `plan-state.json` directly — set the plan's
status to `"reviewing"`.

Then:
- Announce: "I'm using the finishing-a-development-branch skill to complete this work."
- **REQUIRED SUB-SKILL:** Use superpowers:finishing-a-development-branch
- Follow that skill to verify tests, present options, execute choice

## When to Stop and Ask for Help

**STOP executing immediately when:**
- Hit a blocker mid-batch (missing dependency, test fails, instruction unclear)
- Plan has critical gaps preventing starting
- You don't understand an instruction
- Verification fails repeatedly

**Ask for clarification rather than guessing.**

## When to Revisit Earlier Steps

**Return to Review (Step 1) when:**
- Partner updates the plan based on your feedback
- Fundamental approach needs rethinking

**Don't force through blockers** - stop and ask.

## Remember
- Review plan critically first
- Follow plan steps exactly
- Don't skip verifications
- Reference skills when plan says to
- Between batches: just report and wait
- Stop when blocked, don't guess
- Never start implementation on main/master branch without explicit user consent

## Integration

**Required workflow skills:**
- **superpowers:using-git-worktrees** - REQUIRED: Set up isolated workspace before starting
- **superpowers:writing-plans** - Creates the plan this skill executes
- **superpowers:finishing-a-development-branch** - Complete development after all tasks
