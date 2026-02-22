# ast-grep Reference

Binary: `ast-grep` (alias `sg` if configured)
Version: 0.40.x

AST-based structural code search, lint, and rewrite using tree-sitter grammars. Matches syntax structure, not text — won't accidentally rename inside strings, comments, or unrelated identifiers.

## Core Commands

```bash
# Search for pattern
ast-grep run --pattern 'PATTERN' --lang LANG [PATHS...]

# Search and rewrite
ast-grep run --pattern 'PATTERN' --rewrite 'REPLACEMENT' --lang LANG [PATHS...]

# Interactive rewrite (confirm each change)
ast-grep run --pattern 'PATTERN' --rewrite 'REPLACEMENT' --lang LANG --interactive

# Apply all rewrites without confirmation
ast-grep run --pattern 'PATTERN' --rewrite 'REPLACEMENT' --lang LANG --update-all

# Scan with rule file
ast-grep scan --rule rule.yml [PATHS...]

# JSON output for scripting
ast-grep run --pattern 'PATTERN' --lang LANG --json
```

Language is inferred from file extensions when `--lang` is omitted and paths are provided.

## Metavariable Syntax

| Syntax | Matches | Example |
|--------|---------|---------|
| `$VAR` | Single AST node | `$FUNC($A)` matches `foo(x)` |
| `$_` | Single node (anonymous, non-capturing) | `$_($A)` matches any call with one arg |
| `$$$` | Zero or more nodes (anonymous) | `foo($$$)` matches `foo()`, `foo(a)`, `foo(a, b, c)` |
| `$$$ARGS` | Zero or more nodes (named) | `foo($$$ARGS)` captures all args |
| `$$VAR` | Single node including unnamed tree-sitter nodes | `return $$A` matches `return 123` and `return;` |

**Key behavior:** Same named metavariable used twice must match identical content. `$A == $A` matches `x == x` but not `x == y`.

## Common Patterns

### Find all calls to a function
```bash
ast-grep run -p 'fmt.Errorf($$$)' -l go
```

### Rename a function
```bash
ast-grep run -p 'oldName($$$ARGS)' -r 'newName($$$ARGS)' -l go -U
```

### Find method calls on a type
```bash
ast-grep run -p '$OBJ.Close()' -l go
```

### Match specific argument patterns
```bash
# Find assert calls where expected is nil
ast-grep run -p 'assert.Equal($T, nil, $$$)' -l go
```

### Swap function arguments
```bash
ast-grep run -p 'assertEqual($EXPECTED, $ACTUAL)' -r 'assertEqual($ACTUAL, $EXPECTED)' -l python -U
```

### Find unused error returns
```bash
ast-grep run -p '$_, _ = $FUNC($$$)' -l go
```

### Rewrite error handling
```bash
ast-grep run -p 'errors.New($MSG)' -r 'fmt.Errorf($MSG)' -l go -U
```

## Output Options

```bash
--json              # Structured JSON output (for piping)
--json=pretty       # Pretty-printed JSON
--json=stream       # Streaming JSON (one match per line)
-A NUM              # Show NUM lines after match
-B NUM              # Show NUM lines before match
-C NUM              # Show NUM context lines around match
--heading always    # Always show filename heading
--color never       # Disable color (for piping)
```

## File Targeting

```bash
ast-grep run -p 'PATTERN' -l go                    # all Go files in cwd
ast-grep run -p 'PATTERN' -l go src/ internal/      # specific directories
ast-grep run -p 'PATTERN' --globs '!*_test.go' -l go  # exclude test files
ast-grep run -p 'PATTERN' --globs '**/*.tsx' -l tsx    # glob filter
```

## Strictness Levels

Control how precisely the pattern must match the AST:

| Level | Behavior |
|-------|----------|
| `cst` | Exact match including all trivial nodes |
| `smart` (default) | Match all except trivial source nodes |
| `ast` | Match only named AST nodes |
| `relaxed` | Like `ast` but ignores comments |
| `signature` | Like `relaxed` but ignores text content |

```bash
ast-grep run -p 'PATTERN' --strictness relaxed -l go
```

## Rule Files (YAML)

For complex matching with constraints, use rule files:

```yaml
# rule.yml
id: no-console-log
language: typescript
rule:
  pattern: console.log($$$)
message: Remove console.log before committing
severity: warning
```

```bash
ast-grep scan --rule rule.yml
```

Rules support `constraints`, `has`, `inside`, `follows`, `precedes`, and boolean combinators (`all`, `any`, `not`).

## Debugging Patterns

```bash
# Print the AST of a pattern to understand matching
ast-grep run -p 'your_pattern' --debug-query=ast -l go

# Print CST (includes unnamed nodes)
ast-grep run -p 'your_pattern' --debug-query=cst -l go
```

## Supported Languages

Go, TypeScript, JavaScript, Python, Rust, C, C++, Java, Kotlin, Swift, Ruby, Lua, and many more. Full list: `ast-grep run --help` or https://ast-grep.github.io/reference/languages.html

## When to Use ast-grep vs Alternatives

- **ast-grep** when you need syntax-aware precision (renames, finding specific call patterns)
- **comby** when you need multi-line structural rewrites with balanced delimiters
- **sd/rg** when you're doing simple literal string operations
