# Create Fix Agent and Skill Implementation Plan

**Goal:** Replace the custodian agent type with a new "fixer" agent that subsumes all custodian ops work (cleanup, triage, state resets) while adding first-class debugging, investigation, loose-end resolution, and targeted verification capabilities.

**Architecture:** Rename `custodian` → `fixer` across the entire codebase: Go constants, scaffold templates, embedded skills, wizard defaults, opencode.jsonc template, config phase mapping, and tests. Write a new `kasmos-fixer` skill that merges the existing custodian operational playbook with a structured debugging/investigation methodology (root-cause analysis, evidence gathering, targeted verification, loose-end triage). Remove all `kasmos-custodian` artifacts. The broken `kasmos-custodial` symlink is also cleaned up.

**Tech Stack:** Go (bubbletea TUI), embedded templates (`embed.FS`), SKILL.md (markdown), opencode.jsonc (JSONC), TOML config

**Size:** Medium (estimated ~3 hours, 5 tasks, 2 waves)

---

## Wave 1: Rename custodian → fixer across Go code and templates

> All tasks in this wave are independent — they touch different file sets with no cross-dependencies.

### Task 1: Rename Go constants, app logic, and tests

**Files:**
- Modify: `session/instance.go`
- Modify: `app/app_state.go`
- Modify: `app/app_test.go`
- Modify: `app/app_input.go` (if references exist)
- Modify: `internal/check/project.go`
- Modify: `internal/initcmd/wizard/roles.go`
- Modify: `internal/initcmd/wizard/roles_test.go`
- Modify: `internal/initcmd/wizard/wizard.go`
- Modify: `internal/initcmd/wizard/wizard_test.go`
- Modify: `internal/initcmd/wizard/model.go`
- Modify: `internal/initcmd/wizard/model_agents_test.go`
- Modify: `internal/initcmd/scaffold/scaffold.go`
- Modify: `internal/initcmd/scaffold/scaffold_test.go`

**Step 1: write the failing test**

Update existing test assertions that reference `AgentTypeCustodian` or `"custodian"` to expect `AgentTypeFixer` / `"fixer"`. Key locations:
- `app/app_test.go:134` — change `session.AgentTypeCustodian` → `session.AgentTypeFixer`
- `internal/initcmd/wizard/roles_test.go` — update role description expectations
- `internal/initcmd/wizard/model_agents_test.go` — update agent defaults
- `internal/initcmd/scaffold/scaffold_test.go` — update template expectations

```bash
go test ./session/... ./app/... ./internal/initcmd/... -v -count=1 2>&1 | head -50
```

expected: FAIL — `AgentTypeFixer` undefined, `"fixer"` not found in maps

**Step 2: run test to verify it fails**

```bash
go test ./session/... ./app/... ./internal/initcmd/... -count=1 2>&1 | tail -20
```

**Step 3: write minimal implementation**

Rename across Go source files using `ast-grep` for the constant and `sd` for string literals:

1. In `session/instance.go`: rename `AgentTypeCustodian = "custodian"` → `AgentTypeFixer = "fixer"`
2. In `app/app_state.go`: update `case session.AgentTypeCustodian:` → `case session.AgentTypeFixer:` and the `ResolveProfile("custodian", ...)` → `ResolveProfile("fixer", ...)`; update `spawnAdHocAgent` to use `AgentTypeFixer`
3. In `internal/initcmd/wizard/roles.go`: rename `"custodian"` entries to `"fixer"` in both maps, update description text
4. In `internal/initcmd/wizard/wizard.go`: rename `"custodian"` in `DefaultAgentRoles()`, `RoleDefaults()`, and `PhaseMapping`
5. In `internal/initcmd/wizard/model.go`: rename `"custodian"` in `PhaseMapping` and the `renderOpenCodeConfig` role loop
6. In `internal/initcmd/scaffold/scaffold.go`: rename `"custodian"` in the `renderOpenCodeConfig` role iteration
7. In `internal/check/project.go`: rename `"kasmos-custodian"` → `"kasmos-fixer"` in `EmbeddedSkillNames`

**Step 4: run test to verify it passes**

```bash
go test ./session/... ./app/... ./internal/initcmd/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance.go app/app_state.go app/app_test.go app/app_input.go \
  internal/check/project.go internal/initcmd/wizard/roles.go \
  internal/initcmd/wizard/roles_test.go internal/initcmd/wizard/wizard.go \
  internal/initcmd/wizard/wizard_test.go internal/initcmd/wizard/model.go \
  internal/initcmd/wizard/model_agents_test.go \
  internal/initcmd/scaffold/scaffold.go internal/initcmd/scaffold/scaffold_test.go
git commit -m "refactor: rename custodian agent type to fixer across Go code"
```

### Task 2: Rename scaffold templates (agent prompts + opencode.jsonc)

**Files:**
- Rename: `internal/initcmd/scaffold/templates/opencode/agents/custodian.md` → `fixer.md`
- Rename: `internal/initcmd/scaffold/templates/claude/agents/custodian.md` → `fixer.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/opencode.jsonc`
- Modify: content of both renamed `.md` files (update role references)

**Step 1: write the failing test**

The scaffold tests should already fail from Task 1's constant rename. Verify the template files exist at the new paths by adding or updating a test that checks `templates.ReadFile("templates/opencode/agents/fixer.md")` succeeds.

```bash
go test ./internal/initcmd/scaffold/... -v -count=1 -run TestScaffold
```

expected: FAIL — template file `fixer.md` not found

**Step 2: run test to verify it fails**

```bash
go test ./internal/initcmd/scaffold/... -count=1 2>&1 | tail -10
```

**Step 3: write minimal implementation**

1. `mv internal/initcmd/scaffold/templates/opencode/agents/custodian.md internal/initcmd/scaffold/templates/opencode/agents/fixer.md`
2. `mv internal/initcmd/scaffold/templates/claude/agents/custodian.md internal/initcmd/scaffold/templates/claude/agents/fixer.md`
3. In both `fixer.md` files: replace "custodian" with "fixer" in role description, skill reference (`kasmos-fixer`), and behavioral text. Update the role identity from "ops/janitor" to "debugger, investigator, and ops fixer".
4. In `opencode.jsonc`: rename the `"custodian"` agent block key to `"fixer"`, rename `{{CUSTODIAN_MODEL}}` → `{{FIXER_MODEL}}`, `{{CUSTODIAN_EFFORT_LINE}}` → `{{FIXER_EFFORT_LINE}}`, `{{CUSTODIAN_TEMP}}` → `{{FIXER_TEMP}}`
5. In `scaffold.go`: update the `renderOpenCodeConfig` role loop from `"custodian"` to `"fixer"` and the placeholder prefix from `CUSTODIAN_` to `FIXER_`

**Step 4: run test to verify it passes**

```bash
go test ./internal/initcmd/scaffold/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add internal/initcmd/scaffold/templates/
git commit -m "refactor: rename custodian scaffold templates to fixer"
```

### Task 3: Rename embedded skill from kasmos-custodian to kasmos-fixer

**Files:**
- Rename: `internal/initcmd/scaffold/templates/skills/kasmos-custodian/` → `kasmos-fixer/`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md` (placeholder — full content written in Wave 2)
- Rename: `.agents/skills/kasmos-custodian/` → `kasmos-fixer/`
- Modify: `.agents/skills/kasmos-fixer/SKILL.md` (placeholder — full content written in Wave 2)
- Remove: `.opencode/skills/kasmos-custodian` symlink
- Remove: `.opencode/skills/kasmos-custodial` broken symlink
- Create: `.opencode/skills/kasmos-fixer` symlink → `../../.agents/skills/kasmos-fixer`

**Step 1: write the failing test**

The `internal/check/project.go` `EmbeddedSkillNames` was already updated in Task 1 to expect `kasmos-fixer`. Verify the embedded template directory exists:

```bash
go test ./internal/check/... -v -count=1
```

**Step 2: run test to verify it fails**

```bash
go test ./internal/check/... -count=1 2>&1 | tail -10
```

**Step 3: write minimal implementation**

1. `mv internal/initcmd/scaffold/templates/skills/kasmos-custodian internal/initcmd/scaffold/templates/skills/kasmos-fixer`
2. Update the SKILL.md frontmatter: `name: kasmos-fixer`, update description line (placeholder content for now — Wave 2 writes the full skill)
3. `mv .agents/skills/kasmos-custodian .agents/skills/kasmos-fixer`
4. `rm .opencode/skills/kasmos-custodian` (symlink)
5. `rm .opencode/skills/kasmos-custodial` (broken symlink)
6. `ln -s ../../.agents/skills/kasmos-fixer .opencode/skills/kasmos-fixer`
7. Update `.gitignore` if it references `kasmos-custodian` or `kasmos-custodial`

**Step 4: run test to verify it passes**

```bash
go build ./... && go test ./... -count=1
```

expected: PASS, full build succeeds

**Step 5: commit**

```bash
git add internal/initcmd/scaffold/templates/skills/ .agents/skills/ \
  .opencode/skills/ internal/check/project.go
git commit -m "refactor: rename kasmos-custodian skill to kasmos-fixer"
```

---

## Wave 2: Write the kasmos-fixer skill with debugging methodology

> **depends on wave 1:** the skill files must exist at the `kasmos-fixer` path before we can write final content into them.

### Task 4: Write the kasmos-fixer SKILL.md with full debugging/investigation methodology

**Files:**
- Modify: `.agents/skills/kasmos-fixer/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md`
- Modify: `.opencode/skills/kasmos-fixer/SKILL.md` (via symlink — same file as `.agents/`)

**Step 1: write the failing test**

No automated test for skill content — this is a documentation/prompt artifact. Verify the file renders correctly by checking it contains required sections:

```bash
rg '## Where You Fit' .agents/skills/kasmos-fixer/SKILL.md
rg '## Debugging Protocol' .agents/skills/kasmos-fixer/SKILL.md
rg '## Cleanup Protocol' .agents/skills/kasmos-fixer/SKILL.md
```

expected: no matches yet (placeholder content from Wave 1)

**Step 2: write the skill content**

The `kasmos-fixer` skill must contain these sections, merging custodian ops with debugging:

1. **Frontmatter** — `name: kasmos-fixer`, description covering both debugging and ops
2. **Identity** — "You are the fixer agent — debugger, investigator, and operational troubleshooter"
3. **CLI Tools Hard Gate** — same banned-tools table as other kasmos skills
4. **Where You Fit** — expanded scope table:
   - Custodian ops: fix stuck plan states, clean worktrees/branches, trigger waves, triage, merge/PR, version bumps
   - Debugging: investigate test failures, trace root causes, reproduce bugs, verify fixes
   - Investigation: audit implementation completeness, check for loose ends, verify edge cases
   - Targeted verification: run specific test suites, confirm fix correctness, validate integration
5. **Debugging Protocol** — structured 4-phase methodology:
   - Phase 1: Evidence Gathering (read errors, reproduce, check recent changes, trace data flow)
   - Phase 2: Pattern Analysis (find working examples, compare, list differences)
   - Phase 3: Hypothesis Testing (form hypothesis, test with minimal change, one variable at a time)
   - Phase 4: Fix Implementation (write failing test, implement fix, verify no regressions)
   - Escalation rule: after 3 failed fixes, stop — it's architecture, not a bug
6. **Investigation Protocol** — for loose-end triage:
   - Scan for TODOs, FIXMEs, incomplete implementations
   - Cross-reference plan tasks against actual implementation
   - Check test coverage gaps
   - Verify error handling completeness
7. **Targeted Verification** — evidence-first approach:
   - Identify verification command
   - Run it, read full output
   - Confirm claim matches evidence
   - Never claim success without proof
8. **Cleanup Protocol** — carried over from custodian (3-pass: worktrees, branches, ghost entries)
9. **Available CLI Commands** — `kas plan` commands (carried over)
10. **Available Slash Commands** — carried over from custodian
11. **Release Version Bump** — carried over from custodian
12. **Safety Rules** — carried over + new debugging safety rules
13. **Mode Signaling** — updated sentinel names: `fixer-cleanup-*`, `fixer-triage-*`, `fixer-done-*`

Write identical content to both:
- `.agents/skills/kasmos-fixer/SKILL.md` (canonical)
- `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md` (embedded template)

**Step 3: verify the skill content**

```bash
rg '## Where You Fit' .agents/skills/kasmos-fixer/SKILL.md
rg '## Debugging Protocol' .agents/skills/kasmos-fixer/SKILL.md
rg '## Investigation Protocol' .agents/skills/kasmos-fixer/SKILL.md
rg '## Targeted Verification' .agents/skills/kasmos-fixer/SKILL.md
rg '## Cleanup Protocol' .agents/skills/kasmos-fixer/SKILL.md
```

expected: all sections present

**Step 4: commit**

```bash
git add .agents/skills/kasmos-fixer/SKILL.md \
  internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md
git commit -m "feat: write kasmos-fixer skill with debugging and investigation methodology"
```

### Task 5: Update the project-level kasmos-fixer skill (`.opencode/skills/`) and agent prompts

**Files:**
- Modify: `.opencode/skills/kasmos-fixer/SKILL.md` (this is the live skill loaded by the current project's agents — it's a symlink to `.agents/skills/kasmos-fixer/SKILL.md`, so it was already updated in Task 4)
- Modify: `.opencode/agents/fixer.md` (if it exists — the live opencode agent prompt for this project)
- Modify: `.claude/agents/fixer.md` (if it exists — the live claude agent prompt for this project)
- Verify: symlink integrity for `.opencode/skills/kasmos-fixer`

**Step 1: verify symlink and content**

```bash
ls -la .opencode/skills/kasmos-fixer
rg 'kasmos-fixer' .opencode/skills/kasmos-fixer/SKILL.md
```

expected: symlink valid, content matches `.agents/skills/kasmos-fixer/SKILL.md`

**Step 2: update live agent prompts**

If `.opencode/agents/custodian.md` exists, rename to `fixer.md` and update content to reference `kasmos-fixer` skill. Same for `.claude/agents/custodian.md`.

```bash
fd 'custodian' .opencode/agents/ .claude/agents/ 2>/dev/null
```

Rename and update content: change "custodian" → "fixer", update skill load instruction to `kasmos-fixer`.

**Step 3: full build and test verification**

```bash
go build ./... && go test ./... -count=1
```

expected: PASS — full build and all tests green

**Step 4: commit**

```bash
git add .opencode/ .claude/ .agents/
git commit -m "feat: update live agent prompts and verify kasmos-fixer skill integration"
```
