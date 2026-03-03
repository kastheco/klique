# rg (ripgrep) Reference

Binary: `rg`
Version: 15.1.0

Fast, recursive text search that respects `.gitignore`. The required replacement for `grep` — always faster, always `.gitignore`-aware, and saner flag names.

## Core Commands

```bash
rg 'pattern'                       # search cwd recursively (default)
rg 'pattern' src/ internal/        # search specific paths
rg 'pattern' file.go               # search single file
rg 'pattern' -t go                 # all Go files (type-based)
rg 'pattern' -g '*.go'             # all .go files (glob-based)
```

## File Targeting — Agents Get This Wrong Constantly

### Type-based filtering (preferred over globs)

```bash
rg 'pattern' -t go                 # search Go files (uses built-in type defs)
rg 'pattern' -T test               # exclude test files (all languages)
rg 'pattern' -t go -T go-test      # Go files but not *_test.go
rg --type-list                     # show all built-in type names
```

### Glob-based filtering

```bash
rg 'pattern' -g '*.go'             # include only .go files
rg 'pattern' -g '!*_test.go'       # exclude test files
rg 'pattern' -g '*.{go,ts}'        # multiple extensions
rg 'pattern' -g 'src/**/*.go'      # files in specific subdir
```

### ⚠️ PITFALL: `--include` does not exist in rg

```bash
# WRONG — this is a grep flag, rg will error or ignore it
rg 'pattern' --include '*.go'

# RIGHT — use -t or -g
rg 'pattern' -t go
rg 'pattern' -g '*.go'
```

### When to use `-t` vs `-g`

- Use `-t go` when you want all Go source files (uses rg's built-in definition)
- Use `-g '*.go'` when you need precise glob control or rg has no type for that extension
- Prefer `-t` for common languages: `go`, `ts`, `js`, `py`, `rust`, `json`, `yaml`, `toml`

## Case Sensitivity — Default Is Smart Case, Not Case-Sensitive

rg defaults to **smart case**: case-insensitive when pattern is all-lowercase, case-sensitive when pattern contains any uppercase.

```bash
rg 'todo'          # case-insensitive (all lowercase pattern)
rg 'TODO'          # case-SENSITIVE (has uppercase) — NOT what you think
rg 'Todo'          # case-sensitive (has uppercase)

rg -i 'pattern'    # --ignore-case: always case-insensitive
rg -s 'pattern'    # --case-sensitive: always case-sensitive
rg -S 'pattern'    # --smart-case: explicit (same as default, useful in scripts)
```

### ⚠️ PITFALL: All-uppercase patterns are case-sensitive by default

```bash
# Searching for "TODO" — matches "TODO" only, NOT "todo" or "Todo"
rg 'TODO'         # case-sensitive because pattern has uppercase

# To match all variants:
rg -i 'todo'      # case-insensitive
```

## Multiline Matching — Agents Struggle With This

By default, rg treats each line as independent. To match patterns spanning multiple lines:

```bash
rg -U 'foo\nbar'              # --multiline: enables \n in patterns
rg -U --multiline-dotall 'func.*return nil\n}'  # . also matches \n
```

### ⚠️ PITFALL: `\n` in pattern without `-U` matches literally or silently fails

```bash
# WRONG — \n is treated as literal backslash-n, matches nothing
rg 'func .*\{.*\n.*return nil'

# RIGHT — use -U for multiline
rg -U 'func [^{]+\{[^}]*\n[^}]*return nil' -t go
```

Complex multiline example — find Go functions ending in `return nil`:
```bash
rg -U 'func.*\{[\s\S]*?return nil\n\}' -t go
```

## Output Control

```bash
rg -l 'pattern'              # --files-with-matches: filenames only
rg -c 'pattern'              # --count: match count per file
rg -n 'pattern'              # --line-number: show line numbers (default on terminal)
rg --no-heading 'pattern'    # don't group output by filename
rg --json 'pattern'          # structured JSON (for scripting/piping)
rg -o 'pattern'              # --only-matching: print matched text only
rg -A 3 'pattern'            # 3 lines after each match
rg -B 3 'pattern'            # 3 lines before each match
rg -C 3 'pattern'            # 3 lines before and after each match
```

## Fixed String Mode

Use `-F` when your pattern contains regex metacharacters:

```bash
rg -F 'array[0]'             # matches literal "array[0]", not regex char class
rg -F 'err != nil'           # no need to escape anything
rg -F '(err)'                # literal parens
rg -F 'map[string]string'    # Go type literal
```

## Regex Syntax Notes

rg uses Rust's `regex` crate — same engine as `sd`. Key differences from PCRE/grep:

- **No lookahead/lookbehind** without `-P` flag: `(?=...)`, `(?<=...)` don't work by default
- **Word boundaries**: `\b` works — `rg '\bfoo\b'` matches `foo` but not `foobar`
- **Non-capturing groups**: `(?:...)` works
- **PCRE2 features**: add `-P` / `--pcre2` flag to enable lookarounds

```bash
rg '\bfoo\b'                 # word boundary (works without -P)
rg -P '(?<=prefix)suffix'   # lookbehind (requires -P)
rg -P '(?=.*foo).*bar'      # lookahead (requires -P)
```

## Common Pitfalls

| Pitfall | Wrong | Right | Why |
|---------|-------|-------|-----|
| grep include flag | `rg --include '*.go'` | `rg -t go` or `rg -g '*.go'` | `--include` doesn't exist in rg |
| multiline without flag | `rg 'foo\nbar'` | `rg -U 'foo\nbar'` | `\n` is literal without `-U` |
| case sensitivity assumption | `rg 'TODO'` (expecting case-insensitive) | `rg -i 'todo'` | smart-case makes all-uppercase case-sensitive |
| unescaped regex chars | `rg 'map[string]'` | `rg -F 'map[string]'` | `[string]` is a character class in regex |
| PCRE features without flag | `rg '(?<=foo)bar'` | `rg -P '(?<=foo)bar'` | lookarounds require `-P` |
| multiline with dot | `rg -U 'func.*}'` | `rg -U --multiline-dotall 'func.*}'` | `.` doesn't match `\n` in multiline by default |

## Practical Recipes

```bash
# Find all TODOs in Go source (not tests)
rg 'TODO|FIXME|HACK' -t go -T go-test

# List files containing a pattern (for piping to other tools)
rg -l 'deprecated' -t go

# Count occurrences per file
rg -c 'err :=' -t go | sort -t: -k2 -rn | head -20

# Find Go functions named something specific
rg '^func \w*Handler\w*\(' -t go

# Search for literal Go type (fixed string)
rg -F 'map[string]interface{}' -t go

# Find multiline pattern: if block without error check
rg -U 'if err != nil \{[^}]*\}' -t go -l
```
