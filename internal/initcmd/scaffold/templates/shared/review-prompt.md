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
