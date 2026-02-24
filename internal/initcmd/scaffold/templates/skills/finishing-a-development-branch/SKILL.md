---
name: finishing-a-development-branch
description: Use when implementation is complete, all tests pass, and you need to decide how to integrate the work - guides completion of development work by presenting structured options for merge, PR, or cleanup
---

# Finishing a Development Branch

## Overview

Guide completion of development work by presenting clear options and handling chosen workflow.

**Core principle:** Verify tests → Signal kasmos → Present options → Execute choice → Clean up.

**Announce at start:** "I'm using the finishing-a-development-branch skill to complete this work."

## The Process

### Step 1: Verify Tests

**Before presenting options, verify tests pass:**

```bash
# Run project's test suite
npm test / cargo test / pytest / go test ./...
```

**If tests fail:**
```
Tests failing (<N> failures). Must fix before completing:

[Show failures]

Cannot proceed with merge/PR until tests pass.
```

Stop. Don't proceed to Step 2.

**If tests pass:** Continue to Step 2.

### Step 2: Determine Base Branch

```bash
# Try common base branches
git merge-base HEAD main 2>/dev/null || git merge-base HEAD master 2>/dev/null
```

Or ask: "This branch split from main - is that correct?"

### Step 3: Present Options

Present exactly these 4 options:

```
Implementation complete. What would you like to do?

1. Merge back to <base-branch> locally
2. Push and create a Pull Request
3. Keep the branch as-is (I'll handle it later)
4. Discard this work

Which option?
```

**Don't add explanation** - keep options concise.

### Step 4: Execute Choice

#### Option 1: Merge Locally

```bash
# Switch to base branch
git checkout <base-branch>

# Pull latest
git pull

# Merge feature branch
git merge <feature-branch>

# Verify tests on merged result
<test command>

# If tests pass
git branch -d <feature-branch>
```

Signal kasmos that the work was approved and merged:
```bash
touch docs/plans/.signals/review-approved-<date>-<name>.md
```

Then: Cleanup worktree (Step 5)

#### Option 2: Push and Create PR

```bash
# Push branch
git push -u origin <feature-branch>

# Create PR
gh pr create --title "<title>" --body "$(cat <<'EOF'
## Summary
<2-3 bullets of what changed>

## Test Plan
- [ ] <verification steps>
EOF
)"
```

Signal kasmos that the work is out for review (approved to proceed):
```bash
touch docs/plans/.signals/review-approved-<date>-<name>.md
```

Then: Cleanup worktree (Step 5)

#### Option 3: Keep As-Is

Report: "Keeping branch <name>. Worktree preserved at <path>."

**Don't cleanup worktree. Don't write a sentinel** — the plan stays in `reviewing` status
until the user decides what to do.

#### Option 4: Discard

**Confirm first:**
```
This will permanently delete:
- Branch <name>
- All commits: <commit-list>
- Worktree at <path>

Type 'discard' to confirm.
```

Wait for exact confirmation.

If confirmed:
```bash
git checkout <base-branch>
git branch -D <feature-branch>
```

Signal kasmos that the work was discarded:
```bash
touch docs/plans/.signals/review-approved-<date>-<name>.md
```

Then: Cleanup worktree (Step 5)

### Step 5: Cleanup Worktree

**For Options 1, 2, 4:**

Check if in worktree:
```bash
git worktree list | grep $(git branch --show-current)
```

If yes:
```bash
git worktree remove <worktree-path>
```

**For Option 3:** Keep worktree.

## kasmos Sentinel Files

kasmos monitors `docs/plans/.signals/` to track plan lifecycle. The sentinel filename must
match the plan filename exactly (same `<date>-<name>.md` portion).

| Situation | Sentinel |
|-----------|----------|
| Merged locally, PR created, or discarded | `review-approved-<date>-<name>.md` |
| Reviewer requested changes before merge | `review-changes-<date>-<name>.md` |

**Do not edit `plan-state.json` directly** — kasmos owns that file. The sentinel drives the
transition automatically.

If a human reviewer requests changes after reviewing a PR, write `review-changes-<date>-<name>.md`
instead — this transitions the plan back to `implementing` so the coder can address feedback.

## Quick Reference

| Option | Merge | Push | Sentinel | Keep Worktree | Cleanup Branch |
|--------|-------|------|----------|---------------|----------------|
| 1. Merge locally | ✓ | - | review-approved | - | ✓ |
| 2. Create PR | - | ✓ | review-approved | ✓ | - |
| 3. Keep as-is | - | - | none | ✓ | - |
| 4. Discard | - | - | review-approved | - | ✓ (force) |

## Common Mistakes

**Skipping test verification**
- **Problem:** Merge broken code, create failing PR
- **Fix:** Always verify tests before offering options

**Open-ended questions**
- **Problem:** "What should I do next?" → ambiguous
- **Fix:** Present exactly 4 structured options

**Automatic worktree cleanup**
- **Problem:** Remove worktree when might need it (Option 2, 3)
- **Fix:** Only cleanup for Options 1 and 4

**No confirmation for discard**
- **Problem:** Accidentally delete work
- **Fix:** Require typed "discard" confirmation

**Forgetting the sentinel**
- **Problem:** kasmos TUI still shows plan as `reviewing` indefinitely
- **Fix:** Always write the appropriate sentinel for Options 1, 2, 4

## Red Flags

**Never:**
- Proceed with failing tests
- Merge without verifying tests on result
- Delete work without confirmation
- Force-push without explicit request
- Skip the sentinel for Options 1, 2, or 4

**Always:**
- Verify tests before offering options
- Present exactly 4 options
- Get typed confirmation for Option 4
- Write sentinel before worktree cleanup
- Clean up worktree for Options 1 & 4 only

## Integration

**Called by:**
- **subagent-driven-development** - After all tasks complete and implement-finished sentinel written
- **executing-plans** - After all batches complete and implement-finished sentinel written

**Pairs with:**
- **using-git-worktrees** - Cleans up worktree created by that skill
