# kasmos-native skills design

## problem

the current skill system is a hybrid mess. 15 superpowers skills (designed for single-agent
claude code sessions) are patched with `KASMOS_MANAGED` env var branching to work in kasmos's
multi-agent orchestrated context. every skill checks the env var independently, agents load
2-4 skills with chain dispatches, and cross-role contamination means coders get planner
instructions, reviewers get implementation guidance, etc.

specific issues:
- **15 skills with scattered branching** — each independently checks `KASMOS_MANAGED`
- **cross-role contamination** — coder loads executing-plans (which discusses worktree creation,
  wave orchestration, branch finishing — none of which are the coder's job)
- **skill chains** — brainstorming → writing-plans → executing-plans → finishing-branch,
  each a separate dispatch costing context window
- **redundant lifecycle management** — skills AND kasmos both try to manage worktrees, waves,
  reviews, and branch finishing
- **4 overlapping review systems** — requesting-code-review, subagent-driven-development,
  kasmos reviewer instance, pr-review-toolkit
- **cli-tools enforcement is weak** — agents told to "MUST read cli-tools SKILL.md at session
  start" but it's a separate load that gets skipped

## solution

replace all lifecycle-related superpowers skills with 5 kasmos-native skills. each of the 4
agent roles (planner, coder, reviewer, custodial) loads exactly 1 role skill. a lightweight
lifecycle skill exists for orientation in ad-hoc sessions.

### design principles

1. **one skill per role** — agent loads 1 skill, gets everything it needs
2. **dual-mode** — each skill works in both managed (kasmos TUI) and manual (raw terminal)
   sessions. mode check happens once, in a compact signaling section at the end
3. **embedded disciplines** — TDD, debugging, verification are inline where needed, not
   separate dispatch targets
4. **embedded cli-tools hard gate** — banned-tools table + tool selection table copied
   inline into every role skill. deep references (ast-grep.md, comby.md, etc.) stay as
   on-demand reads in the cli-tools skill's resources/
5. **no cross-role instructions** — planner skill has zero implementation guidance, coder
   skill has zero plan-writing guidance

### what changes between modes

| concern | managed (`KASMOS_MANAGED=1`) | manual (unset) |
|---|---|---|
| signaling | sentinel files in `.signals/` | direct plan-state.json edits |
| worktree | already created by kasmos | agent creates via git worktree |
| wave orchestration | kasmos spawns N agents per wave | agent executes sequentially |
| review | kasmos spawns reviewer instance | agent self-reviews or dispatches subagent |
| branch finishing | kasmos context menu (merge/PR) | agent offers merge/PR/keep/discard |
| execution handoff | stop after signaling | offer next-step choices |

### what's identical in both modes

- plan format, sizing, task structure, wave headers
- TDD discipline (Red-Green-Refactor, iron law)
- debugging discipline (4-phase root cause approach)
- verification discipline (evidence before claims)
- review checklist (spec + quality, all tiers blocking, self-fix protocol)
- shared worktree safety (when peers exist)
- commit conventions
- cli-tools hard gate (banned tools, tool selection)

## skill inventory

### 1. kasmos-lifecycle (~80 lines)

lightweight meta-skill. NOT a router/dispatcher — just context for agents that need
orientation (ad-hoc sessions, chat agent).

contents:
- plan lifecycle FSM: `ready → planning → implementing → reviewing → done`
- valid transitions and events (one table)
- signal file mechanics: agents write sentinels in `docs/plans/.signals/`, kasmos scans ~500ms
- mode detection: check `KASMOS_MANAGED`. if set, kasmos manages transitions. if unset, self-manage.
- brief role descriptions (planner, coder, reviewer, custodial)

NOT loaded by role-specific agents — they get lifecycle context from their own skill's intro.

### 2. kasmos-planner (~250 lines)

replaces: `brainstorming` + `writing-plans`

sections:
- **cli-tools hard gate** (banned-tools + tool selection inline)
- **where you fit** — brief FSM context, your work does ready → planning → ready
- **design exploration** — explore context, clarifying questions, 2-3 approaches, approval
- **plan document format** — header (Goal, Architecture, Tech Stack, Size), sizing table,
  `## Wave N` sections, `### Task N:` within waves, granularity rules, TDD-structured steps
- **after writing the plan** — TodoWrite, commit
- **signaling** — managed: planner-finished sentinel, stop. manual: register in plan-state.json,
  commit, offer execution choices.

### 3. kasmos-coder (~350 lines)

replaces: `executing-plans` + `subagent-driven-development` + `test-driven-development` +
`systematic-debugging` + `verification-before-completion` + `receiving-code-review`

sections:
- **cli-tools hard gate** (banned-tools + tool selection inline)
- **where you fit** — managed: you implement ONE task (KASMOS_TASK). manual: execute plan sequentially.
- **TDD discipline** — RED-GREEN-REFACTOR, iron law, embedded directly
- **shared worktree safety** — when KASMOS_PEERS > 0: no git add ., specific files only,
  task-numbered commits
- **debugging discipline** — 4-phase root cause approach, max 3 attempts, parallel-aware
- **verification** — evidence before claims, run full tests, check exit codes
- **handling reviewer feedback** — evaluate technically, fix one at a time, test each
- **signaling** — managed: implement-finished sentinel, stop. manual: execute waves sequentially,
  self-review between waves, write sentinel OR update plan-state.json, handle branch finishing.

### 4. kasmos-reviewer (~200 lines)

replaces: `requesting-code-review` + `receiving-code-review` + review prompt template

sections:
- **cli-tools hard gate** (banned-tools + tool selection inline)
- **where you fit** — review branch diff only (git diff main..HEAD or difft)
- **review checklist** — spec compliance + code quality
- **self-fix protocol** — trivial = commit directly, complex = kick to coder
- **all tiers blocking** — Critical, Important, Minor all must resolve. round tracking.
- **verification** — run tests before approving
- **signal format** — review-approved or review-changes with structured heredoc. managed: stop.
  manual: additionally offer merge/PR/keep/discard.

### 5. kasmos-custodial (~150 lines)

new skill for the custodial agent (from `plan/kasmos-custodial-agent` branch).

sections:
- **cli-tools hard gate** (banned-tools + tool selection inline)
- **where you fit** — ops/janitor, NOT feature work
- **available CLI commands** — `kas plan list/set-status/transition/implement`
- **available slash commands** — `/kas.reset-plan`, `/kas.finish-branch`, `/kas.cleanup`,
  `/kas.implement`, `/kas.triage`
- **cleanup protocol** — stale worktrees, orphan branches, ghost plan entries
- **safety rules** — force flag required, confirm destructive ops, never modify plan content

## what gets removed

these superpowers skills are no longer scaffolded or referenced by kasmos:

| removed skill | knowledge goes to |
|---|---|
| using-superpowers | kasmos-lifecycle + agent role templates |
| brainstorming | kasmos-planner (design section) |
| writing-plans | kasmos-planner (format section) |
| executing-plans | kasmos-coder (execution section) |
| subagent-driven-development | kasmos-coder (manual mode) + kasmos native |
| dispatching-parallel-agents | kasmos native (wave orchestration) |
| test-driven-development | kasmos-coder (TDD section) |
| systematic-debugging | kasmos-coder (debugging section) |
| requesting-code-review | kasmos-reviewer (checklist section) |
| receiving-code-review | kasmos-coder (feedback section) |
| using-git-worktrees | kasmos-coder (manual mode) + kasmos native |
| verification-before-completion | all 3 role skills (verification section) |
| finishing-a-development-branch | kasmos-coder/kasmos-reviewer (manual mode) |

kept: `cli-tools` (deep reference files still needed), `writing-skills` (meta, not lifecycle)

## multi-harness distribution

skills are authored in the kasmos repo and distributed to all 3 harnesses:

```
authoritative source (kasmos repo):
  .claude/skills/kasmos-*/SKILL.md
  internal/initcmd/scaffold/templates/skills/kasmos-*/SKILL.md

kas skills sync → symlinks to all harnesses:
  ~/.agents/skills/kasmos-*               ← canonical global
  ~/.claude/skills/kasmos-*               → symlink
  ~/.config/opencode/skills/kasmos-*      → symlink

kas init → scaffolds into target project:
  <project>/.claude/skills/kasmos-*
  <project>/.opencode/skills/kasmos-*     → symlink to ../../.agents/skills/
```

## code changes required

### prompt builders (app/app_state.go, app/wave_prompt.go)

- `buildPlanPrompt()`: "use the `kasmos-planner` skill."
- `buildTaskPrompt()`: "use the `kasmos-coder` skill."
- `buildImplementPrompt()`: "use the `kasmos-coder` skill."
- `buildWaveAnnotationPrompt()`: reference kasmos-planner for format

### review prompt template (internal/initcmd/scaffold/templates/shared/review-prompt.md)

- "use the `kasmos-reviewer` skill." (replaces requesting-code-review reference)
- simplify template since the skill embeds the full protocol

### agent role templates (claude + opencode + codex)

each role loads exactly 1 kasmos skill:

- `coder.md`: "load the `kasmos-coder` skill."
- `planner.md`: "load the `kasmos-planner` skill."
- `reviewer.md`: "load the `kasmos-reviewer` skill."
- `custodial.md`: "load the `kasmos-custodial` skill."
- `chat.md`: no kasmos skill required (can load kasmos-lifecycle if needed)

same changes for claude, opencode, and codex agent templates.

### scaffold pipeline (internal/initcmd/scaffold/)

replace scaffolded skills:
- remove: writing-plans, executing-plans, subagent-driven-development, requesting-code-review,
  finishing-a-development-branch
- add: kasmos-lifecycle, kasmos-planner, kasmos-coder, kasmos-reviewer, kasmos-custodial
- keep: cli-tools (deep references still needed)

### opencode commands (custodial branch)

- update `kas.*.md` references from superpowers to kasmos-custodial
- ensure `kas` binary name (not `kq`)

### kas skills sync (internal/initcmd/harness/sync.go)

- ensure new kasmos-* skills get synced to all harness global dirs
- no structural changes needed if canonical copies are in ~/.agents/skills/
