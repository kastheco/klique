# difftastic Reference

Binary: `difft`
Version: 0.67.x

Structural diff that understands syntax via tree-sitter. Compares code by AST structure, not line-by-line text. Produces dramatically cleaner output for refactors, moves, and formatting changes by ignoring irrelevant whitespace/formatting differences.

## Basic Usage

```bash
difft old.go new.go                    # compare two files
difft old_dir/ new_dir/                # compare directories
```

## Git Integration

```bash
# One-off diff
GIT_EXTERNAL_DIFF=difft git diff

# One-off log
GIT_EXTERNAL_DIFF=difft git log -p -- path/to/file.go

# One-off show
GIT_EXTERNAL_DIFF=difft git show HEAD

# Permanent config (adds to ~/.gitconfig)
git config --global diff.external difft
```

## Display Modes

```bash
difft --display side-by-side file1 file2      # default: two columns
difft --display side-by-side-show-both f1 f2  # always two columns even for pure adds/removes
difft --display inline file1 file2            # single column (like traditional diff)
difft --display json file1 file2              # machine-readable JSON
```

## Key Options

| Flag | Effect | Default |
|------|--------|---------|
| `--context N` | Context lines around changes | 3 |
| `--width N` | Column width for wrapping | terminal width |
| `--tab-width N` | Spaces per tab | 4 |
| `--color WHEN` | Color output: always/auto/never | auto |
| `--background dark\|light` | Optimize colors for background | dark |
| `--syntax-highlight on\|off` | Enable/disable highlighting | on |
| `--strip-cr` | Remove `\r` before diffing | off |
| `--skip-unchanged` | Only show changed files in dir diff | off |

## Environment Variables

All options can be set via environment variables prefixed with `DFT_`:

| Variable | Equivalent Flag |
|----------|----------------|
| `DFT_CONTEXT` | `--context` |
| `DFT_WIDTH` | `--width` |
| `DFT_DISPLAY` | `--display` |
| `DFT_COLOR` | `--color` |
| `DFT_BACKGROUND` | `--background` |
| `DFT_TAB_WIDTH` | `--tab-width` |
| `DFT_SYNTAX_HIGHLIGHT` | `--syntax-highlight` |

## Language Support

Difftastic supports 50+ languages via tree-sitter. Language is detected from file extensions. Override with `--language` flag.

## When to Use difft vs Alternatives

- **difft**: Reviewing refactors, renames, code moves — ignores irrelevant formatting noise
- **git diff**: When you need patch-compatible output or line-level granularity
- **diff**: Legacy fallback, no syntax awareness
