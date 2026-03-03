# Move Permission Cache to DB Implementation Plan

**Goal:** Replace the per-repo JSON file permission cache (`<repo>/.kasmos/permission-cache.json`) with a SQLite-backed store using the existing `planstore.db`, including automatic migration of existing cached permissions on first start.

**Architecture:** A new `permissions` table is added to the shared `~/.config/kasmos/planstore.db` SQLite database (alongside `plans`, `topics`, and `audit_events`). A `PermissionStore` interface mirrors the existing `PermissionCache` API (`IsAllowedAlways`, `Remember`) but backed by SQL. On app startup, if the legacy JSON file exists, its entries are imported into the DB and the file is removed. The `home` struct in `app.go` swaps from `*config.PermissionCache` to `config.PermissionStore`, and all callsites (`app_input.go`, `app.go` metadata tick) use the interface transparently.

**Tech Stack:** Go, `database/sql`, `modernc.org/sqlite` (already a dependency), `config` package

**Size:** Small (estimated ~1.5 hours, 3 tasks, 1 wave)

---

## Wave 1: Permission Store + Migration + Wiring

### Task 1: PermissionStore Interface and SQLite Implementation

**Files:**
- Create: `config/permission_store.go`
- Create: `config/permission_store_test.go`

**Step 1: write the failing test**

```go
func TestSQLitePermissionStore_RememberAndLookup(t *testing.T) {
    store, err := NewSQLitePermissionStore(":memory:")
    require.NoError(t, err)
    defer store.Close()

    assert.False(t, store.IsAllowedAlways("test-project", "/opt/*"))
    store.Remember("test-project", "/opt/*")
    assert.True(t, store.IsAllowedAlways("test-project", "/opt/*"))
    assert.False(t, store.IsAllowedAlways("test-project", "/tmp/*"))
    assert.False(t, store.IsAllowedAlways("other-project", "/opt/*"))
}
```

Also test: `Forget` (removes a single pattern), `ListPatterns` (returns all patterns for a project), `Close` idempotency, and that the schema creates the table on first open.

**Step 2: run test to verify it fails**

```bash
go test ./config/... -run TestSQLitePermissionStore -v
```

expected: FAIL — `NewSQLitePermissionStore undefined`

**Step 3: write minimal implementation**

Create `config/permission_store.go`:

- Define `PermissionStore` interface:
  ```go
  type PermissionStore interface {
      IsAllowedAlways(project, pattern string) bool
      Remember(project, pattern string)
      Forget(project, pattern string)
      ListPatterns(project string) []string
      Close() error
  }
  ```
- Define `SQLitePermissionStore` struct wrapping `*sql.DB`.
- Schema: `CREATE TABLE IF NOT EXISTS permissions (id INTEGER PRIMARY KEY, project TEXT NOT NULL, pattern TEXT NOT NULL, decision TEXT NOT NULL DEFAULT 'allow_always', created_at TEXT NOT NULL, UNIQUE(project, pattern))`.
- `NewSQLitePermissionStore(dbPath string)` opens DB, runs schema, returns store.
- `IsAllowedAlways` does `SELECT 1 FROM permissions WHERE project=? AND pattern=? AND decision='allow_always'`.
- `Remember` does `INSERT OR REPLACE INTO permissions (project, pattern, decision, created_at) VALUES (?, ?, 'allow_always', ?)`.
- `Forget` does `DELETE FROM permissions WHERE project=? AND pattern=?`.
- `ListPatterns` does `SELECT pattern FROM permissions WHERE project=? ORDER BY pattern`.

**Step 4: run test to verify it passes**

```bash
go test ./config/... -run TestSQLitePermissionStore -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/permission_store.go config/permission_store_test.go
git commit -m "feat: add PermissionStore interface and SQLite implementation"
```

### Task 2: Migration Function from JSON Cache

**Files:**
- Create: `config/permission_migrate.go`
- Create: `config/permission_migrate_test.go`

**Step 1: write the failing test**

```go
func TestMigratePermissionCache_ImportsAndRemovesFile(t *testing.T) {
    dir := t.TempDir()

    // Write a legacy permission-cache.json
    data := `{"/opt/*": "allow_always", "Execute bash command": "allow_always"}`
    require.NoError(t, os.WriteFile(filepath.Join(dir, "permission-cache.json"), []byte(data), 0644))

    store, err := NewSQLitePermissionStore(":memory:")
    require.NoError(t, err)
    defer store.Close()

    err = MigratePermissionCache(dir, "test-project", store)
    require.NoError(t, err)

    // Patterns should be in the DB
    assert.True(t, store.IsAllowedAlways("test-project", "/opt/*"))
    assert.True(t, store.IsAllowedAlways("test-project", "Execute bash command"))

    // JSON file should be removed
    _, err = os.Stat(filepath.Join(dir, "permission-cache.json"))
    assert.True(t, os.IsNotExist(err))
}

func TestMigratePermissionCache_NoFileIsNoop(t *testing.T) {
    store, err := NewSQLitePermissionStore(":memory:")
    require.NoError(t, err)
    defer store.Close()

    err = MigratePermissionCache(t.TempDir(), "test-project", store)
    assert.NoError(t, err) // missing file is not an error
}

func TestMigratePermissionCache_IdempotentOnRerun(t *testing.T) {
    dir := t.TempDir()
    data := `{"/opt/*": "allow_always"}`
    require.NoError(t, os.WriteFile(filepath.Join(dir, "permission-cache.json"), []byte(data), 0644))

    store, err := NewSQLitePermissionStore(":memory:")
    require.NoError(t, err)
    defer store.Close()

    // First migration
    require.NoError(t, MigratePermissionCache(dir, "test-project", store))
    // Second call (file gone) — should be a no-op
    require.NoError(t, MigratePermissionCache(dir, "test-project", store))

    assert.True(t, store.IsAllowedAlways("test-project", "/opt/*"))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/... -run TestMigratePermissionCache -v
```

expected: FAIL — `MigratePermissionCache undefined`

**Step 3: write minimal implementation**

Create `config/permission_migrate.go`:

- `MigratePermissionCache(cacheDir, project string, store PermissionStore) error`:
  1. Read `<cacheDir>/permission-cache.json`. If missing (`os.IsNotExist`), return nil.
  2. Unmarshal into `map[string]string`.
  3. For each entry where value == `"allow_always"`, call `store.Remember(project, key)`.
  4. Remove the JSON file with `os.Remove`.
  5. Return nil.

**Step 4: run test to verify it passes**

```bash
go test ./config/... -run TestMigratePermissionCache -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/permission_migrate.go config/permission_migrate_test.go
git commit -m "feat: add MigratePermissionCache to import legacy JSON into SQLite"
```

### Task 3: Wire PermissionStore into App and Update All Callsites

**Files:**
- Modify: `app/app.go` (home struct field, newHome init, metadata tick auto-approve)
- Modify: `app/app_input.go` (permission confirm handler)
- Modify: `app/app_permission_test.go` (test helpers and assertions)
- Delete: `config/permission_cache.go`
- Delete: `config/permission_cache_test.go`

**Step 1: write the failing test**

Update `newTestHomeWithCache` in `app_permission_test.go` to use `SQLitePermissionStore` with `:memory:` instead of `NewPermissionCache(t.TempDir())`. All existing tests should still compile and pass with the new interface.

```go
func newTestHomeWithCache(t *testing.T) *home {
    t.Helper()
    permStore, err := config.NewSQLitePermissionStore(":memory:")
    require.NoError(t, err)
    t.Cleanup(func() { permStore.Close() })

    spin := spinner.New(spinner.WithSpinner(spinner.Dot))
    return &home{
        ctx:               context.Background(),
        state:             stateDefault,
        appConfig:         config.DefaultConfig(),
        nav:               ui.NewNavigationPanel(&spin),
        menu:              ui.NewMenu(),
        tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
        toastManager:      overlay.NewToastManager(&spin),
        activeRepoPath:    t.TempDir(),
        program:           "opencode",
        permissionStore:   permStore,
        permissionHandled: make(map[*session.Instance]string),
    }
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestUpdate_Permission -v
```

expected: FAIL — `m.permissionCache` references no longer compile (field renamed to `permissionStore`)

**Step 3: write minimal implementation**

**In `app/app.go`:**
- Rename struct field `permissionCache *config.PermissionCache` → `permissionStore config.PermissionStore`.
- In `newHome()` (~line 409-413), replace the `permCacheDir` / `NewPermissionCache` / `Load` block with:
  1. Open `NewSQLitePermissionStore(planstore.ResolvedDBPath())`.
  2. Call `config.MigratePermissionCache(permCacheDir, project, permStore)` for one-time migration.
  3. Assign `h.permissionStore = permStore`.
  4. Add cleanup: close the store when the app exits (add to existing defer chain or `Close` method).
- In the metadata tick handler (~line 978): change `m.permissionCache.IsAllowedAlways(cacheKey)` → `m.permissionStore.IsAllowedAlways(m.activeProject(), cacheKey)`. The `activeProject()` helper derives the project name from `activeRepoPath` using `filepath.Base`, matching how planstore does it.
- Keep the `nil` guard: `m.permissionStore != nil`.

**In `app/app_input.go`:**
- In the permission confirm handler (~line 712-714): change `m.permissionCache.Remember(cacheKey)` + `m.permissionCache.Save()` → `m.permissionStore.Remember(m.activeProject(), cacheKey)`. No explicit `Save()` needed — SQLite writes are immediate.

**In `app/app_permission_test.go`:**
- Update all `m.permissionCache.Remember(...)` → `m.permissionStore.Remember("test-project", ...)`.
- Update all `m.permissionCache.IsAllowedAlways(...)` → `m.permissionStore.IsAllowedAlways("test-project", ...)`.

**Remove dead code:**
- Delete `config/permission_cache.go` and `config/permission_cache_test.go` (fully replaced).

Note: `config.CacheKey()` is still needed — it's a pure helper that picks pattern vs description. Keep it in a surviving file (e.g. `permission_store.go`).

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v
go test ./config/... -v
```

expected: PASS (all existing permission tests pass with the new store)

**Step 5: commit**

```bash
git add app/app.go app/app_input.go app/app_permission_test.go config/permission_store.go
git rm config/permission_cache.go config/permission_cache_test.go
git commit -m "refactor: replace PermissionCache with PermissionStore across all callsites"
```
