# typos Reference

Binary: `typos`
Version: 1.43.x (typos-cli)

Fast source code spell checker. Understands programming conventions — camelCase, snake_case, identifiers, filenames. Finds typos in code, strings, comments, and file paths.

## Basic Usage

```bash
typos                          # check entire project from cwd
typos src/                     # check specific directory
typos path/to/file.go          # check single file
typos --write-changes          # auto-fix all typos
typos --write-changes src/     # auto-fix in specific path
```

## Key Flags

| Flag | Effect |
|------|--------|
| `--write-changes` / `-w` | Fix typos in-place |
| `--diff` | Show diff of what would change |
| `--format FORMAT` | Output format: `brief`, `long`, `json` |
| `--exclude GLOB` | Exclude files matching glob |
| `--force-exclude GLOB` | Force exclude (overrides includes) |
| `--hidden` | Check hidden files/directories |
| `--no-check-filenames` | Skip filename checking |
| `--no-check-files` | Skip file content checking |
| `--no-unicode` | Skip unicode homoglyph detection |

## Output Formats

```bash
typos --format brief           # compact: file:line:col: typo -> fix
typos --format long            # verbose with context
typos --format json            # machine-readable JSON
```

## Configuration

typos uses `_typos.toml`, `typos.toml`, or `.typos.toml` in the project root. The most common need is suppressing false positives for domain-specific terms.

```toml
# Suppress false positives: map "detected-typo" -> "intended-word"
# If both are the same, typos treats it as intentional and skips it.
[default.extend-words]
nto = "nto"        # project-specific abbreviation, not "not"
ques = "ques"      # "queues" abbreviation used as-is
ser = "ser"        # serialization shorthand

# Suppress false positives for identifiers (camelCase, PascalCase tokens)
[default.extend-identifiers]
MyCustomIdent = "MyCustomIdent"   # legacy identifier, not a typo
FooBarBaz = "FooBarBaz"

# Per-language configuration (applies only to that file type)
[type.rust.extend-words]
ser = "ser"        # Rust serialization convention

[type.go.extend-words]
init = "init"      # Go init functions

# Exclude paths entirely
[files]
extend-exclude = ["vendor/", "*.generated.go", "testdata/"]
```

### Adding a False Positive Exclusion

When `typos` flags something that is not actually a typo:

1. Run `typos` to identify flagged words
2. Add an entry to `[default.extend-words]` mapping the flagged word to itself
3. Re-run `typos` to confirm the false positive is suppressed

Example: if typos flags `auth` as a typo for `auth` (it won't, but as a pattern):
```toml
[default.extend-words]
auth = "auth"
```

## Common Workflow

```bash
# 1. Check what typos exist
typos

# 2. Preview fixes
typos --diff

# 3. Auto-fix
typos --write-changes

# 4. For remaining false positives, add to typos.toml then re-run
typos
```

## Pre-commit Integration

Run `typos` before commits to catch typos early. Pair with `--format brief` for CI output.
