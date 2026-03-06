---
name: cli-tools
description: Use when performing code search, refactoring, file transformations, diffing, string replacement, structured data processing, spell checking, or codebase metrics — or when about to reach for grep, sed, awk, find, diff, or wc.
---

# CLI Tools

Modern CLI tools that replace legacy unix utilities for code-related tasks. These tools understand syntax, structure, and context — producing safer, faster, and more accurate results than regex-based alternatives.

<HARD-GATE>
## Banned Tools

These legacy tools are NEVER permitted. Using them is a violation, not a preference.

| Banned | Replacement | No Exceptions |
|--------|-------------|---------------|
| `grep` | `rg` (ripgrep) | Even for simple one-liners. `rg` is faster, respects .gitignore, and handles encoding correctly |
| `grep -r` | `rg` | Recursive grep is still grep. Always `rg` |
| `grep -E` | `rg` | Extended regex is still grep. `rg` supports the same patterns |
| `sed` | `sd` | Even for one-liners. `sd` has saner syntax and no delimiter escaping |
| `awk` | `yq`/`jq` (structured) or `sd` (text) | No awk for any purpose |
| `find` | `fd` or glob tools | Even for simple file listing. `fd` respects .gitignore; use `fd -e go` for extension |
| `diff` (standalone) | `difft` | `git diff` is fine — standalone `diff` is not |
| `wc -l` | `scc` | Even for single files |

**`git diff` is allowed** — it's a git subcommand, not standalone `diff`. Use `GIT_EXTERNAL_DIFF=difft git diff` when reviewing code changes.

**STOP.** If you are about to type `grep`, `sed`, `awk`, `find`, `diff`, or `wc` — stop and use the replacement. There are no exceptions. "Just this once" is a violation.
</HARD-GATE>

## Tool Selection by Task

Pick the right tool for the job. When multiple tools could work, the table shows the preferred choice and why.

| Task | Use | Not | Why |
|------|-----|-----|-----|
| Rename symbol across files | `ast-grep` | `sed`/`sd` | AST-aware, won't rename inside strings/comments |
| Structural code rewrite | `comby` | `sed`/`awk` | Understands balanced delimiters, nesting |
| Find code pattern | `ast-grep --pattern` | `grep`/`rg` | Matches syntax, not text |
| Find literal string | `rg` | `grep` | Fast, respects .gitignore, correct encoding |
| Find files by name/extension | `fd` | `find` | Respects .gitignore, simpler syntax |
| Replace string in files | `sd` | `sed` | No delimiter escaping, sane defaults |
| Read/modify YAML/TOML/JSON | `yq` / `jq` | `sed`/`awk` | Understands structure, preserves formatting |
| Review code changes | `difft` | `diff` | Syntax-aware, ignores formatting noise |
| Spell check code | `typos` | manual | Understands camelCase, identifiers |
| Count lines / codebase metrics | `scc` | `wc -l`/`cloc` | Fast, includes complexity estimates |
| Multi-line structural rewrite | `comby` | `sed` | Handles cross-line patterns with holes |
| Add/change function params | `comby` | manual | `func F(:[params])` patterns |
| Simple literal find-replace | `sd` | `sed` | Simpler syntax, in-place by default |

## ast-grep vs comby Decision

Both do structural code operations. Choose based on the transformation:

| Scenario | Winner | Reason |
|----------|--------|--------|
| Rename all calls to a function | ast-grep | AST-level symbol matching |
| Find all usages of a pattern | ast-grep | Tree-sitter precision |
| Add/change function parameters | comby | Hole syntax `:[params]` is natural |
| Replace entire function body | comby | `{:[_]}` discards balanced block |
| Insert code after a function | comby | Match + append in rewrite |
| Add to import block | comby | `import (:[imports])` pattern |
| Multi-line structural rewrite | comby | Balanced delimiter matching |
| Lint-style pattern detection | ast-grep | Rule-based scanning |

## Quick Reference

### rg (ripgrep)

Fast text search that respects .gitignore. Primary replacement for all `grep` variants.

```bash
rg 'pattern' [path]                            # basic search (recursive by default)
rg 'pattern' --type go                         # filter by language type
rg 'pattern' -g '*.go'                         # filter by glob
rg 'pattern' -g '*.go' -g '!*_test.go'         # include/exclude globs
rg -i 'pattern'                                # case insensitive
rg -F 'literal[0]'                             # fixed string (no regex)
rg -U 'start\n.*end'                           # multiline match
rg -l 'pattern'                                # list matching files only
rg -c 'pattern'                                # count matches per file
rg --no-heading -n 'pattern'                   # machine-readable output (file:line:match)
```

**In-depth reference:** [resources/rg.md](resources/rg.md)

### ast-grep (`ast-grep`)

AST-based code search and replace using tree-sitter. Matches syntax structure, not text.

```bash
ast-grep run --pattern 'fmt.Errorf($$$)' --lang go          # find all calls
ast-grep run --pattern 'errors.New($A)' --rewrite 'fmt.Errorf($A)' --lang go  # replace
ast-grep run --pattern '$A != nil' --lang go src/            # search specific path
```

Key patterns: `$VAR` (single node), `$$$` (variable-length args), `$_` (wildcard)

**In-depth reference:** [resources/ast-grep.md](resources/ast-grep.md)

### comby (`comby`)

Structural search/replace that understands balanced delimiters, strings, and comments. Superior to regex for multi-line code transformations.

```bash
comby 'func :[name](:[args]) {:[body]}' 'func :[name](:[args]) error {:[body]}' .go -in-place
comby 'import (:[imports])' 'import (:[imports]
	"new/package"
)' file.go -in-place
```

**Critical rule:** Always use `{:[body]}` (inline braces), never split `{` and `}` on separate lines.

Key holes: `:[var]` (everything), `:[[var]]` (word), `:[_]` (unnamed wildcard), `:[var\n]` (to newline)

**In-depth reference:** [resources/comby.md](resources/comby.md) — read this before any non-trivial comby usage. Contains critical pitfalls around whitespace normalization and balanced delimiter matching.

### difftastic (`difft`)

Syntax-aware structural diff. Dramatically cleaner output for refactors and moves.

```bash
difft old.go new.go                              # compare files
GIT_EXTERNAL_DIFF=difft git diff                  # git integration
GIT_EXTERNAL_DIFF=difft git log -p -- path/file   # file history
difft --display inline old.go new.go              # single-column output
```

**In-depth reference:** [resources/difftastic.md](resources/difftastic.md)

### sd (`sd`)

Modern sed alternative. In-place by default, no delimiter escaping needed.

```bash
sd 'old_name' 'new_name' src/**/*.go              # replace in files
sd 'version = "(\d+)"' 'version = "2"' file.toml  # regex with groups
sd -p 'foo' 'bar' file.txt                         # preview (dry run)
sd -F 'literal[0]' 'replacement' file.txt          # fixed string mode
```

**In-depth reference:** [resources/sd.md](resources/sd.md)

### yq (`yq`)

YAML/JSON processor (jq wrapper for YAML). Query and modify structured config files.

```bash
yq '.metadata.name' file.yaml                     # read field
yq -y '.key = "value"' file.yaml                   # modify YAML
yq -y '.dependencies += {"new": "^1.0"}' pkg.yaml  # add to map
cat file.yaml | yq '.items[].name'                 # query array
```

Note: This is the Python-based `yq` (kislyuk/yq), a jq wrapper. Uses jq filter syntax. Use `-y` flag for YAML output, otherwise outputs JSON.

**In-depth reference:** [resources/yq.md](resources/yq.md)

### typos (`typos`)

Fast source code spell checker. Understands identifiers, camelCase, filenames.

```bash
typos                          # check entire project
typos src/                     # check specific path
typos --write-changes          # auto-fix typos
typos --format brief           # compact output
```

Run before commits to catch typos in identifiers, strings, and comments.

**In-depth reference:** [resources/typos.md](resources/typos.md)

### scc (`scc`)

Fast code counter with complexity estimates. Useful for project scope assessment.

```bash
scc                                  # full project summary
scc internal/                        # specific directory
scc --by-file --sort lines           # per-file breakdown
scc --not-match '_test.go$'          # exclude test files
scc --include-ext go,ts              # specific languages only
```

**In-depth reference:** [resources/scc.md](resources/scc.md)

## Violations

These violations are not suggestions. If you do any of these, you are violating the skill.

| Violation | Required Fix |
|-----------|-------------|
| Using `grep` for anything | Use `rg` for text search, `ast-grep` for code patterns |
| Using `grep -r` (recursive) | Use `rg` — recursive by default, also respects .gitignore |
| Using `grep -E` (extended regex) | Use `rg` — supports full regex including extended patterns |
| Using `sed` for anything | Use `sd` for replacements, `ast-grep`/`comby` for refactoring |
| Using `awk` for anything | Use `yq`/`jq` for structured data, `sd` for text processing |
| Using `find` for anything | Use `fd` for file finding, glob tools for path patterns |
| Using standalone `diff` | Use `difft` for syntax-aware structural diffs |
| Using `wc -l` for counting | Use `scc` for language-aware counts + complexity |
| Splitting `{` / `}` in comby templates | Always inline: `{:[body]}` not `{\n:[body]\n}` |
| Forgetting `-in-place` with comby | Without it, comby only previews changes |
