---
description: Force-reset a plan's status (bypasses FSM)
agent: custodial
---

# /kas.reset-plan

Force-override a plan's status, bypassing normal FSM transition rules.

## Arguments

```
$ARGUMENTS
```

Expected format: `<plan-file> <status>`

Example: `/kas.reset-plan 2026-02-20-my-plan.md ready`

## Process

1. Parse arguments into plan filename and target status
2. If arguments are missing or malformed, show usage and list current plans:
   ```bash
   kas plan list
   ```
3. Show current status before changing:
   ```bash
   kas plan list | rg "<plan-file>"
   ```
4. Execute the override:
   ```bash
   kas plan set-status <plan-file> <status> --force
   ```
5. Confirm the change:
   ```bash
   kas plan list | rg "<plan-file>"
   ```

## Valid statuses
ready, planning, implementing, reviewing, done, cancelled
