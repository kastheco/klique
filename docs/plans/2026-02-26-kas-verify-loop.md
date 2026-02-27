# Verify Loop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** update all reviewer/fixer instructions so the review loop continues until every issue at every severity tier is addressed — no "note for later" escape hatch.

**Architecture:** purely instructional changes across markdown files. No Go code changes — the FSM loop (`reviewing` → `review-changes` → `implementing` → `implement-finished` → `reviewing`) already works. We're updating the text that flows through it.

**Tech Stack:** markdown prompt files, skill docs

**Size:** Small (estimated ~1 hour, 3 tasks, no waves)

---

## Wave 1

### Task 1: Core Review Instructions

Update the three files that form the TUI-orchestrated review protocol: the review prompt template, the code-reviewer template, and the requesting-code-review skill.

**Files:**
- Modify: `internal/initcmd/scaffold/templates/shared/review-prompt.md`
- Modify: `.claude/skills/requesting-code-review/code-reviewer.md`
- Modify: `.claude/skills/requesting-code-review/SKILL.md`

**Step 1: Rewrite review-prompt.md**

Replace the entire file with:

```markdown
Review the implementation of plan: {{PLAN_NAME}}

Plan file: {{PLAN_FILE}}

Read the plan file to understand the goals, architecture, and tasks that were implemented.

IMPORTANT: Only review changes from this branch. Use `git diff main..HEAD` to see exactly
what was changed by the implementation — do NOT review code that was inherited from main.
Files may contain code from main that is outside the scope of this plan.

Load the `requesting-code-review` superpowers skill for structured review methodology.

Focus areas:
- Does the implementation match the plan's stated goals and architecture?
- Were any tasks implemented incorrectly or incompletely?
- Code quality, error handling, test coverage
- Regressions or unintended side effects introduced by this branch's changes

## All severity tiers are blocking

Every issue you find — Critical, Important, or Minor — must be resolved before approval.
There is no "note for later" or "nice to have" category. If you flag it, it gets fixed.

## Self-fix protocol

For trivial issues, fix them yourself instead of kicking back to the coder:

**Self-fix (commit directly):**
- Typos in code, comments, or strings
- Missing or wrong doc comments
- Obvious one-liner fixes (wrong constant name, missing return)
- Import cleanup
- Trivial formatting the linter missed

**Kick to coder (write review-changes signal):**
- Anything requiring debugging or investigation
- Logic changes, even small ones
- Missing test coverage
- Architectural concerns
- Anything where the right fix isn't immediately obvious

If only self-fixable issues remain, fix them all and write review-approved.
If any coder-required issues exist, self-fix what you can first, then write review-changes.

## When you are done

You MUST write a signal file to indicate your verdict. Without this, the orchestrator
cannot progress the plan lifecycle.

If approved (zero issues remaining after self-fixes):
```
echo "Approved. <brief summary>" > docs/plans/.signals/review-approved-{{PLAN_FILENAME}}
```

If changes are required (issues that need a coder):
```
cat > docs/plans/.signals/review-changes-{{PLAN_FILENAME}} << 'SIGNAL'
## review round N

### critical
- [file:line] description — why it matters

### important
- [file:line] description — why it matters

### minor
- [file:line] description — why it matters

### self-fixed (no action needed)
- [file:line] what was fixed
SIGNAL
```

Use the structured format above. Include the round number (1 for first review, 2 for re-review
after fixes, etc.). Omit empty tiers. Every item must have a file:line reference.

Write exactly one of these signal files before you finish.
```

**Step 2: Update code-reviewer.md**

In `.claude/skills/requesting-code-review/code-reviewer.md`, make these changes:

a) Replace the `#### Minor (Nice to Have)` header (line 76-77) with:

```markdown
#### Minor (Must Fix)
[Code style, optimization opportunities, documentation improvements]
```

b) Replace the Assessment section (lines 88-92) with:

```markdown
### Assessment

**Clean?** [Yes/No]

If Yes: zero issues found — approve.
If No: list all issues above. ALL tiers are blocking — every issue must be resolved.
```

c) Replace the example Assessment section (lines 141-145) with:

```markdown
### Assessment

**Clean: No**

Two Important issues (help text, date validation) and one Minor issue (progress indicators) must be resolved before approval.
```

d) In the Critical Rules DO section (lines 96-101), add after "Give clear verdict":

```markdown
- Treat ALL tiers as blocking — no "note for later"
```

e) In the Critical Rules DON'T section (lines 103-108), add after "Avoid giving a clear verdict":

```markdown
- Approve with outstanding issues at any tier
- Use "Ready to merge: With fixes" — either clean or not
```

**Step 3: Update requesting-code-review SKILL.md**

In `.claude/skills/requesting-code-review/SKILL.md`, make these changes:

a) Replace the "Act on feedback" section (lines 43-47) with:

```markdown
**3. Act on feedback:**
- ALL tiers are blocking — Critical, Important, and Minor
- Fix every issue the reviewer flagged
- Push back if reviewer is wrong (with reasoning)
- Loop: fix → re-review → repeat until clean
```

b) Replace the Red Flags section (lines 92-98) with:

```markdown
## Red Flags

**Never:**
- Skip review because "it's simple"
- Ignore issues at any severity tier
- Proceed with unfixed issues, no matter how minor
- Argue with valid technical feedback
- Approve with outstanding issues ("note for later")
```

c) Update the example (lines 66-74) — replace:

```
  Assessment: Ready to proceed

You: [Fix progress indicators]
[Continue to Task 3]
```

with:

```
  Assessment: Not clean — 2 issues remain

You: [Fix progress indicators AND magic number]
[Re-dispatch reviewer]
Reviewer: Clean — zero issues
[Continue to Task 3]
```

**Step 4: Verify consistency**

Run: `rg "note.*later\|nice to have\|ready.*with fixes" .claude/skills/requesting-code-review/`

Expected: zero matches.

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/templates/shared/review-prompt.md .claude/skills/requesting-code-review/
git commit -m "feat: all-tier blocking review loop — core review instructions"
```

---

## Wave 2

### Task 2: Subagent Review Skills

Update the subagent-driven-development skill and its reviewer prompt templates to enforce all-blocking tiers and the self-fix protocol. Then sync the two copy locations.

**Files:**
- Modify: `.claude/skills/subagent-driven-development/SKILL.md`
- Modify: `.claude/skills/subagent-driven-development/code-quality-reviewer-prompt.md`
- Modify: `.claude/skills/subagent-driven-development/spec-reviewer-prompt.md`
- Sync: `.agents/skills/subagent-driven-development/SKILL.md` (copy from `.claude/`)
- Sync: `internal/initcmd/scaffold/templates/skills/subagent-driven-development/SKILL.md` (copy from `.claude/`)

**Step 1: Update subagent-driven-development SKILL.md**

a) In the "Red Flags" section, replace lines 270-274 ("If reviewer finds issues"):

```markdown
**If reviewer finds issues:**
- ALL tiers are blocking — Critical, Important, and Minor
- Dispatch fix subagent with specific instructions for every flagged issue
- Reviewer reviews again after fixes
- Repeat until reviewer reports zero issues
- Don't skip the re-review
- Don't skip Minor issues — they are not "nice to have"
```

b) In the "Red Flags" `**Never:**` list, after "Proceed with unfixed issues from review" (line 255), add:

```markdown
- Treat Minor issues as non-blocking — all tiers must pass
```

**Step 2: Update code-quality-reviewer-prompt.md**

Replace the entire file with:

````markdown
# Code Quality Reviewer Prompt Template

Use this template when dispatching a code quality reviewer subagent.

**Purpose:** Verify implementation is well-built (clean, tested, maintainable)

**Only dispatch after spec compliance review passes.**

```
Task tool (superpowers:code-reviewer):
  Use template at requesting-code-review/code-reviewer.md

  WHAT_WAS_IMPLEMENTED: [from implementer's report]
  PLAN_OR_REQUIREMENTS: Task N from [plan-file]
  BASE_SHA: [commit before task]
  HEAD_SHA: [current commit]
  DESCRIPTION: [task summary]
```

**All tiers are blocking.** The reviewer must report either:
- ✅ Clean — zero issues at any tier
- ❌ Issues found — list every issue with file:line references

There is no "Ready to merge: With fixes" or "Note Minor for later" outcome.

**Self-fix protocol:** For trivial issues (typos, doc comments, obvious one-liners),
the reviewer fixes them directly and commits. Non-trivial issues go back to a fix subagent.

**Code reviewer returns:** Strengths, Issues (Critical/Important/Minor — all blocking), Assessment (Clean or Not Clean)
````

**Step 3: Update spec-reviewer-prompt.md**

In `.claude/skills/subagent-driven-development/spec-reviewer-prompt.md`, replace the Report section at the end (lines 58-61):

```markdown
    Report using the structured format:

    ## review round N

    If clean (zero issues found after inspection):
    - ✅ Spec compliant — zero issues

    If issues found:
    ### critical
    - [file:line] description — why it matters

    ### important
    - [file:line] description — why it matters

    ### minor
    - [file:line] description — why it matters

    ALL tiers are blocking. Every issue must be resolved before approval.
    There is no "note for later" category.
```

**Step 4: Sync copies**

```bash
cp .claude/skills/subagent-driven-development/SKILL.md .agents/skills/subagent-driven-development/SKILL.md
cp .claude/skills/subagent-driven-development/SKILL.md internal/initcmd/scaffold/templates/skills/subagent-driven-development/SKILL.md
```

**Step 5: Verify consistency**

Run: `rg "note.*later\|nice to have\|ready.*with fixes" .claude/skills/subagent-driven-development/ .agents/skills/subagent-driven-development/ internal/initcmd/scaffold/templates/skills/subagent-driven-development/`

Expected: zero matches.

**Step 6: Commit**

```bash
git add .claude/skills/subagent-driven-development/ .agents/skills/subagent-driven-development/ internal/initcmd/scaffold/templates/skills/subagent-driven-development/
git commit -m "feat: all-tier blocking review loop — subagent review skills"
```

---

### Task 3: Reviewer Agent Configs

Add self-fix protocol guidance to all four reviewer agent config files.

**Files:**
- Modify: `.claude/agents/reviewer.md`
- Modify: `.opencode/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`

**Step 1: Update .claude/agents/reviewer.md**

After the existing Workflow section (after line 17 "Be specific about issues — cite file paths and line numbers."), add:

```markdown

## Review Protocol

All severity tiers are blocking — Critical, Important, and Minor. The review loop continues
until you produce a clean pass with zero issues.

**Self-fix trivial issues** (typos, doc comments, obvious one-liners) directly — commit and
continue reviewing. Only kick back to the coder for issues requiring debugging, logic changes,
missing tests, or anything where the right fix isn't immediately obvious.
```

**Step 2: Update .opencode/agents/reviewer.md**

Apply the same addition after line 16 (same position as .claude version — after "Be specific about issues").

**Step 3: Update template/claude/agents/reviewer.md**

Apply the same addition after line 16 (after "Be specific about issues").

**Step 4: Update template/opencode/agents/reviewer.md**

Apply the same addition after line 16 (after "Be specific about issues").

**Step 5: Verify all four files have the protocol**

Run: `rg "all severity tiers are blocking" .claude/agents/reviewer.md .opencode/agents/reviewer.md internal/initcmd/scaffold/templates/claude/agents/reviewer.md internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`

Expected: 4 matches (one per file).

**Step 6: Commit**

```bash
git add .claude/agents/reviewer.md .opencode/agents/reviewer.md internal/initcmd/scaffold/templates/claude/agents/reviewer.md internal/initcmd/scaffold/templates/opencode/agents/reviewer.md
git commit -m "feat: all-tier blocking review loop — reviewer agent configs"
```
