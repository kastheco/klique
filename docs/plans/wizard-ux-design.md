# Wizard UX Overhaul Design

## Problem

The `kq init` wizard runs 11+ separate `huh.NewForm().Run()` calls that clear the terminal
between each. Previous answers become invisible. The opencode model list (50+ items) fills the
entire terminal in a default-height Select. You lose track of which agent you're configuring
and what you already chose.

## Solution

One stacked form per agent with a Note header showing progress, plus `.Height(8).Filterable(true)`
on model Selects. Reduces screen clears from 11+ to 5, keeps full context visible.

## Screen Flow

```
[1. Harness selection]
[2. Coder config     — stacked, Note: "Harnesses: claude, opencode"]
[3. Reviewer config  — stacked, Note: "✓ coder: opencode / sonnet-4-6 / high"]
[4. Planner config   — stacked, Note: "✓ coder: ... | ✓ reviewer: ..."]
[5. Phase mapping    — stacked, Note: full agent summary]
```

## Per-Agent Form Layout

```
┌──────────────────────────────────────────────────┐
│  Configure: reviewer                              │
│                                                   │
│  ✓ coder    opencode / claude-sonnet-4-6 / high  │
│  ▸ reviewer configuring...                        │
│  ○ planner                                        │
│                                                   │
│  Harness   ▸ opencode                            │
│  Enabled   ▸ Yes                                 │
│  Model     ▸ claude-sonnet-4-6           /filter │
│  Temp      ▸ _                                   │
│  Effort    ▸ high                                │
│                                                   │
│            enter confirm  •  esc back             │
└──────────────────────────────────────────────────┘
```

- Note header: `✓` done, `▸` current, `○` pending
- Model Select: `.Height(8).Filterable(true)` — 8 visible rows, type to filter
- All fields visible simultaneously in one form
- Conditional fields: if Enabled=false skip rest, if no temp support omit field
- Theme: `huh.ThemeCharm()`

## File Changes

| File | Change |
|------|--------|
| `stage_agents.go` | Merge 3 forms into 1 stacked form per agent with Note header |
| `stage_harness.go` | Add ThemeCharm |
| `stage_phases.go` | Add Note header with agent summary, ThemeCharm |
| `wizard.go` | No change |

## What Doesn't Change

- Stage execution order, State struct, ToTOMLConfig, ToAgentConfigs
- One form.Run() per agent (1 instead of 3)
- Conditional field logic
