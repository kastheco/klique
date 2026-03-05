# Session Context

## User Prompts

### Prompt 1

Implement move-default-config.md using the `kasmos-coder` skill. Retrieve the full plan with `kas task show move-default-config.md` and execute all tasks sequentially.

Reviewer feedback from previous round:
Round 1 — changes required.

## critical

- `internal/initcmd/initcmd_test.go:27,60-65` — `TestWritePhase` regression: test creates config dir at `filepath.Join(tmpHome, ".config", "kasmos")` and asserts `config.toml` exists there (line 60), but `GetConfigDir()` now returns `<cwd>/.kasmos...

