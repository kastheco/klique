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

typos uses `_typos.toml`, `typos.toml`, or `.typos.toml` in the project root:

```toml
# Ignore specific words globally
[default.extend-words]
nto = "nto"        # not a typo in this project
ques = "ques"

# Ignore specific identifiers
[default.extend-identifiers]
MyTypo = "MyTypo"

# Per-file-type configuration
[type.rust.extend-words]
ser = "ser"        # serialization abbreviation

# Exclude paths
[files]
extend-exclude = ["vendor/", "*.generated.go"]
```

## Common Workflow

```bash
# 1. Check what typos exist
typos

# 2. Preview fixes
typos --diff

# 3. Auto-fix
typos --write-changes

# 4. Review remaining (may need manual config for false positives)
typos
```

## Pre-commit Integration

Run `typos` before commits to catch typos early. Pair with `--format brief` for CI output.

## When to Use typos vs Alternatives

- **typos**: Automated spell checking of code — fast, understands identifiers
- **Manual review**: For domain-specific terms that typos doesn't know
- **codespell**: Alternative spell checker (Python-based, slower)
