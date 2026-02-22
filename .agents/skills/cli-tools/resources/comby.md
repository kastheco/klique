# comby Reference

Binary: `comby`
Version: 1.8.1

Structural search/replace that understands balanced delimiters, strings, and comments. Superior to regex for code transformations, but has specific behaviors that cause **silent corruption if misused**.

## Core Flags

| Flag | Effect |
|------|--------|
| `-in-place` or `-i` | Modify files on disk (both work) |
| `-diff` | Show unified diff of changes |
| `-stdout` | Print result to stdout |
| `-stdin` | Read from stdin pipe |
| `-matcher .go` | Force language parser (auto-detects from extension) |
| `-match-only -newline-separated -stdout` | Extract matches without rewriting |

**Critical:** Without `-in-place`, comby only previews — no changes are written.

## File Targeting

```bash
comby 'pat' 'repl' path/to/file.go       # single file
comby 'pat' 'repl' .go                    # recursive from cwd, all .go files
comby 'pat' 'repl' .go -d src/            # recursive from specific directory
```

**Warning:** `-d /tmp/` scans ALL of /tmp including restricted dirs. Use full file paths instead.

## Hole Syntax

| Syntax | Matches | Behavior |
|--------|---------|----------|
| `:[var]` | Everything (lazy) | Inside delimiters: matches within balanced group incl. newlines. Outside: stops at newline or block start |
| `:[_]` | Everything (unnamed) | Same as `:[var]` but doesn't bind — can match different content each use |
| `:[[var]]` | `\w+` | Alphanumeric + underscore only |
| `:[var.]` | Non-space + punctuation | Good for dotted paths like `a.b.c` |
| `:[var:e]` | Expression | Contiguous non-whitespace OR balanced parens/brackets |
| `:[var\n]` | Line rest | Zero or more chars up to and including newline |
| `:[ var]` | Whitespace only | Spaces/tabs, NOT newlines |

### Variable Binding

- Same variable twice = must match identical content: `foo(:[a], :[a])` only matches `foo(x, x)`
- `:[_]` is special wildcard: `foo(:[_], :[_])` matches `foo(x, y)` — no binding

## THE Critical Rule: Balanced Delimiters

**`{:[body]}` is the ONLY safe way to match a block body.** Comby understands balanced `{}`, `()`, `[]` — the hole matches everything inside, preserving nesting.

### NEVER split braces across lines

```bash
# BROKEN — DO NOT USE
comby 'func A() {
:[body]
}' '...'
```

This puts `:[body]` OUTSIDE the delimiter pair. Effects:
1. Leading indentation of first line in body gets stripped
2. The `}` may match a NESTED `}` instead of the function's closing brace
3. Content between nested `}` and true closing `}` gets silently eaten

### ALWAYS use inline braces

```bash
# CORRECT — always use this form
comby 'func A() {:[body]}' '...'
```

## THE Critical Bug: Newline Collapse on Line Boundaries

When a match template starts with indented content (tab/spaces), comby's whitespace normalization treats `\n\t` in source as equivalent to `\t` in template. This merges lines:

```
Source:     }
            defaults.Harness = x

Template:   '	defaults.Harness = :[v]'
Result:     }	defaults.Harness = REPLACED   ← TWO LINES MERGED INTO ONE
```

### Workarounds

1. **Anchor to previous line:** Include the `}` (or whatever precedes) in your match template
2. **Use `:[_\n]` line hole:** `:[_\n]\tdefaults.Harness = :[v]` captures the boundary
3. **Match full surrounding block:** `if ... {:[_]}` + content after
4. **Best: match at function/block level** using `{:[body]}` and rewrite the whole body

## Whitespace Normalization Rules

- Template `\n` ≈ source `\n` ≈ source `\n\t` ≈ source `\n    ` (all treated as "some whitespace")
- Single space in template matches any amount of whitespace in source (including newlines)
- Blank lines in template DO match blank lines in source
- **Rewrite templates preserve literal newlines and indentation as written**

## Safe Patterns for Go

### Add parameter to function
```bash
comby 'func Foo(:[params]) :[ret] {:[body]}' \
     'func Foo(:[params], newParam Type) :[ret] {:[body]}' file.go -i
```

### Replace entire function body
```bash
comby 'func Foo(:[params]) :[ret] {:[_]}' \
     'func Foo(:[params]) :[ret] {
	// new body here
}' file.go -i
```

### Insert function after another
```bash
comby 'func Existing(:[p]) :[r] {:[body]}' \
     'func Existing(:[p]) :[r] {:[body]}

func NewFunc() {
	// ...
}' file.go -i
```

### Add to import block
```bash
comby 'import (:[imports])' 'import (:[imports]
	"new/package"
)' file.go -i
```

### Replace specific call pattern
```bash
comby 'oldFunc(:[args])' 'newFunc(:[args])' file.go -i
```

### Conditional rewrite with rules
```bash
comby 'foo(":[arg]")' 'bar(":[arg]")' file.go -i -rule 'where :[arg] == "specific"'
```

## Shell Quoting

- Single quotes preserve everything including newlines and tabs
- For Go strings with `\033`, shell won't interpret inside single quotes (correct)
- For apostrophes in comments: use `$'...'` with escaped `'\''` or double quotes

## Anti-Patterns

| Anti-Pattern | Why It Breaks |
|-------------|---------------|
| Split `{` and `}` on separate lines with `:[body]` between | Hole is outside delimiter pair — silent corruption |
| Match indented content at start of template without anchor | Newline collapse merges lines |
| Use `:[comment\n]` for multi-line comments | Matches far more than expected |
| Use `:[params]` in rewrite without it in match | Comby substitutes it literally as `:[params]` |
| `-d /tmp/` | Permission errors on systemd private dirs |
| Replace huge function bodies with literal text | Better to use `{:[_]}` to discard and write new body |
