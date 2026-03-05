# Move Default Config to Project-Local `.kasmos/` Implementation Plan

**Goal:** Centralize all kasmos config and state into the project-local `.kasmos/` directory (instead of `~/.config/kasmos/`) so that multiple OS users on the same repository (e.g. openfos via systemd) share config and state through the filesystem.

**Architecture:** `GetConfigDir()` changes from returning `~/.config/kasmos/` to returning `<cwd>/.kasmos/`. A one-time migration copies config files from the legacy XDG location on first use. `ResolvedDBPath()` delegates to `GetConfigDir()` instead of hardcoding. The `.gitignore` selectively un-ignores `config.toml` so project-wide agent settings are version-controlled while runtime artifacts remain ignored.

**Tech Stack:** Go 1.24+, testify, `t.Chdir()` for test CWD isolation

**Size:** Small (estimated ~1 hour, 2 tasks, 2 waves)

---

## Wave 1: Core config directory rewrite

### Task 1: Rewrite `GetConfigDir` to return project-local `.kasmos/` with legacy migration

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`
- Modify: `.gitignore`

**Step 1: write the failing test**

Replace the existing `TestGetConfigDir` tests in `config/config_test.go` with new tests that expect `.kasmos` as the config directory suffix. Use `t.Chdir(tempDir)` to control CWD. Key test cases:

```go
func TestGetConfigDir(t *testing.T) {
	t.Run("returns .kasmos relative to working directory", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		configDir, err := GetConfigDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tempDir, ".kasmos"), configDir)
	})

	t.Run("migrates config.toml from legacy XDG location", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		// Create legacy config at ~/.config/kasmos/
		legacyDir := filepath.Join(tempHome, ".config", "kasmos")
		require.NoError(t, os.MkdirAll(legacyDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(legacyDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = true\n"), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(projectDir, ".kasmos"), configDir)

		// Config should be copied to new location
		data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "animate_banner")

		// Legacy file should still exist (copy, not move)
		assert.FileExists(t, filepath.Join(legacyDir, "config.toml"))
	})

	t.Run("skips migration when config already exists in .kasmos", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		// Create config in both locations
		kasmosDir := filepath.Join(projectDir, ".kasmos")
		require.NoError(t, os.MkdirAll(kasmosDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(kasmosDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = false\n"), 0644))

		legacyDir := filepath.Join(tempHome, ".config", "kasmos")
		require.NoError(t, os.MkdirAll(legacyDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(legacyDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = true\n"), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)

		// Should use existing .kasmos config, NOT overwrite with legacy
		data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "animate_banner = false")
	})

	t.Run("no-ops when neither location has config", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Chdir(projectDir)
		t.Setenv("HOME", t.TempDir())

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(projectDir, ".kasmos"), configDir)
	})
}
```

Also update `TestLoadConfig` and `TestSaveConfig` to use `t.Chdir(tempDir)` instead of `t.Setenv("HOME", ...)` and assert paths under `.kasmos/` rather than `.config/kasmos/`.

**Step 2: run test to verify it fails**

```bash
go test ./config/... -run TestGetConfigDir -v
```

Expected: FAIL — tests expect `.kasmos` suffix but `GetConfigDir` still returns `.config/kasmos`.

**Step 3: write minimal implementation**

In `config/config.go`:

1. Rewrite `GetConfigDir()`:
   - Use `os.Getwd()` to get CWD
   - Return `filepath.Join(cwd, ".kasmos")`
   - Fast path: if `config.toml` or `config.json` exists in target, return immediately
   - Otherwise, attempt one-time migration from legacy XDG dirs (`~/.config/kasmos`, `~/.klique`, `~/.hivemind`)

2. Add `copyIfMissing(src, dst string)` helper:
   - Skip if `dst` already exists
   - Read `src`, write to `dst` with `0644` perms
   - Silently skip on any error (non-fatal)

3. The migration copies these files: `config.json`, `config.toml`, `state.json`, `taskstore.db`

4. Remove the old `os.Rename`-based migration logic (the legacy `.klique`/`.hivemind` rename is no longer appropriate — those dirs were under `$HOME` but we're now targeting `.kasmos/` under CWD).

Also update `.gitignore`: change the blanket `.kasmos/` rule to selective ignoring that allows `config.toml` to be tracked:
```
/.kasmos/*
!/.kasmos/config.toml
```

**Step 4: run test to verify it passes**

```bash
go test ./config/... -run "TestGetConfigDir|TestLoadConfig|TestSaveConfig" -v
```

Expected: PASS

**Step 5: commit**

```bash
git add config/config.go config/config_test.go .gitignore
git commit -m "refactor: move config dir from ~/.config/kasmos to project-local .kasmos/"
```

---

## Wave 2: Update dependent paths

> **depends on wave 1:** `ResolvedDBPath()` needs to call the rewritten `GetConfigDir()` to return the correct `.kasmos/taskstore.db` path.

### Task 2: Update `ResolvedDBPath` to use `GetConfigDir` and verify integration

**Files:**
- Modify: `config/taskstore/factory.go`
- Create: `config/taskstore/factory_test.go`

**Step 1: write the failing test**

Create `config/taskstore/factory_test.go`:

```go
func TestResolvedDBPath(t *testing.T) {
	t.Run("returns taskstore.db under .kasmos in working directory", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		dbPath := ResolvedDBPath()

		assert.Equal(t, filepath.Join(projectDir, ".kasmos", "taskstore.db"), dbPath)
	})
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run TestResolvedDBPath -v
```

Expected: FAIL — `ResolvedDBPath` still hardcodes `~/.config/kasmos/taskstore.db`.

**Step 3: write minimal implementation**

Rewrite `ResolvedDBPath()` in `config/taskstore/factory.go` to delegate to `config.GetConfigDir()`:

```go
import "github.com/kastheco/kasmos/config"

func ResolvedDBPath() string {
	dir, err := config.GetConfigDir()
	if err != nil {
		return filepath.Join(".", ".kasmos", "taskstore.db")
	}
	return filepath.Join(dir, "taskstore.db")
}
```

Update the doc comment to reflect the new behavior (returns `<cwd>/.kasmos/taskstore.db`).

**Step 4: run test to verify it passes**

```bash
go test ./config/taskstore/... -run TestResolvedDBPath -v
```

Expected: PASS

Then run the full test suite to verify no regressions:

```bash
go test ./... 2>&1 | tail -30
```

**Step 5: commit**

```bash
git add config/taskstore/factory.go config/taskstore/factory_test.go
git commit -m "refactor: update ResolvedDBPath to use project-local .kasmos/ config dir"
```
