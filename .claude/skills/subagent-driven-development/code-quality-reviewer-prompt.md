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
