# sd Reference

Binary: `sd`
Version: 1.0.x

Modern `sed` alternative for find-and-replace. Uses regex by default, modifies files in-place by default. No delimiter escaping needed — simpler syntax than sed for common operations.

## Basic Usage

```bash
sd 'FIND' 'REPLACE' [FILES...]       # replace in files (in-place)
sd 'FIND' 'REPLACE'                   # read from stdin, write to stdout
cat file | sd 'old' 'new'             # pipe mode
```

## Key Flags

| Flag | Effect |
|------|--------|
| `-p`, `--preview` | Dry run — show changes without writing |
| `-F`, `--fixed-strings` | Treat patterns as literal strings (no regex) |
| `-f FLAGS` | Regex flags: `c` case-sensitive, `i` case-insensitive, `m` multi-line, `s` dotall, `w` full words, `e` disable multi-line |
| `-n N`, `--max-replacements N` | Limit replacements per file (0 = unlimited) |

## Regex Syntax

sd uses Rust's `regex` crate syntax (similar to PCRE but not identical):

```bash
# Capture groups with $1, $2, etc.
sd 'version = "(\d+)\.(\d+)"' 'version = "$1.99"' Cargo.toml

# Named groups
sd '(?P<major>\d+)\.(?P<minor>\d+)' '$major.0' file.txt

# Character classes
sd '[A-Z]+_[A-Z]+' 'REPLACED' file.txt

# Non-greedy
sd 'func\(.*?\)' 'func()' file.go
```

## Common Operations

### Simple string replacement
```bash
sd 'oldName' 'newName' src/**/*.go
```

### Fixed string (no regex interpretation)
```bash
sd -F 'array[0]' 'array[idx]' file.go
```

### Case-insensitive replace
```bash
sd -f i 'todo' 'DONE' file.md
```

### Word-boundary matching
```bash
sd -f w 'foo' 'bar' file.go          # replaces 'foo' but not 'foobar'
```

### Multi-line with dotall
```bash
sd -f s 'start.*?end' 'REPLACED' file.txt
```

### Preview before applying
```bash
sd -p 'dangerous_pattern' 'safe_replacement' file.go
```

### Stdin/stdout pipe
```bash
ast-grep run -p 'pattern' --json | sd 'old' 'new'
```

## When to Use sd vs Alternatives

- **sd**: Simple string/regex replacements in files. Fastest path for non-structural changes.
- **ast-grep**: When replacement must be syntax-aware (renaming symbols without hitting strings/comments)
- **comby**: When replacement involves balanced delimiters or multi-line structural patterns
- **sed**: BANNED. Use `sd` instead — always.
