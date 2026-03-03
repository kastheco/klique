# Review Approval Gate Design

**Goal:** block auto-transition from `reviewing` → `done` and require manual user approval via popups before a plan can be marked done.

## Two Trigger Paths

### Path A — Sentinel (`review-approved` signal)

The reviewer agent completed and explicitly wrote a `review-approved` sentinel. Happy path.

1. **Popup 1**: "review approved — {plan}" → `[y] read review  [esc] dismiss`
   - `y` → select reviewer instance, enter focus mode so user can read output
   - `esc` → dismiss, plan stays in `reviewing`, revisit via context menu
2. **Popup 2** (on exiting focus mode from a reviewer with pending approval):
   `[m] merge  [p] create pr  [esc] dismiss`
   - `m` → kill plan instances, merge branch to main, FSM `ReviewApproved` → done
   - `p` → PR title input overlay → push + `gh pr create`, FSM `ReviewApproved` → done
   - `esc` → dismiss, plan stays in `reviewing`, pending approval preserved for context menu

### Path B — Reviewer death (tmux exits, no sentinel)

Ambiguous outcome — agent may have crashed or finished without writing a sentinel.

- **Popup**: "reviewer exited — approve {plan}?" → `[y] approve  [n] reject  [esc] dismiss`
  - `y` → enters Path A flow (focus mode on reviewer → Popup 2 merge/PR)
  - `n` → respawn reviewer (redo the review), plan stays in `reviewing`
  - `esc` → dismiss, stays in `reviewing`, come back via context menu

## State Tracking

- `pendingApprovals map[string]bool` on `home` struct — tracks plans with confirmed approval awaiting merge/PR choice. In-memory only; on restart, reviewer-death fallback re-triggers the popup.
- No new FSM states — plan stays in `reviewing` until merge or PR, then `ReviewApproved` fires.
- Dedicated `pendingApprovalPRAction` field for Popup 2's PR choice — avoids state collision with `pendingWaveNextAction` from wave orchestration.

## Context Menu Alternate Path

For plans in `reviewing` status, two new context menu items:

- **"review & merge"** → confirmation popup → merge branch → `ReviewApproved` → done
- **"create pr & push"** → PR title input → push + `gh pr create` → `ReviewApproved` → done

This is the escape hatch for dismissed popups or revisiting later.

## Cleanup

`pendingApprovals` entries are cleared on:
- Merge or PR creation (success path)
- Plan cancel
- Plan start-over
- Manual "mark done" via context menu
