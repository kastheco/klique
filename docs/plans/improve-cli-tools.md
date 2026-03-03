# Improve CLI Tools Skill — Hard Ban Legacy Tools

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Harden the cli-tools skill so legacy unix tools (`grep`, `sed`, `awk`, `diff`, `wc -l`) are explicitly banned with hard-gate language, not merely discouraged.

**Architecture:** Add a `<HARD-GATE>` banned tools section to SKILL.md, harden "Common Mistakes" into "Violations", remove "legacy fallback" language from resource files, then propagate identical changes to all 3 copies of the skill tree.

**Tech Stack:** Markdown skill files, `sd` for string replacements

---

## Wave 1: Core skill changes

### Task 1: Add banned tools hard-gate to SKILL.md

**Files:**
- Modify: `.opencode/skills/cli-tools/SKILL.md:6-8`

**Step 1: Add the banned tools section**

Insert the following block between the intro paragraph (line 8) and the `## Tool Selection by Task` header (line 10). The new content goes after line 8 (`...than regex-based alternatives.`) and before the blank line + `## Tool Selection by Task`.

Replace this in `.opencode/skills/cli-tools/SKILL.md`:

```
Modern CLI tools that replace legacy unix utilities for code-related tasks. These tools understand syntax, structure, and context — producing safer, faster, and more accurate results than regex-based alternatives.

## Tool Selection by Task
```

With:

```
Modern CLI tools that replace legacy unix utilities for code-related tasks. These tools understand syntax, structure, and context — producing safer, faster, and more accurate results than regex-based alternatives.

<HARD-GATE>
## Banned Tools

These legacy tools are NEVER permitted. Using them is a violation, not a preference.

| Banned | Replacement | No Exceptions |
|--------|-------------|---------------|
| `grep` | `rg` (ripgrep) | Even for simple searches. `rg` is always faster and respects .gitignore |
| `sed` | `sd` | Even for one-liners. `sd` has saner syntax and no delimiter escaping |
| `awk` | `yq`/`jq` (structured) or `sd` (text) | No awk for any purpose |
| `diff` (standalone) | `difft` | `git diff` is fine — standalone `diff` is not |
| `wc -l` | `scc` | Even for single files |

**`git diff` is allowed** — it's a git subcommand, not standalone `diff`. Use `GIT_EXTERNAL_DIFF=difft git diff` when reviewing code changes.

If you catch yourself thinking "grep is fine for this simple case" — it's not. Use `rg`.
</HARD-GATE>

## Tool Selection by Task
```

**Step 2: Run `rg 'HARD-GATE' .opencode/skills/cli-tools/SKILL.md` to verify the section was added**

Expected: 2 matches (opening and closing tags)

**Step 3: Commit**

```bash
git add .opencode/skills/cli-tools/SKILL.md
git commit -m "feat(cli-tools): add hard-gate banned tools section"
```

---

### Task 2: Rename "Common Mistakes" to "Violations" and harden language

**Files:**
- Modify: `.opencode/skills/cli-tools/SKILL.md:149-157`

**Step 1: Replace the Common Mistakes section**

Replace this at the bottom of `.opencode/skills/cli-tools/SKILL.md`:

```
## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Using `sed` for code refactoring | Use `ast-grep` or `comby` — they understand syntax |
| Splitting `{` / `}` in comby templates | Always inline: `{:[body]}` not `{\n:[body]\n}` |
| Forgetting `-in-place` with comby | Without it, comby only previews changes |
| Using `grep` to find code patterns | Use `ast-grep --pattern` for syntax-aware search |
| Using `diff` for code review | Use `difft` for syntax-aware structural diffs |
| Manual YAML editing with `sed` | Use `yq` to preserve structure and formatting |
| Using `wc -l` for project metrics | Use `scc` for language-aware counts + complexity |
```

With:

```
## Violations

These are not suggestions. If you do any of these, you are violating the skill.

| Violation | Required Fix |
|-----------|-------------|
| Using `grep` for anything | Use `rg` for text search, `ast-grep` for code patterns |
| Using `sed` for anything | Use `sd` for replacements, `ast-grep`/`comby` for refactoring |
| Using `awk` for anything | Use `yq`/`jq` for structured data, `sd` for text processing |
| Using standalone `diff` | Use `difft` for syntax-aware structural diffs |
| Using `wc -l` for counting | Use `scc` for language-aware counts + complexity |
| Splitting `{` / `}` in comby templates | Always inline: `{:[body]}` not `{\n:[body]\n}` |
| Forgetting `-in-place` with comby | Without it, comby only previews changes |
```

**Step 2: Run `rg 'Violations' .opencode/skills/cli-tools/SKILL.md` to verify**

Expected: 2 matches (the header and the intro sentence)

**Step 3: Commit**

```bash
git add .opencode/skills/cli-tools/SKILL.md
git commit -m "feat(cli-tools): rename common mistakes to violations with hard language"
```

---

### Task 3: Remove "legacy fallback" language from resource files

**Files:**
- Modify: `.opencode/skills/cli-tools/resources/sd.md:85`
- Modify: `.opencode/skills/cli-tools/resources/difftastic.md:75`
- Modify: `.opencode/skills/cli-tools/resources/scc.md:91`

**Step 1: Fix sd.md**

Replace this line in `.opencode/skills/cli-tools/resources/sd.md`:

```
- **sed**: Legacy fallback. sd is strictly better for interactive use — simpler syntax, sane defaults.
```

With:

```
- **sed**: BANNED. Use `sd` instead — always.
```

**Step 2: Fix difftastic.md**

Replace this line in `.opencode/skills/cli-tools/resources/difftastic.md`:

```
- **diff**: Legacy fallback, no syntax awareness
```

With:

```
- **diff**: BANNED. Use `difft` instead — always. (`git diff` is fine, standalone `diff` is not.)
```

**Step 3: Fix scc.md**

Replace this line in `.opencode/skills/cli-tools/resources/scc.md`:

```
- **wc -l**: Only when you literally need raw line count of a single file
```

With:

```
- **wc -l**: BANNED. Use `scc` instead — even for single files.
```

**Step 4: Run `rg 'BANNED' .opencode/skills/cli-tools/resources/` to verify**

Expected: 3 matches, one per file

**Step 5: Commit**

```bash
git add .opencode/skills/cli-tools/resources/sd.md \
       .opencode/skills/cli-tools/resources/difftastic.md \
       .opencode/skills/cli-tools/resources/scc.md
git commit -m "feat(cli-tools): ban legacy tools in resource files"
```

---

## Wave 2: Propagate to all copies

### Task 4: Sync changes to `.agents/skills/cli-tools/`

**Files:**
- Modify: `.agents/skills/cli-tools/SKILL.md`
- Modify: `.agents/skills/cli-tools/resources/sd.md`
- Modify: `.agents/skills/cli-tools/resources/difftastic.md`
- Modify: `.agents/skills/cli-tools/resources/scc.md`

**Step 1: Copy all changed files**

```bash
cp .opencode/skills/cli-tools/SKILL.md .agents/skills/cli-tools/SKILL.md
cp .opencode/skills/cli-tools/resources/sd.md .agents/skills/cli-tools/resources/sd.md
cp .opencode/skills/cli-tools/resources/difftastic.md .agents/skills/cli-tools/resources/difftastic.md
cp .opencode/skills/cli-tools/resources/scc.md .agents/skills/cli-tools/resources/scc.md
```

**Step 2: Verify with diff**

```bash
difft .opencode/skills/cli-tools/SKILL.md .agents/skills/cli-tools/SKILL.md
difft .opencode/skills/cli-tools/resources/sd.md .agents/skills/cli-tools/resources/sd.md
difft .opencode/skills/cli-tools/resources/difftastic.md .agents/skills/cli-tools/resources/difftastic.md
difft .opencode/skills/cli-tools/resources/scc.md .agents/skills/cli-tools/resources/scc.md
```

Expected: All 4 diffs should show no differences.

**Step 3: Commit**

```bash
git add .agents/skills/cli-tools/
git commit -m "feat(cli-tools): sync hardened skill to .agents/skills/"
```

---

### Task 5: Sync changes to scaffold templates

**Files:**
- Modify: `internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/sd.md`
- Modify: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/difftastic.md`
- Modify: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/scc.md`

**Step 1: Copy all changed files**

```bash
cp .opencode/skills/cli-tools/SKILL.md internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md
cp .opencode/skills/cli-tools/resources/sd.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/sd.md
cp .opencode/skills/cli-tools/resources/difftastic.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/difftastic.md
cp .opencode/skills/cli-tools/resources/scc.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/scc.md
```

**Step 2: Verify with diff**

```bash
difft .opencode/skills/cli-tools/SKILL.md internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md
difft .opencode/skills/cli-tools/resources/sd.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/sd.md
difft .opencode/skills/cli-tools/resources/difftastic.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/difftastic.md
difft .opencode/skills/cli-tools/resources/scc.md internal/initcmd/scaffold/templates/skills/cli-tools/resources/scc.md
```

Expected: All 4 diffs should show no differences.

**Step 3: Commit**

```bash
git add internal/initcmd/scaffold/templates/skills/cli-tools/
git commit -m "feat(cli-tools): sync hardened skill to scaffold templates"
```

---

## Wave 3: Final verification

### Task 6: Verify all copies are identical and no legacy language remains

**Files:**
- Read: all 3 copies of SKILL.md and resource files

**Step 1: Verify no "legacy fallback" language remains anywhere**

```bash
rg -i 'legacy fallback' .opencode/skills/cli-tools/ .agents/skills/cli-tools/ internal/initcmd/scaffold/templates/skills/cli-tools/
```

Expected: 0 matches

**Step 2: Verify HARD-GATE exists in all copies**

```bash
rg 'HARD-GATE' .opencode/skills/cli-tools/SKILL.md .agents/skills/cli-tools/SKILL.md internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md
```

Expected: 6 matches (2 per file — opening and closing tags)

**Step 3: Verify BANNED exists in all resource copies**

```bash
rg 'BANNED' .opencode/skills/cli-tools/resources/ .agents/skills/cli-tools/resources/ internal/initcmd/scaffold/templates/skills/cli-tools/resources/
```

Expected: 9 matches (3 per directory — one in sd.md, difftastic.md, scc.md each)

**Step 4: Verify "Violations" header exists in all SKILL.md copies**

```bash
rg '## Violations' .opencode/skills/cli-tools/SKILL.md .agents/skills/cli-tools/SKILL.md internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md
```

Expected: 3 matches (one per file)

**Step 5: Verify all 3 SKILL.md copies are byte-identical**

```bash
diff <(cat .opencode/skills/cli-tools/SKILL.md) <(cat .agents/skills/cli-tools/SKILL.md) && echo "opencode == agents: OK"
diff <(cat .opencode/skills/cli-tools/SKILL.md) <(cat internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md) && echo "opencode == templates: OK"
```

Expected: Both print OK with no diff output.
