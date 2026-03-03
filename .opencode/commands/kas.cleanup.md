---
description: Remove stale worktrees, orphan branches, and ghost plan entries
agent: custodial
---

# /kas.cleanup

Three-pass cleanup of worktrees, branches, and plan state.

## Arguments

```
$ARGUMENTS
```

Optional flags: `--execute` to actually delete (default is dry-run).

## Process

### Pass 1: Stale worktrees

Find worktrees whose plan is done or cancelled:

```bash
git worktree list
kas plan list
```

Cross-reference: any worktree on a `plan/*` branch where the plan status is `done` or `cancelled` is stale.

### Pass 2: Orphan branches

Find local `plan/*` branches with no matching entry in the plan store:

```bash
git branch --list 'plan/*'
kas plan list
```

### Pass 3: Ghost plan entries

Find entries in the plan store with no corresponding branch or worktree:

```bash
kas plan list
git branch --list 'plan/*'
```

### Output

Report findings grouped by pass. If `--execute` was specified, remove stale worktrees
(`git worktree remove`), delete orphan branches (`git branch -d`), and flag ghost entries.

If dry-run (default), just list what would be cleaned.
