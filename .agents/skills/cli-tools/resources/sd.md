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

> **Use `-f w` (word boundary) when your pattern is a substring of longer identifiers.** Without it, `sd 'foo' 'bar'` also replaces `foobar`, `prefoo`, etc. This is the most common source of over-broad replacements.

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

### Word-boundary matching (avoid replacing substrings)
```bash
sd -f w 'foo' 'bar' file.go          # replaces 'foo' but not 'foobar' or 'prefoo'
```

### Preview before applying
```bash
sd -p 'dangerous_pattern' 'safe_replacement' file.go
```

### Fixed string (no regex interpretation)
```bash
sd -F 'array[0]' 'array[idx]' file.go
```

### Case-insensitive replace
```bash
sd -f i 'todo' 'DONE' file.md
```

### Multi-line with dotall
```bash
sd -f s 'start.*?end' 'REPLACED' file.txt
```

### Stdin/stdout pipe
```bash
ast-grep run -p 'pattern' --json | sd 'old' 'new'
```
