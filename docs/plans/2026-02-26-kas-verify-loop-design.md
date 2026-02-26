# Verify Loop Design

**Goal:** update all reviewer instructions so the review ↔ fix loop continues until every issue at every severity tier has been addressed. No "note for later" escape hatch.

## Core Rule

All three severity tiers (Critical, Important, Minor) are blocking. The review loop runs until the reviewer produces a clean pass with zero findings. The only valid approval is zero issues remaining.

Reviewer verdict:
- **Clean** (zero findings) → write `review-approved` signal
- **Issues found at any tier** → self-fix trivials, write `review-changes` signal for the rest

Applies to both TUI-orchestrated (kasmos FSM) and in-session subagent flows.

## Reviewer Self-Fix Protocol

When the reviewer finds issues, it triages each one into self-fix or kick-to-coder:

**Self-fix (reviewer handles in-place, commits):**
- Typos in code, comments, or strings
- Missing/wrong doc comments
- Obvious one-liner fixes (wrong constant name, missing return)
- Import cleanup
- Trivial formatting the linter missed

**Kick to coder (write `review-changes`):**
- Anything requiring debugging or investigation
- Logic changes, even small ones
- Missing test coverage
- Architectural concerns
- Anything where the right fix isn't immediately obvious

If only self-fixable issues remain, the reviewer fixes them all and writes `review-approved`. If any coder-required issues exist, the reviewer self-fixes what it can first (reducing coder workload), then writes `review-changes` with remaining items.

## Structured Feedback Format

The `review-changes` signal body uses a structured format:

```
## review round N

### critical
- [file:line] description — why it matters

### important
- [file:line] description — why it matters

### minor
- [file:line] description — why it matters

### self-fixed (no action needed)
- [file:line] what was fixed
```

- Round number tracks iteration count
- `self-fixed` section is informational so coder knows what changed
- Every item has file:line reference
- Empty tiers omitted

No Go code changes — structured text flows through the existing `spawnCoderWithFeedback` pipe.

## Files to Change

**Primary files (6):**
1. `internal/initcmd/scaffold/templates/shared/review-prompt.md` — rewrite with tier enforcement, self-fix protocol, structured feedback, loop-until-clean
2. `.claude/skills/requesting-code-review/SKILL.md` — all tiers blocking
3. `.claude/skills/requesting-code-review/code-reviewer.md` — only "clean" or "issues found"
4. `.claude/skills/subagent-driven-development/SKILL.md` — all tiers blocking, self-fix protocol
5. `.claude/skills/subagent-driven-development/code-quality-reviewer-prompt.md` — self-fix + structured output
6. `.claude/skills/subagent-driven-development/spec-reviewer-prompt.md` — all findings blocking, structured output

**Synced copies (2):**
7. `.agents/skills/subagent-driven-development/SKILL.md`
8. `internal/initcmd/scaffold/templates/skills/subagent-driven-development/SKILL.md`

**Reviewer agent configs (4):**
9. `.claude/agents/reviewer.md`
10. `.opencode/agents/reviewer.md`
11. `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`
12. `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`
