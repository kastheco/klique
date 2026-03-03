# Session Context

## User Prompts

### Prompt 1

Implement docs/plans/02b-rewrite-overlay-ui.md using the `kasmos-coder` skill. Execute all tasks sequentially.

Reviewer feedback from previous round:
Round 1 — changes required.

## critical

- `ui/overlay/manager.go:21-29` — `Manager.Show()` calls `o.SetSize(m.w, m.h)` which overwrites every overlay's constructor-set width with the full viewport dimensions. This causes all overlays to render at full viewport width instead of their intended sizes (ConfirmationOverlay: 50→viewport, Permission...

