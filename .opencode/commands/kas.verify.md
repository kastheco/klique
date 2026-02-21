---
description: Tiered verification with built-in specialist checks (self-contained)
agent: reviewer
---

# /kas.verify - Tiered Verification (Self-Contained)

Run a three-tier verification workflow with early exits:
1) static analysis, 2) reality assessment, 3) optional simplification suggestions.

This command is self-contained and does not depend on external plugin commands or agent files.

## User Input

```text
$ARGUMENTS
```

Treat arguments as optional scope hints (specific files, directories, or review focus).

## Phase 0 - Build review context

Run:

```bash
git status
git diff --stat
git diff
git diff --cached
```

If both `git diff` and `git diff --cached` are empty: report `Nothing to verify` and stop.

## Phase 1 - Change profile and Tier 1 static analysis

### 1a) Derive change profile

From changed files + diff content, set these booleans:

- `has_code_changes`
- `has_error_handling_changes` (try/catch, recover, error wrapping/propagation, fallback paths)
- `has_comments_or_docs_changes` (comments, docstrings, markdown/docs edits)
- `has_type_or_schema_changes` (types, interfaces, structs, schema definitions, validation contracts)
- `has_test_changes`
- `docs_only` (all changes are docs)
- `config_only` (all changes are config)

### 1b) Select Tier 1 specialist checks

Run only relevant checks:

- `core-code-reviewer` (run for any code change; also run for config-only changes)
- `silent-failure-hunter` (run when `has_error_handling_changes`)
- `comment-analyzer` (run when `has_comments_or_docs_changes` and code is touched)
- `type-design-analyzer` (run when `has_type_or_schema_changes`)
- `pr-test-analyzer` (run when `has_test_changes` OR code changed without corresponding tests)

If `docs_only`, skip Tier 1 and continue to Tier 2.

If your runtime supports subagents, run selected Tier 1 checks in parallel. If not, run sequentially.

### 1c) Tier 1 check rubrics (embedded)

#### core-code-reviewer

Review for correctness, security, readability, performance, maintainability, and robust error handling.

Focus:
- logic bugs and edge cases
- unsafe behavior or vulnerabilities
- unnecessary complexity or unclear naming
- regressions and missing safeguards

#### silent-failure-hunter

Audit error handling with zero tolerance for hidden failures.

Flag:
- empty catch/ignore blocks
- swallowed errors without propagation
- fallback behavior that masks real failures
- broad catches that can hide unrelated errors
- missing user-actionable error reporting

#### comment-analyzer

Audit comments/docstrings for long-term accuracy.

Flag:
- comments that contradict implementation
- stale TODO/FIXME and outdated assumptions
- comments that describe obvious code instead of intent
- missing documentation for non-obvious public behavior

#### type-design-analyzer

Audit type/schema design quality and invariants.

Evaluate:
- encapsulation and illegal-state prevention
- invariant clarity and enforcement
- nullability/optionality correctness
- schema strictness vs over-permissiveness

#### pr-test-analyzer

Audit test adequacy for changed behavior.

Flag:
- missing coverage for critical paths and edge cases
- missing negative/error-path tests
- brittle tests tied to implementation details
- weak assertions that would miss regressions

### 1d) Tier 1 exit rules

- Any Critical or High finding -> `BLOCKED` (stop)
- Any Medium finding -> `NEEDS_CHANGES` (stop)
- Otherwise continue to Tier 2

## Phase 2 - Reality assessment (required when changes exist)

Run a skeptical completion audit focused on what actually works, not what appears implemented.

Validate:

- behavior correctness end-to-end
- integration completeness (no stubs or half-wired seams)
- alignment between claimed completion and real implementation
- practical operability under realistic failure paths

When feasible, run representative verification commands for touched areas (tests/build/lint) and include results.

Tier 2 exits:

- severe functional gaps -> `BLOCKED`
- actionable but non-severe gaps -> `NEEDS_CHANGES`
- no material gaps -> `VERIFIED`

## Phase 3 - Simplification pass (optional, only if VERIFIED)

If `VERIFIED` and code changed, provide non-blocking simplification suggestions.

Focus:

- remove unnecessary abstraction/nesting
- improve readability without behavior change
- reduce cognitive load and maintenance risk

Do not change code in this command; suggestions only.

## Output format (strict)

```markdown
DECISION: VERIFIED | NEEDS_CHANGES | BLOCKED
TIER_REACHED: 1 | 2 | 3
SEVERITY_SUMMARY: Critical=<n>, High=<n>, Medium=<n>, Low=<n>

SCOPE:
- arguments: <value or none>
- files_reviewed: <count>

CHANGE_PROFILE:
- has_code_changes: <true|false>
- has_error_handling_changes: <true|false>
- has_comments_or_docs_changes: <true|false>
- has_type_or_schema_changes: <true|false>
- has_test_changes: <true|false>
- docs_only: <true|false>
- config_only: <true|false>

CHECKS_RUN:
- Tier 1: <comma-separated checks or skipped>
- Tier 2: reality-assessment <run|skipped>
- Tier 3: code-simplifier <run|skipped>

FINDINGS:
- [severity] [check] file:line - issue and impact; recommended fix

REALITY_GAPS:
- [gap severity] claim vs actual behavior (include evidence)

SIMPLIFICATION_SUGGESTIONS:
- optional improvements (only when DECISION=VERIFIED)

NEXT_ACTION:
- one concrete operator step
```

## Rules

- Read-only review; do not modify files
- Never auto-fix without explicit user request
- Keep findings concrete with `file:line` references
- If a check is skipped, state why
- Empty diff exits early with `Nothing to verify`
