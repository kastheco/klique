# Cleanup Superpowers Legacy CLI References Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all banned legacy CLI tool references (`grep`, `awk`, `wc -l`) in superpowers skills with their modern counterparts (`rg`, `ast-grep`, `sd`, `scc`).

**Architecture:** Sweep through all `.md` files under `~/.config/opencode/skills/superpowers/` and `~/.claude/skills/`, replacing banned tool invocations with modern equivalents. Some references are in example code blocks (need contextual replacement), others are in prose (simpler text swaps). A few cases use `grep` inside pipelines (`| grep`) where `rg` is the drop-in replacement.

**Tech Stack:** `sd` for simple replacements, manual edits for contextual changes

---

## Audit Summary

**Files with violations (superpowers — `~/.config/opencode/skills/superpowers/`):**

| File | Tool | Line | Context |
|------|------|------|---------|
| `using-git-worktrees/SKILL.md` | `grep` | 33 | `grep -i "worktree.*director" CLAUDE.md` |
| `writing-skills/examples/CLAUDE_MD_TESTING.md` | `grep` | 85,95,123 | `grep -r "keyword"` in example CLAUDE.md variants |
| `writing-skills/anthropic-best-practices.md` | `grep` | 323-328 | `grep -i "revenue"` in BigQuery example |
| `requesting-code-review/SKILL.md` | `grep`+`awk` | 56 | `git log \| grep "Task 1" \| head -1 \| awk '{print $1}'` |
| `receiving-code-review/SKILL.md` | `grep` | 92 | `grep codebase for actual usage` (prose) |
| `finishing-a-development-branch/SKILL.md` | `grep` | 142 | `git worktree list \| grep $(git branch --show-current)` |
| `systematic-debugging/SKILL.md` | `grep` | 97 | `env \| grep IDENTITY` |
| `systematic-debugging/root-cause-tracing.md` | `grep` | 89 | `npm test 2>&1 \| grep 'DEBUG git init'` |

**Files with violations (`~/.claude/skills/`):**

| File | Tool | Lines | Context |
|------|------|-------|---------|
| `project-auditor/references/vibe-code-patterns.md` | `grep`+`wc -l` | 31,413-441 | Heavy grep usage throughout audit commands |
| `project-auditor/references/code-quality-checklist.md` | `grep` | 12 | `grep -r ": any"` in checklist table |
| `spec-kitty/SKILL.md` | `grep` | 331 | "7 security grep commands must pass" (prose) |

**Not violations (kept as-is):**
- `git diff` references — `git diff` is a git subcommand, not standalone `diff`
- `verification-before-completion/SKILL.md` — only mentions "VCS diff", not the `diff` command

---

## Wave 1: Superpowers Core Skills

### Task 1: Fix using-git-worktrees/SKILL.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/using-git-worktrees/SKILL.md:33`

**Step 1: Replace grep with rg**

Change line 33 from:
```bash
grep -i "worktree.*director" CLAUDE.md 2>/dev/null
```
to:
```bash
rg -i "worktree.*director" CLAUDE.md 2>/dev/null
```

**Step 2: Verify the change**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/using-git-worktrees/SKILL.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/using-git-worktrees/SKILL.md
git commit -m "fix(skills): replace grep with rg in using-git-worktrees"
```

### Task 2: Fix requesting-code-review/SKILL.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/requesting-code-review/SKILL.md:56`

**Step 1: Replace grep+awk pipeline with git log format**

Change line 56 from:
```bash
BASE_SHA=$(git log --oneline | grep "Task 1" | head -1 | awk '{print $1}')
```
to:
```bash
BASE_SHA=$(git log --oneline | rg "Task 1" | head -1 | cut -d' ' -f1)
```

Note: `awk '{print $1}'` is replaced with `cut -d' ' -f1` which is a simple field extractor, not a full awk program. Alternatively `git log --format='%h' --grep="Task 1" -1` avoids the pipeline entirely, but keeping the pipeline form matches the existing example style.

**Step 2: Verify**

Run: `rg 'grep|awk' ~/.config/opencode/skills/superpowers/requesting-code-review/SKILL.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/requesting-code-review/SKILL.md
git commit -m "fix(skills): replace grep+awk with rg+cut in requesting-code-review"
```

### Task 3: Fix finishing-a-development-branch/SKILL.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/finishing-a-development-branch/SKILL.md:142`

**Step 1: Replace grep with rg**

Change line 142 from:
```bash
git worktree list | grep $(git branch --show-current)
```
to:
```bash
git worktree list | rg $(git branch --show-current)
```

**Step 2: Verify**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/finishing-a-development-branch/SKILL.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/finishing-a-development-branch/SKILL.md
git commit -m "fix(skills): replace grep with rg in finishing-a-development-branch"
```

### Task 4: Fix receiving-code-review/SKILL.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/receiving-code-review/SKILL.md:92`

**Step 1: Replace prose reference**

Change line 92 from:
```
  grep codebase for actual usage
```
to:
```
  rg codebase for actual usage
```

**Step 2: Verify**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/receiving-code-review/SKILL.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/receiving-code-review/SKILL.md
git commit -m "fix(skills): replace grep with rg in receiving-code-review"
```

### Task 5: Fix systematic-debugging/SKILL.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/systematic-debugging/SKILL.md:97`

**Step 1: Replace grep with rg in env pipeline**

Change line 97 from:
```bash
env | grep IDENTITY || echo "IDENTITY not in environment"
```
to:
```bash
env | rg IDENTITY || echo "IDENTITY not in environment"
```

**Step 2: Verify**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/systematic-debugging/SKILL.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/systematic-debugging/SKILL.md
git commit -m "fix(skills): replace grep with rg in systematic-debugging"
```

### Task 6: Fix systematic-debugging/root-cause-tracing.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/systematic-debugging/root-cause-tracing.md:89`

**Step 1: Replace grep with rg**

Change line 89 from:
```bash
npm test 2>&1 | grep 'DEBUG git init'
```
to:
```bash
npm test 2>&1 | rg 'DEBUG git init'
```

**Step 2: Verify**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/systematic-debugging/root-cause-tracing.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/systematic-debugging/root-cause-tracing.md
git commit -m "fix(skills): replace grep with rg in root-cause-tracing"
```

## Wave 2: Writing Skills (Example Content)

These files contain `grep` inside example CLAUDE.md content that teaches users how to structure their own instructions. The examples should model modern tool usage.

### Task 7: Fix writing-skills/examples/CLAUDE_MD_TESTING.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/writing-skills/examples/CLAUDE_MD_TESTING.md:85,95,123`

**Step 1: Replace all three grep references with rg**

Line 85: `grep -r "keyword" ~/.claude/skills/` → `rg "keyword" ~/.claude/skills/`
Line 95: `grep -r "keyword" ~/.claude/skills/ --include="SKILL.md"` → `rg "keyword" ~/.claude/skills/ -g "SKILL.md"`
Line 123: `grep -r "symptom" ~/.claude/skills/` → `rg "symptom" ~/.claude/skills/`

Note: `rg` respects `.gitignore` and is recursive by default, so `-r` is dropped. The `--include` flag becomes `-g` glob.

**Step 2: Verify**

Run: `rg 'grep' ~/.config/opencode/skills/superpowers/writing-skills/examples/CLAUDE_MD_TESTING.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/writing-skills/examples/CLAUDE_MD_TESTING.md
git commit -m "fix(skills): replace grep with rg in CLAUDE_MD_TESTING examples"
```

### Task 8: Fix writing-skills/anthropic-best-practices.md

**Files:**
- Modify: `~/.config/opencode/skills/superpowers/writing-skills/anthropic-best-practices.md:323-328`

**Step 1: Replace grep references in BigQuery example**

This is an example CLAUDE.md snippet showing data analysis. Replace:
```
Find specific metrics using grep:

```bash
grep -i "revenue" reference/finance.md
grep -i "pipeline" reference/sales.md
grep -i "api usage" reference/product.md
```
```

With:
```
Find specific metrics using rg:

```bash
rg -i "revenue" reference/finance.md
rg -i "pipeline" reference/sales.md
rg -i "api usage" reference/product.md
```
```

**Step 2: Verify**

Run: `rg '\bgrep\b' ~/.config/opencode/skills/superpowers/writing-skills/anthropic-best-practices.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.config/opencode/skills/superpowers/writing-skills/anthropic-best-practices.md
git commit -m "fix(skills): replace grep with rg in anthropic-best-practices example"
```

## Wave 3: External Skills (~/.claude/skills/)

### Task 9: Fix project-auditor/references/vibe-code-patterns.md

**Files:**
- Modify: `~/.claude/skills/project-auditor/references/vibe-code-patterns.md:31,413-441`

**Step 1: Replace all grep and wc -l references**

This file has heavy grep usage in audit commands. Replace all instances:

| Original | Replacement |
|----------|-------------|
| `grep -r "TODO\|FIXME\|HACK\|XXX" --include="*.py" ...` | `rg "TODO\|FIXME\|HACK\|XXX" -g "*.py" -g "*.ts" -g "*.js"` |
| `grep -rn "pass$\|return$\|{}" --include="*.py"` | `rg -n "pass$\|return$\|{}" -g "*.py"` |
| `grep -r "console.log\|print(" --include="*.py" ...` | `rg "console.log\|print(" -g "*.py" -g "*.ts" -g "*.js"` |
| `grep -rE "https?://[a-z0-9]" ... \| grep -v test \| grep -v node_modules` | `rg -E "https?://[a-z0-9]" -g "*.py" -g "*.ts" -g "*.js" -g "!*test*" -g "!node_modules"` |
| `grep -rE "(password\|secret\|key\|token)..."` | `rg -E "(password\|secret\|key\|token)\s*=\s*['\"][^'\"]+['\"]" -g "*.py" -g "*.ts" -g "*.js"` |
| `grep -r "class.*:" --include="*.py" \| wc -l` | `rg -c "class.*:" -g "*.py" \| rg -v '^0$'` (or just `rg "class.*:" -g "*.py" --count-matches`) |
| `grep -r "Factory" --include="*.py" --include="*.ts"` | `rg "Factory" -g "*.py" -g "*.ts"` |
| `grep -r "try:" --include="*.py" \| wc -l` | `rg "try:" -g "*.py" --count-matches` |
| `grep -r "await " --include="*.py" \| wc -l` | `rg "await " -g "*.py" --count-matches` |
| `npm ls --all 2>&1 \| grep "UNMET"` | `npm ls --all 2>&1 \| rg "UNMET"` |

**Step 2: Verify**

Run: `rg 'grep|wc -l' ~/.claude/skills/project-auditor/references/vibe-code-patterns.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.claude/skills/project-auditor/references/vibe-code-patterns.md
git commit -m "fix(skills): replace grep+wc with rg in vibe-code-patterns"
```

### Task 10: Fix project-auditor/references/code-quality-checklist.md

**Files:**
- Modify: `~/.claude/skills/project-auditor/references/code-quality-checklist.md:12`

**Step 1: Replace grep in checklist table**

Change:
```
| No `any` types | No explicit `any` usage | `grep -r ": any" --include="*.ts"` |
```
to:
```
| No `any` types | No explicit `any` usage | `rg ": any" -g "*.ts"` |
```

**Step 2: Verify**

Run: `rg 'grep' ~/.claude/skills/project-auditor/references/code-quality-checklist.md`
Expected: no matches

**Step 3: Commit**

```bash
git add ~/.claude/skills/project-auditor/references/code-quality-checklist.md
git commit -m "fix(skills): replace grep with rg in code-quality-checklist"
```

### Task 11: Review spec-kitty/SKILL.md (prose-only — no change needed)

**Files:**
- Review: `~/.claude/skills/spec-kitty/SKILL.md:331`

Line 331 says: "7 security grep commands must pass". This is describing spec-kitty's own workflow which we don't control. **Skip — no change.** Spec-kitty's internal security checks are its own concern.

## Wave 4: Verification

### Task 12: Full audit — confirm zero remaining violations

**Step 1: Scan superpowers for any remaining banned tools**

```bash
rg '\bgrep\b|\bsed\b|\bawk\b|\bwc -l\b' ~/.config/opencode/skills/superpowers/ -g '*.md'
```

Expected: no matches

**Step 2: Scan ~/.claude/skills for remaining violations (excluding spec-kitty)**

```bash
rg '\bgrep\b|\bsed\b|\bawk\b|\bwc -l\b' ~/.claude/skills/ -g '*.md' --glob='!spec-kitty/**'
```

Expected: no matches

**Step 3: Squash commits into single cleanup commit**

```bash
git rebase -i HEAD~10  # squash all fix(skills) commits
git commit --amend -m "fix(skills): replace all legacy CLI tool references with modern counterparts

Replace grep→rg, awk→cut, wc -l→rg --count-matches across all
superpowers and external skills. Aligns skill examples with the
cli-tools hard-gate banning grep/sed/awk/diff/wc."
```
