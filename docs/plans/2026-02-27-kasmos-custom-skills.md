# kasmos-native skills implementation plan

> **For Claude:** REQUIRED SUB-SKILL: Use the `kasmos-coder` skill (or `executing-plans` if kasmos-coder doesn't exist yet) to implement this plan task-by-task.

**Goal:** audit and improve cli-tools deep references, then replace 15 superpowers skills with 5 kasmos-native skills (kasmos-lifecycle, kasmos-planner, kasmos-coder, kasmos-reviewer, kasmos-custodial), update all agent templates and prompt builders across claude/opencode/codex harnesses, and clean up the old scaffolded skills.

**Architecture:** 5 self-contained SKILL.md files authored in the kasmos repo's `.agents/skills/` (canonical source), with scaffold template copies in `internal/initcmd/scaffold/templates/skills/`. each role skill embeds the cli-tools hard gate (banned-tools + tool selection) inline. mode-aware signaling sections handle both managed and manual contexts. agent role templates across all 3 harnesses updated to reference exactly 1 kasmos skill per role. prompt builders in Go code updated to reference new skill names. cli-tools deep references audited for accuracy, completeness, and effective agent communication.

**Tech Stack:** markdown (SKILL.md format with YAML frontmatter), Go (prompt builders, scaffold pipeline, tests)

**Size:** Large (estimated ~6 hours, 11 tasks, 4 waves)

---

## Wave 1: Audit and Improve Deep References

> **Justification:** the cli-tools hard gate is embedded inline in every kasmos-native skill. the deep reference files (`resources/*.md`) are loaded on-demand when agents need a specific tool. before writing the 5 skills, we need to ensure the references are accurate, complete, and optimally structured for agent consumption. this wave also evaluates YAML frontmatter across all existing skills to establish best practices for the new skill descriptions.

### Task 1: Audit cli-tools SKILL.md and YAML frontmatter

**Files:**
- Modify: `.opencode/skills/cli-tools/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md`
- Modify: `.agents/skills/cli-tools/SKILL.md`

Evaluate the main cli-tools SKILL.md for:

1. **YAML frontmatter quality** — the current description is 295 chars. per the writing-skills CSO guidance, descriptions should start with "Use when..." and describe triggering conditions only (never summarize workflow). evaluate whether the current description is optimal or if it leaks workflow details that cause agents to shortcut.

2. **Hard gate effectiveness** — the `<HARD-GATE>` block is the most critical section. evaluate:
   - are the banned-tools entries complete? any missing tools that agents still reach for?
   - is the "No Exceptions" column persuasive enough? agents still use `grep` despite the ban.
   - should `find` be added to the banned list (replaced by `fd` or glob tools)?

3. **Tool selection table** — evaluate whether the "Not" column is useful or redundant given the hard gate already bans those tools. consider whether the table would be more effective as a decision tree or if the current format works.

4. **ast-grep vs comby decision table** — this is a common confusion point. evaluate whether the current table is sufficient or needs examples showing the decision boundary more clearly.

5. **Quick reference sections** — each tool has a 3-4 line quick reference plus a link to the deep reference. evaluate whether the quick references contain the right commands (most commonly needed) or if they should be adjusted.

6. **Violations table** — evaluate completeness. are there violations agents commonly commit that aren't listed?

Apply improvements directly. keep the same overall structure but tighten language, fix any inaccuracies, and ensure the hard gate is as bulletproof as possible. sync changes across all 3 copies (`.opencode/skills/`, `.agents/skills/`, and `internal/initcmd/scaffold/templates/skills/`).

**Commit:** `refactor: audit and improve cli-tools SKILL.md`

### Task 2: Audit and improve ast-grep and comby deep references

**Files:**
- Modify: `.opencode/skills/cli-tools/resources/ast-grep.md`
- Modify: `.opencode/skills/cli-tools/resources/comby.md`

These are the two most complex tools and the ones agents struggle with most. Evaluate each for:

1. **Version accuracy** — ast-grep is listed as 0.40.x, comby as 1.8.1. verify these are current or update.

2. **ast-grep.md** (158 lines):
   - metavariable syntax table — is `$$VAR` documented correctly? agents confuse `$$$` vs `$$$ARGS`.
   - common patterns section — are the Go examples the most useful ones? consider adding TypeScript/Python examples since agents work across languages.
   - rule files section — is the YAML rule example sufficient? agents often need `constraints` and `has` but these are only mentioned in passing.
   - the "When to Use" section at the bottom duplicates the main SKILL.md decision table — evaluate whether to keep, remove, or differentiate.

3. **comby.md** (157 lines):
   - the two "Critical" sections (balanced delimiters and newline collapse) are the most important content. evaluate whether they're prominent enough and whether the examples clearly show the failure mode.
   - hole syntax table — the `:[var:e]` (expression) hole is underdocumented. agents don't know when to use it vs `:[var]`.
   - safe patterns for Go — evaluate whether these cover the most common operations agents perform. consider adding "remove a function parameter" and "wrap function body in if-check" patterns.
   - anti-patterns table — evaluate completeness.
   - shell quoting section — is this necessary or does it add noise?

Apply improvements. focus on clarity for agents: make the most dangerous pitfalls impossible to miss, and ensure the most common operations have copy-paste examples.

**Commit:** `refactor: audit and improve ast-grep and comby references`

### Task 3: Audit and improve remaining deep references (difft, sd, yq, typos, scc)

**Files:**
- Modify: `.opencode/skills/cli-tools/resources/difftastic.md`
- Modify: `.opencode/skills/cli-tools/resources/sd.md`
- Modify: `.opencode/skills/cli-tools/resources/yq.md`
- Modify: `.opencode/skills/cli-tools/resources/typos.md`
- Modify: `.opencode/skills/cli-tools/resources/scc.md`

These 5 references are simpler tools. Evaluate each for:

1. **Version accuracy** — verify listed versions are current.

2. **difftastic.md** (75 lines): lean and focused. evaluate whether the git integration section covers the most common use case (reviewing branch diffs). consider adding `GIT_EXTERNAL_DIFF=difft git diff main..HEAD` as a prominent example since reviewers need this constantly.

3. **sd.md** (85 lines): evaluate whether the `-f w` (word boundary) flag is documented prominently enough — agents often do overly broad replacements. the "When to Use" section duplicates the main SKILL.md — evaluate whether to keep or differentiate.

4. **yq.md** (113 lines): the Python vs Go yq distinction is critical and currently buried in a "Note:" line. evaluate whether this should be more prominent (agents install the wrong one). the jq filter cheat sheet is useful but long — evaluate whether it belongs here or should be trimmed.

5. **typos.md** (87 lines): evaluate whether the configuration section (TOML format) is sufficient. agents often need to add false-positive exclusions but the example only shows `extend-words`.

6. **scc.md** (93 lines): evaluate whether the common operations cover what agents actually use scc for (mostly `scc` and `scc --include-ext go`). the "When to Use" section duplicates the main SKILL.md.

Apply improvements. for all 5: remove "When to Use" sections that duplicate the main SKILL.md (the main file is the decision point, not the reference), tighten language, fix version numbers, and ensure the most common operations are prominent.

**Commit:** `refactor: audit and improve difft, sd, yq, typos, scc references`

---

## Wave 2: Write the 5 kasmos-native Skills

> **Depends on Wave 1:** the cli-tools hard gate content (banned-tools table, tool selection table, violations table) is embedded inline in every kasmos-native skill. the audit in Wave 1 ensures this content is accurate before it gets copied into 5 new files. all 5 skills are independent of each other and can be written in parallel.

### Task 4: Write kasmos-lifecycle skill

**Files:**
- Create: `.agents/skills/kasmos-lifecycle/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md`

Write the lightweight meta-skill (~80 lines). Contents:

**YAML frontmatter:**
```yaml
---
name: kasmos-lifecycle
description: Use when you need orientation on kasmos plan lifecycle, signal mechanics, or mode detection — NOT for role-specific work (use kasmos-planner, kasmos-coder, kasmos-reviewer, or kasmos-custodial instead)
---
```

**Sections:**
- Plan lifecycle FSM table: `ready → planning → implementing → reviewing → done` with valid transitions and triggering events
- Signal file mechanics: agents write sentinels in `docs/plans/.signals/`, kasmos scans every ~500ms, sentinels consumed after processing. sentinel naming convention: `<event>-<planfile>` (e.g. `planner-finished-2026-02-27-feature.md`)
- Mode detection section: check `KASMOS_MANAGED` env var. managed = kasmos handles transitions via sentinels, manual = agent self-manages via plan-state.json
- Brief role descriptions (planner writes plans, coder implements tasks, reviewer checks quality, custodial does ops)
- "load the skill for your current role" — no chain dispatching, no cross-role instructions

The scaffold template should be an identical copy of the `.agents/skills/` version.

**Commit:** `feat: add kasmos-lifecycle skill`

### Task 5: Write kasmos-planner skill

**Files:**
- Create: `.agents/skills/kasmos-planner/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/kasmos-planner/SKILL.md`

Write the planner skill (~250 lines). Consolidates `brainstorming` + `writing-plans`. See the design doc at `docs/plans/2026-02-27-kasmos-native-skills-design.md` section "2. kasmos-planner" for the full specification.

Key sections:
- **CLI Tools Hard Gate** — copy the audited banned-tools table, tool selection table, and violations table from the Wave 1 output. place at the top inside `<HARD-GATE>` tags.
- **Where You Fit** — brief FSM context. your work transitions the plan from ready → planning → ready.
- **Design Exploration** — explore context, clarifying questions one at a time, 2-3 approaches with trade-offs, get approval. YAGNI ruthlessly.
- **Plan Document Format** — header with Goal/Architecture/Tech Stack/Size, sizing table (Trivial/Small/Medium/Large), `## Wave N` sections with dependency justifications, `### Task N:` within waves, granularity rules (15-45 min per task), TDD-structured steps.
- **After Writing the Plan** — TodoWrite, commit.
- **Signaling** — managed: `touch docs/plans/.signals/planner-finished-<planfile>`, do not edit plan-state.json, stop. manual: register in plan-state.json with `"status": "ready"`, commit, offer execution choices.

**Commit:** `feat: add kasmos-planner skill`

### Task 6: Write kasmos-coder skill

**Files:**
- Create: `.agents/skills/kasmos-coder/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md`

Write the coder skill (~350 lines). Consolidates `executing-plans` + `subagent-driven-development` + `test-driven-development` + `systematic-debugging` + `verification-before-completion` + `receiving-code-review`. See the design doc section "3. kasmos-coder" for the full specification.

Key sections:
- **CLI Tools Hard Gate** — same inline copy as kasmos-planner.
- **Where You Fit** — managed: you implement ONE task (KASMOS_TASK env var). manual: execute the full plan sequentially by wave. env vars: KASMOS_TASK, KASMOS_WAVE, KASMOS_PEERS.
- **TDD Discipline** — RED-GREEN-REFACTOR, iron law (no production code without failing test), embedded directly. no separate skill dispatch.
- **Shared Worktree Safety** — when KASMOS_PEERS > 0: never `git add .`, never `git stash`/`git reset`/`git checkout --` on files you didn't touch, never run project-wide formatters. DO `git add` specific files, commit frequently with task number.
- **Debugging Discipline** — 4-phase root cause approach (investigate → pattern analysis → hypothesis testing → implementation), max 3 fix attempts then escalate.
- **Verification** — evidence before claims, run full tests, check exit codes. red flags: "should", "probably", "seems to".
- **Handling Reviewer Feedback** — evaluate technically (no performative agreement), fix one at a time, test each, push back if technically incorrect.
- **Signaling** — managed: `touch docs/plans/.signals/implement-finished-<planfile>`, stop. manual: execute waves sequentially, self-review between waves, handle branch finishing (verify → merge/PR/keep/discard → cleanup).

**Commit:** `feat: add kasmos-coder skill`

### Task 7: Write kasmos-reviewer skill

**Files:**
- Create: `.agents/skills/kasmos-reviewer/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/kasmos-reviewer/SKILL.md`

Write the reviewer skill (~200 lines). Consolidates `requesting-code-review` + `receiving-code-review` + review prompt template. See the design doc section "4. kasmos-reviewer" for the full specification.

Key sections:
- **CLI Tools Hard Gate** — same inline copy.
- **Where You Fit** — review the implementation branch. in managed mode, kasmos spawns you after coders finish. review only branch diff: `GIT_EXTERNAL_DIFF=difft git diff main..HEAD`.
- **Review Checklist** — spec compliance (matches plan goals? all tasks complete? scope creep?) + code quality (error handling, DRY, edge cases, test coverage, production readiness).
- **Self-Fix Protocol** — self-fix: typos, doc comments, obvious one-liners, import cleanup. kick to coder: debugging, logic changes, missing tests, architectural concerns.
- **All Tiers Blocking** — Critical, Important, Minor all must resolve. round tracking (Round 1, Round 2, etc.).
- **Verification** — run tests before approving, use `difft` for structural diffs, cite file:line in all findings.
- **Signal Format** — approved: `echo "Approved. <summary>" > docs/plans/.signals/review-approved-<planfile>`. changes needed: structured heredoc with round number, severity tiers, file:line refs. managed: write signal, stop. manual: additionally offer merge/PR/keep/discard.

**Commit:** `feat: add kasmos-reviewer skill`

### Task 8: Write kasmos-custodial skill

**Files:**
- Create: `.agents/skills/kasmos-custodial/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/kasmos-custodial/SKILL.md`

Write the custodial skill (~150 lines). New skill for the custodial agent role. See the design doc section "5. kasmos-custodial" for the full specification.

Key sections:
- **CLI Tools Hard Gate** — same inline copy.
- **Where You Fit** — ops/janitor, NOT feature work. you fix stuck states, clean up stale resources, trigger waves, triage plans.
- **Available CLI Commands** — `kas plan list`, `kas plan set-status`, `kas plan transition`, `kas plan implement`. note: use `kas` not `kq`.
- **Available Slash Commands** — `/kas.reset-plan`, `/kas.finish-branch`, `/kas.cleanup`, `/kas.implement`, `/kas.triage`.
- **Cleanup Protocol** — 3-pass: stale worktrees (plan done/cancelled but worktree exists), orphan branches (`plan/*` with no plan-state.json entry), ghost plan entries (plan-state.json entries with no .md file). always dry-run first.
- **Safety Rules** — `--force` required for status overrides, confirm before destructive ops, never modify plan file content, FSM transitions validate state.

**Commit:** `feat: add kasmos-custodial skill`

---

## Wave 3: Update All References Across Harnesses

> **Depends on Wave 2:** skills must exist before templates and prompt builders can reference them. this wave rewires all the pointers.

### Task 9: Update agent role templates (claude + opencode + codex)

**Files:**
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/custodial.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/custodial.md`
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md`
- Modify: `.opencode/agents/coder.md`
- Modify: `.opencode/agents/planner.md`
- Modify: `.opencode/agents/reviewer.md`
- Modify: `.opencode/agents/custodial.md`
- Modify: `.claude/agents/coder.md` (if exists)
- Modify: `.claude/agents/planner.md` (if exists)
- Modify: `.claude/agents/reviewer.md` (if exists)

Each agent template is simplified to load exactly 1 kasmos skill:

**coder.md** (both harnesses): replace the 3-line superpowers skill list (`test-driven-development`, `systematic-debugging`, `verification-before-completion`) with a single line: "load the `kasmos-coder` skill." keep the CLI Tools MANDATORY section (agents still need deep reference files). keep the Parallel Execution section.

**planner.md** (both harnesses): replace the superpowers skill list (`brainstorming`, `writing-plans`) with "load the `kasmos-planner` skill." keep Plan State, Branch Policy, and CLI Tools sections.

**reviewer.md** (both harnesses): replace the superpowers skill list (`requesting-code-review`, `receiving-code-review`) with "load the `kasmos-reviewer` skill." keep CLI Tools section. the Review Protocol section can be simplified since the skill now embeds the full protocol.

**custodial.md** (both harnesses): add "load the `kasmos-custodial` skill." fix `kq` → `kas` in all CLI command references. keep CLI Tools section.

**codex/AGENTS.md**: update all skill references from superpowers names to kasmos-* equivalents.

**Local kasmos repo agents** (`.opencode/agents/`, `.claude/agents/`): mirror the same changes as the scaffold templates.

**Commit:** `refactor: update agent templates to reference kasmos-native skills`

### Task 10: Update prompt builders, review template, and opencode commands

**Files:**
- Modify: `app/app_state.go` (lines 1243-1294: buildPlanPrompt, buildWaveAnnotationPrompt, buildImplementPrompt, buildSoloPrompt)
- Modify: `app/wave_prompt.go` (line 15: buildTaskPrompt cli-tools reference)
- Modify: `app/app_plan_actions_test.go` (line 31: test assertion for skill name)
- Modify: `internal/initcmd/scaffold/templates/shared/review-prompt.md`
- Modify: `.opencode/commands/kas.cleanup.md`
- Modify: `.opencode/commands/kas.finish-branch.md`

**Prompt builders:**
- `buildPlanPrompt()` (app_state.go:1246): change `"Use the \x60writing-plans\x60 superpowers skill"` to `"Use the \x60kasmos-planner\x60 skill"`
- `buildImplementPrompt()` (app_state.go:1275): change `"using the executing-plans superpowers skill"` to `"using the \x60kasmos-coder\x60 skill"`
- `buildTaskPrompt()` (wave_prompt.go:15): change `"Load the \x60cli-tools\x60 skill before starting"` to `"Use the \x60kasmos-coder\x60 skill"` (the coder skill embeds cli-tools inline)
- `buildWaveAnnotationPrompt()`: no superpowers references to change, but verify it doesn't reference any old skill names
- `buildSoloPrompt()`: no skill references currently, leave as-is

**Test updates:**
- `app_plan_actions_test.go:31`: change `assert.Contains(t, prompt, "writing-plans"...)` to `assert.Contains(t, prompt, "kasmos-planner"...)`

**Review prompt template:**
- `shared/review-prompt.md:11`: change `"Load the \x60requesting-code-review\x60 superpowers skill"` to `"Use the \x60kasmos-reviewer\x60 skill"`

**OpenCode commands:**
- `kas.cleanup.md`: change `kq plan` to `kas plan` (lines 26, 28, 37, 42, 45)
- `kas.finish-branch.md`: change `kq plan` to `kas plan` (line 22, 47)
- verify all other `kas.*.md` files already use `kas` not `kq`

**Commit:** `refactor: update prompt builders and review template to kasmos-native skills`

---

## Wave 4: Cleanup Old Skills and Verify

> **Depends on Wave 3:** all references must point to new skills before removing old ones. this wave removes the old scaffolded skills and does a final consistency check.

### Task 11: Remove old scaffolded skills and verify consistency

**Files:**
- Remove: `internal/initcmd/scaffold/templates/skills/writing-plans/`
- Remove: `internal/initcmd/scaffold/templates/skills/executing-plans/`
- Remove: `internal/initcmd/scaffold/templates/skills/subagent-driven-development/`
- Remove: `internal/initcmd/scaffold/templates/skills/requesting-code-review/`
- Remove: `internal/initcmd/scaffold/templates/skills/finishing-a-development-branch/`
- Remove: `.agents/skills/writing-plans/`
- Remove: `.agents/skills/executing-plans/`
- Remove: `.agents/skills/requesting-code-review/`
- Remove: `.agents/skills/subagent-driven-development/`
- Remove: `.agents/skills/finishing-a-development-branch/`

**Steps:**

1. Remove the 5 old skill directories from `internal/initcmd/scaffold/templates/skills/`. these are the scaffold templates that `kas init` copies into new projects.

2. Remove the 5 old skill directories from `.agents/skills/`. these are the canonical project-local copies.

3. Verify no remaining references to old skill names. search for these strings across the entire codebase:
   - `writing-plans` (should only appear in docs/plans/ historical files and the new kasmos-planner skill's internal text)
   - `executing-plans` (should only appear in historical plan files)
   - `subagent-driven-development` (should be gone entirely from active code)
   - `requesting-code-review` (should be gone entirely from active code)
   - `finishing-a-development-branch` (should be gone entirely from active code)
   - `superpowers` (should only appear in skills-lock.json, historical docs, and README.md tagline)

   Use `rg` (not grep) for the search. any remaining references in active code (not docs/plans/ history) must be updated.

4. Verify the scaffold pipeline still works by checking that `WriteProjectSkills` in `scaffold.go` will pick up the new `kasmos-*` directories from `templates/skills/` and skip the removed ones. the function uses `fs.WalkDir` on the embedded `templates` filesystem — removing the directories from the source tree is sufficient.

5. Run `go build ./...` to verify no compilation errors from the prompt builder changes.

6. Run `go test ./...` to verify all tests pass, including the updated `TestBuildPlanPrompt` assertion.

**Commit:** `refactor: remove old superpowers-derived scaffolded skills`
