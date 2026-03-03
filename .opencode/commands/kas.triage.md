---
description: Bulk scan and triage non-terminal plans
agent: custodial
---

# /kas.triage

Scan all non-done/cancelled plans and present them for triage.

## Arguments

```
$ARGUMENTS
```

Optional: specific status to triage (e.g., `ready`, `implementing`).

## Process

1. List all active plans:
   ```bash
   kas plan list
   ```
2. For each non-terminal plan (not done/cancelled), gather context:
   - Branch existence: `git branch --list '<branch>'`
   - Worktree existence: `git worktree list | rg '<branch>'`
   - Last commit on branch: `git log <branch> -1 --format='%ar - %s' 2>/dev/null`
   - Plan registered in store: `kas plan list | rg '<filename>'`
3. Present grouped by status:
   ```
   ## ready (N plans)
   - plan-name.md — branch: plan/name, worktree: yes/no, last commit: 2d ago
   ...

   ## implementing (N plans)
   ...
   ```
4. For each group, ask what to do:
   - ready plans: "implement, cancel, or skip?"
   - implementing plans: "the branch may be stale. cancel, reset to ready, or skip?"
   - reviewing plans: "mark done, reset to implementing, or skip?"
5. Execute chosen actions via `kas plan set-status --force` or `kas plan transition`.
