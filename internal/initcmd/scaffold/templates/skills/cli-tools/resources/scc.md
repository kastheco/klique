# scc Reference

Binary: `scc`
Version: 3.5.x

Fast code counter with complexity estimates. Counts lines of code, comments, blanks, and estimates cyclomatic complexity. Useful for project scope assessment, language breakdown, and effort estimation.

## Basic Usage

```bash
scc                            # full project summary from cwd
scc src/                       # specific directory
scc file.go                    # single file
```

## Key Flags

| Flag | Effect |
|------|--------|
| `--by-file` | Per-file breakdown instead of per-language summary |
| `--sort FIELD` | Sort by: `files`, `lines`, `blanks`, `code`, `comments`, `complexity`, `name` |
| `--include-ext EXT` | Only count specific extensions (comma-separated) |
| `--not-match REGEX` | Exclude files matching regex |
| `--format FMT` | Output format: `tabular` (default), `json`, `csv`, `html`, `wide` |
| `--no-complexity` | Skip complexity calculation (faster) |
| `--no-duplicates` | Skip duplicate file detection |
| `--min-gen` | Include generated files |
| `-z`, `--min-gen-line-length` | Min line length for generated file detection |

## Common Operations

### Project overview
```bash
scc
```

Output shows per-language: files, lines, blanks, comments, code, complexity.

### Per-file breakdown sorted by lines
```bash
scc --by-file --sort lines
```

### Specific languages only
```bash
scc --include-ext go,ts
```

### Exclude test files
```bash
scc --not-match '_test\.go$'
```

### Exclude vendor/generated
```bash
scc --not-match 'vendor/|generated|node_modules'
```

### JSON output for scripting
```bash
scc --format json
```

### Wide format (includes complexity per file)
```bash
scc --format wide --by-file
```

### Compare directories
```bash
scc dir1/
scc dir2/
# Compare output manually or use JSON format for programmatic comparison
```

## Output Columns

| Column | Meaning |
|--------|---------|
| Language | Detected programming language |
| Files | Number of files |
| Lines | Total lines (code + comments + blanks) |
| Blanks | Empty lines |
| Comments | Comment lines |
| Code | Code lines (lines - blanks - comments) |
| Complexity | Estimated cyclomatic complexity |

## When to Use scc vs Alternatives

- **scc**: Fast language-aware metrics with complexity. Best for project scope assessment.
- **wc -l**: BANNED. Use `scc` instead — even for single files.
- **cloc**: Legacy alternative (slower, Perl-based)
- **tokei**: Rust alternative (similar speed, different feature set)
