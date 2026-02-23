# Priority Roadmap — 2026-02-22

> Generated from analysis of all unfinished plans in `plan-state.json`. Builds clean, all tests pass.

## Project State

Klique is a Go TUI (~22.6K lines, 113 Go files) for orchestrating multiple AI agent sessions (Claude Code) with plan-driven workflows, tmux-backed process management, and a tree-mode sidebar.

Over Feb 20-22, ~30 plans shipped: init wizard, plan/task orchestration, tool discovery, sidebar toggle/tree-mode, rose pine moon theme, OSC11 terminal bg detection, global skill sync, topic collision domains, opencode.jsonc generation, tab focus ring, and more.

## Unfinished Plans (11 remaining)

| # | Plan | Status | Has Plan Doc? | Complexity |
|---|------|--------|---------------|------------|
| 1 | `tab-focus-ring` | reviewing | full | **Already merged** (PR #9) — mark done |
| 2 | `dupe-task-after-plan-change` | ready | desc only | Medium-high (state machine bug) |
| 3 | `wave-orchestration` | ready | full (7 tasks, 3 waves) | **High** — core orchestration feature |
| 4 | `rename-to-kasmos` | planning | full (4 phases, 7 parallel agents) | **High** — touches every file |
| 5 | `update-shortcut-hints` | ready | desc only | **Low** — label text changes |
| 6 | `center-col-v-align` | ready | desc only | Low-medium — viewport math |
| 7 | `bubblezone` | ready | desc only | Medium — input handling integration |
| 8 | `contextual-status-bar` | ready | desc only | Medium — new UI component + state wiring |
| 9 | `detect-permissions-needed` | ready | desc only | Medium-high — runtime detection + cache behavior |
| 10 | `customize-superpowers` | ready | desc only | Medium — workflow/skill config |
| 11 | `parallelize-superpowers` | ready | desc only | Medium-high — orchestration model |

## Execution Order

### Stage 0 — Housekeeping (this session, 2 min)

- Mark `tab-focus-ring` → `done` in plan-state.json (already merged via PR #9)

### Stage 1 — Bug Fix + Quick Win (parallel, this session)

| Track A | Track B |
|---------|---------|
| **`dupe-task-after-plan-change`** | **`update-shortcut-hints`** |
| Bug fix — duplicate instances appearing when marking plans done. Fix BEFORE wave orchestration adds more lifecycle complexity. Needs investigation + plan doc. | Trivial keybinding label changes (`D`→`K` kill, `i`→interactive, hide `P`/`c`/`R`). ~15 min. |

### Stage 2 — Core Feature: Wave Orchestration (SEPARATE session)

**`wave-orchestration`** — Full plan doc, 7 tasks, 3 waves. Highest-value remaining feature: parallel task execution within plans.

- Wave 1: planparser + instance fields (parallel)
- Wave 2: orchestrator core + validation gate + prompt builder (parallel)
- Wave 3: wiring + badge rendering (sequential)

Touches `app/`, `session/`, `config/`, `ui/` — wide blast radius. Needs own branch.

### Stage 3 — Visual Fixes (parallel, worktrees OK)

| Track A | Track B |
|---------|---------|
| **`center-col-v-align`** | **`bubblezone`** |
| Fix viewport vertical alignment. Scoped to center pane sizing. | Integrate bubblezone for mouse zones + escape from agent focus. Input handling. |

Zero file overlap. Do after wave-orchestration to avoid fixing alignment against a moving target.

### Stage 4 — UX Enhancements (parallel, separate sessions recommended)

| Track A | Track B | Track C |
|---------|---------|---------|
| **`contextual-status-bar`** | **`detect-permissions-needed`** | **`customize-superpowers`** + **`parallelize-superpowers`** |
| New top bar component. Needs design doc. | Permission indicator + live capture. Needs design doc. | Bundle — both are meta-improvements to skill/agent workflow. |

Additive features, don't block anything. Status bar benefits from wave-orchestration being done (shows wave progress).

### Stage 5 — The Big Rename (SEPARATE session, LAST)

**`rename-to-kasmos`** — 4 phases, Phase 2 dispatches 7 parallel agents across every file category.

**Must be last because:**
1. Touches every single file — creates merge conflicts with any parallel work
2. Requires manual GitHub repo rename steps first (rename `kastheco/kasmos` → `kasmold`, then `klique` → `kasmos`)
3. All feature branches need rebasing after
4. Best at a clean integration point when all work is merged to main

## Visual Summary

```
NOW        Stage 0: mark tab-focus-ring done                    [this session]
           ─────────────────────────────────────────────
           Stage 1A: dupe-task bug fix ──────┐                  [this session]
           Stage 1B: update-shortcut-hints ──┘  (parallel)
           ─────────────────────────────────────────────
NEXT       Stage 2: wave-orchestration                          [SEPARATE session]
           ─────────────────────────────────────────────
THEN       Stage 3A: center-col-v-align ─────┐                 [worktrees OK]
           Stage 3B: bubblezone ─────────────┘  (parallel)
           ─────────────────────────────────────────────
LATER      Stage 4A: contextual-status-bar ──┐
           Stage 4B: detect-permissions ─────┤  (parallel)      [separate sessions]
           Stage 4C: superpowers bundle ─────┘
           ─────────────────────────────────────────────
LAST       Stage 5: rename-to-kasmos                            [SEPARATE session, clean main]
```

## Key Principle

The rename is a "scorched earth" operation — do it when main is clean and no feature branches are in flight. Everything else can be sequenced flexibly, but the **dupe bug must precede wave orchestration**.
