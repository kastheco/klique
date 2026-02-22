---
name: cli-tools
description: Use when performing code search, refactoring, file transformations, diffing, string replacement, structured data processing, spell checking, or codebase metrics. Covers ast-grep, comby, difftastic, sd, yq, typos, and scc. Use instead of legacy tools like grep, sed, awk, diff, and wc for faster, safer, and more accurate results.
---

# CLI Tools

Modern CLI tools that replace legacy unix utilities for code-related tasks. These tools understand syntax, structure, and context — producing safer, faster, and more accurate results than regex-based alternatives.

## Tool Selection by Task

Pick the right tool for the job. When multiple tools could work, the table shows the preferred choice and why.

| Task | Use | Not | Why |
|------|-----|-----|-----|
| Rename symbol across files | `ast-grep` | `sed`/`sd` | AST-aware, won't rename inside strings/comments |
| Structural code rewrite | `comby` | `sed`/`awk` | Understands balanced delimiters, nesting |
| Find code pattern | `ast-grep --pattern` | `grep`/`rg` | Matches syntax, not text |
| Find literal string | `rg` (ripgrep) | `grep` | Fast, respects .gitignore |
| Replace string in files | `sd` | `sed` | No delimiter escaping, sane defaults |
| Read/modify YAML/TOML/JSON | `yq` / `jq` | `sed`/`awk` | Understands structure, preserves formatting |
| Review code changes | `difft` | `diff`/`git diff` | Syntax-aware, ignores formatting noise |
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
