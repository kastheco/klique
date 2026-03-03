# Plan Self-Review Implementation Plan

**Goal:** Add a mandatory self-review gate to the kasmos-planner skill so the planner agent validates its own plan against a structured checklist before signaling completion — catching missing wave headers, vague tasks, and structural issues before they reach implementation.

**Architecture:** The `kasmos-planner` SKILL.md gets a new `## plan review` section inserted between `## after writing the plan` and `## signaling`. The review is a checklist the planner must work through after writing the plan but before committing. Failures require inline fixes (re-edit the plan file) and re-running the checklist until all items pass. Only after a clean review does the planner proceed to commit + signal. No Go code changes — this is purely a skill document update.

**Tech Stack:** Markdown (SKILL.md), kasmos-planner skill, planner agent prompt (.opencode/agents/planner.md)

**Size:** Small (estimated ~45 min, 2 tasks, 1 wave)

---

## Wave 1: Planner Self-Review Gate

### Task 1: Add plan review section to kasmos-planner skill

**Files:**
- Modify: `.opencode/skills/kasmos-planner/SKILL.md`

**Step 1: write the failing test**

The contract test at `contracts/planner_prompt_contract_test.go` does not cover the skill file directly (it covers the agent prompt). Instead, verify manually that the current skill has no review section:

```bash
rg '## plan review' .opencode/skills/kasmos-planner/SKILL.md
```

expected: no matches

**Step 2: write the review section**

Insert a new `## plan review` section in `.opencode/skills/kasmos-planner/SKILL.md` between the `## after writing the plan` section's intro and the `### 1. create todos` step. The new flow becomes:

1. Write the plan file (existing)
2. **Run the self-review checklist** (new)
3. Fix any failures inline (new)
4. Create todos (existing, renumbered)
5. Commit the plan (existing, renumbered)

The review section content:

```markdown
## plan review

after writing the plan file, review it against this checklist before committing.
this is mandatory — do not skip it, even for trivial plans. fix every failure
inline (edit the plan file directly) before proceeding to commit + signal.

### structural checks

- [ ] **wave headers present** — at least one `## Wave 1` header exists. plans without
  wave headers cannot be executed by kasmos.
- [ ] **task headers present** — every wave contains at least one `### Task N: Title` entry.
- [ ] **task numbering sequential** — task numbers are sequential across the entire plan
  (Task 1, Task 2, ..., not restarting per wave).
- [ ] **required header fields** — plan starts with `**Goal:**`, `**Architecture:**`,
  `**Tech Stack:**`, and `**Size:**` fields. all four must be present and non-empty.
- [ ] **wave dependencies justified** — if wave N > 1 exists, it has a
  `> **depends on wave N-1:**` line explaining the dependency. if all tasks are
  independent, they should be in a single wave.

### task quality checks

- [ ] **files listed** — every task has a `**Files:**` block listing create/modify/test paths.
  paths must be exact (no placeholders like `path/to/...`).
- [ ] **TDD steps present** — every task has Step 1 (write failing test), Step 2 (run test,
  expect fail), Step 3 (implement), Step 4 (run test, expect pass), Step 5 (commit).
  trivial tasks (config-only, no testable logic) may omit Steps 1-2 but must note why.
- [ ] **test commands runnable** — `go test` commands reference real package paths that will
  exist after the task's files are created. no `./path/to/package/...` placeholders.
- [ ] **commit messages present** — every task ends with a concrete `git commit -m "..."` step.
- [ ] **no micro-tasks** — no task is < 10 minutes of work. if one is, merge it into an
  adjacent task.
- [ ] **no mega-tasks** — no task is > 45 minutes. if one is, split it.

### coherence checks

- [ ] **goal alignment** — re-read the `**Goal:**` field. does every task contribute to it?
  flag any task that doesn't.
- [ ] **no scope creep** — no task introduces work beyond what the goal describes. if you
  find yourself adding "while we're at it" tasks, remove them.
- [ ] **dependency ordering** — tasks in wave N do not depend on outputs from tasks in the
  same wave (those should be in separate waves). tasks within a wave must be independently
  implementable.
- [ ] **file conflict check** — no two tasks in the same wave modify the same file. if they
  do, either merge the tasks or move one to a later wave.

### review outcome

if all checks pass: proceed to commit + signal.

if any check fails: fix the plan file inline, then re-run the failed checks. do not
proceed until all checks pass. common fixes:
- missing wave headers → wrap all tasks under `## Wave 1`
- missing TDD steps → add the 5-step structure to each task
- vague file paths → look up actual paths with `fd` or `rg`
- micro-task → merge into adjacent task, renumber
- file conflicts in same wave → reorder into separate waves
```

The section is inserted so the full "after writing the plan" flow reads:

```
## after writing the plan
→ ## plan review          ← NEW
→ ### 1. create todos     (was ### 1)
→ ### 2. commit the plan  (was ### 2)
```

**Step 3: verify the edit**

```bash
rg '## plan review' .opencode/skills/kasmos-planner/SKILL.md
rg 'wave headers present' .opencode/skills/kasmos-planner/SKILL.md
rg 'review outcome' .opencode/skills/kasmos-planner/SKILL.md
```

expected: all three match

**Step 4: commit**

```bash
git add .opencode/skills/kasmos-planner/SKILL.md
git commit -m "feat: add mandatory plan self-review checklist to kasmos-planner skill"
```

### Task 2: Update planner agent prompt to reference self-review

**Files:**
- Modify: `.opencode/agents/planner.md`
- Modify: `contracts/planner_prompt_contract_test.go`

**Step 1: write the failing test**

Add a contract assertion that the planner prompt mentions plan review:

```go
// In TestPlannerPromptBranchPolicy (or a new test):
required := []string{
    // ... existing assertions ...
    "plan review",  // new: planner must reference the review step
}
```

```bash
go test ./contracts/... -run TestPlannerPromptBranchPolicy -v
```

expected: FAIL — "planner prompt missing required policy text: \"plan review\""

**Step 2: update the planner agent prompt**

Add a brief reference to the self-review in `.opencode/agents/planner.md`, after the "## Workflow" section:

```markdown
## Plan Review (MANDATORY)

After writing a plan, you MUST run the plan review checklist from the `kasmos-planner`
skill before committing or signaling. Do not skip this step. Fix all failures inline
before proceeding.
```

**Step 3: run test to verify it passes**

```bash
go test ./contracts/... -run TestPlannerPromptBranchPolicy -v
```

expected: PASS

**Step 4: commit**

```bash
git add .opencode/agents/planner.md contracts/planner_prompt_contract_test.go
git commit -m "feat: add plan review reference to planner agent prompt + contract test"
```
