Review the implementation of plan: {{PLAN_NAME}}

Retrieve the plan content with `kas task show {{PLAN_FILE}}` to understand the goals,
architecture, and tasks that were implemented.

IMPORTANT: Only review changes from this branch. Use `git diff $MERGE_BASE..HEAD` to see exactly
what was changed by the implementation — do NOT review code that was inherited from main.
Files may contain code from main that is outside the scope of this plan.

## Worktree awareness

You are reviewing in a **git worktree**. The `main` branch may have advanced since this
worktree was created (other PRs merged). Always use `merge-base` to find the true branch
point — never compare against `main` directly.

Set the merge base once at the start of your review and use it everywhere:

```bash
MERGE_BASE=$(git merge-base main HEAD)
echo "merge base: $MERGE_BASE"
```

All diff and log commands below use `$MERGE_BASE` instead of `main`. This ensures you
only review changes from this branch, regardless of how far main has diverged.

---

## Preflight

Run the following commands before starting any phase:

```bash
git diff --stat $MERGE_BASE..HEAD
git diff $MERGE_BASE..HEAD
```

If the diff is empty, approve immediately:
```bash
kas signal emit review_approved {{PLAN_FILENAME}} --payload "Approved. empty diff — no changes to review."
```
Then stop. No further analysis is needed.

---

## Phase 0: Build review context

Gather the full scope of what changed on this branch:

```bash
git diff --name-only $MERGE_BASE..HEAD          # changed files
git diff --stat $MERGE_BASE..HEAD               # line counts per file
git log $MERGE_BASE..HEAD --oneline             # commit history
```

Review ONLY the branch diff. Do not critique code that pre-exists on main.

---

## Phase 1: Change profile + Tier 1 static analysis

### 1a. Derive change profile

From the diff, determine which categories apply (true/false):

```
has_code_changes:            {{true|false}}
has_error_handling_changes:  {{true|false}}
has_comments_or_docs_changes:{{true|false}}
has_type_or_schema_changes:  {{true|false}}
has_test_changes:            {{true|false}}
docs_only:                   {{true|false}}
config_only:                 {{true|false}}
```

If `docs_only` or `config_only` is true, skip specialist checks that don't apply.

### 1b. Specialist checks

Run each specialist check that applies based on the change profile above.
For each check that does NOT apply, write "skip — <reason>" and move on.

**core-code-reviewer** (when `has_code_changes`)
- correctness: does the logic do what the plan says?
- error paths: are errors propagated and not swallowed?
- resource leaks: goroutines, file handles, connections
- concurrency: shared state, missing locks

**silent-failure-hunter** (when `has_error_handling_changes`)
- errors ignored with `_`
- error values discarded silently
- functions that can fail but return no error
- `go func()` goroutines with no error channel

**comment-analyzer** (when `has_comments_or_docs_changes`)
- doc comments match exported function signatures exactly
- no misleading or stale comments
- no commented-out dead code

**type-design-analyzer** (when `has_type_or_schema_changes`)
- new types follow existing conventions in the package
- no unnecessary pointer indirection
- zero values are useful or explicitly handled

**pr-test-analyzer** (when `has_test_changes`)
- tests are table-driven where appropriate
- assertions use `require` for fatal conditions, `assert` for non-fatal
- no real network/tmux/git in unit tests
- test names describe the scenario, not the implementation

### 1c. Phase 1 exit criteria

Evaluate all findings:

- Any **critical** or **high** severity issue → verdict is `BLOCKED`, stop here
- Any **medium** severity issues only → verdict is `NEEDS_CHANGES`, continue to Phase 2 only to check for additional issues
- No issues, or only low/informational → continue to Phase 2

---

## Phase 2: Reality assessment

Skeptically audit whether the implementation actually delivers on the plan's stated goals.

### 2a. Completion audit

For each task listed in the plan (`kas task show {{PLAN_FILE}}`):
- Was it implemented?
- Does the implementation match the described approach?
- Are there gaps between what was described and what was written?

### 2b. Integration checks

- Do new functions/types integrate cleanly with existing code?
- Are imports correct and minimal?
- Does the code compile without errors?

### 2c. Behavior validation

Run the test suite and capture the outcome:

```bash
go test ./...
```

If tests fail, record which tests and what the error output says.

### 2d. Phase 2 exit criteria

- **Severe correctness gap** (plan goal not implemented, tests fail, build broken) → verdict is `BLOCKED`
- **Actionable gaps** (partial implementation, wrong approach, test failures) → verdict is `NEEDS_CHANGES`
- **No significant gap** → verdict is `VERIFIED`

---

## Phase 3: Simplification pass (optional — only when verdict is VERIFIED)

Only run this phase when the verdict from Phases 1 and 2 is `VERIFIED`.
Findings here are **non-blocking** and must not change the decision tier.

Look for:
- duplicated logic that could be extracted
- variable names that don't communicate intent
- complex expressions that could be simplified
- dead code introduced by this branch

Record suggestions in the signal file under `### suggestions (non-blocking)`.

---

## Machine-readable output

Before writing the signal file, output the following block:

```
DECISION: {{APPROVED|NEEDS_CHANGES|BLOCKED}}
TIER_REACHED: {{0|1|2|3}}
SEVERITY_SUMMARY: critical=N high=N medium=N low=N
ISSUES:
- [file:line] <description>
- [file:line] <description>
```

Replace `{{APPROVED|NEEDS_CHANGES|BLOCKED}}` with one value. Replace `{{0|1|2|3}}` with
the highest phase completed. List all issues with file:line references; write `none` if clean.

Mapping:
- `VERIFIED` with no medium+ issues → `APPROVED`
- Any medium issue, no blocking → `NEEDS_CHANGES`
- Any critical/high issue, or severe correctness gap → `BLOCKED`

---

## Self-fix protocol

For trivial issues, fix them yourself instead of kicking back to the coder:

**Self-fix (commit directly):**
- Typos in code, comments, or strings
- Missing or wrong doc comments
- Obvious one-liner fixes (wrong constant name, missing return)
- Import cleanup

**Kick to coder (emit `review_changes_requested`):**
- Anything requiring debugging or investigation
- Logic changes, even small ones
- Missing test coverage
- Architectural concerns

If only self-fixable issues remain, fix them all and emit `review_approved`.
If any coder-required issues exist, self-fix what you can first, then emit `review_changes_requested`.

---

## Signals

You MUST emit exactly one signal before you finish. Use `kas signal emit`; do not write
legacy `.kasmos/signals/review-*` files directly. Without a signal, the orchestrator
cannot progress the plan lifecycle.

**Approved** (zero coder-required issues remaining after self-fixes):
```bash
kas signal emit review_approved {{PLAN_FILENAME}} --payload "Approved. <brief summary>"
```

**Changes required** (issues that need a coder):
```bash
kas signal emit review_changes_requested {{PLAN_FILENAME}} --payload "$(cat <<'SIGNAL'
## review round N

### critical
- [file:line] description — why it matters

### high
- [file:line] description — why it matters

### medium
- [file:line] description — why it matters

### self-fixed (no action needed)
- [file:line] what was fixed

### suggestions (non-blocking)
- [file:line] optional improvement
SIGNAL
)"
```

Include the round number (1 for first review, 2 for re-review after fixes, etc.).
Omit empty tiers. Every item must have a `file:line` reference.
