# Store Plans in DB Implementation Plan

**Goal:** Eliminate `docs/plans/` and `plan-state.json` entirely by storing plan content + metadata in the SQLite database, with the TUI auto-managing an embedded HTTP server so the DB becomes the single source of truth.

**Architecture:** The TUI embeds the `kas serve` HTTP server as a goroutine (no separate process). Plan markdown content moves into a new `content` column on the `plans` table. All plan reads/writes go through the `planstore.Store` interface. The `.signals/` directory moves to `.kasmos/signals/` (project-local, gitignored) since it's ephemeral IPC, not persistent state. The `planstate` package's JSON fallback path is removed — the store is always available because it's embedded. Agents still write plan `.md` files to the worktree; the TUI ingests their content into the DB when processing sentinel signals.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), net/http, bubbletea, planstore package, planfsm package

**Size:** Large (estimated ~8 hours, 8 tasks, 4 waves)

---

## Wave 1: Schema + Store API Extensions

### Task 1: Add Content Column to SQLite Schema and Store Interface

**Files:**
- Modify: `config/planstore/store.go`
- Modify: `config/planstore/sqlite.go`
- Test: `config/planstore/sqlite_test.go`

**Step 1: write the failing test**

```go
func TestSQLiteStore_CreateWithContent(t *testing.T) {
    store := newTestStore(t)
    entry := PlanEntry{
        Filename: "2026-02-28-test.md",
        Status:   StatusReady,
        Content:  "# Test Plan\n\n## Wave 1\n\n### Task 1: Do thing\n",
    }
    require.NoError(t, store.Create("proj", entry))
    got, err := store.Get("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, entry.Content, got.Content)
}

func TestSQLiteStore_GetContent(t *testing.T) {
    store := newTestStore(t)
    entry := PlanEntry{
        Filename: "2026-02-28-test.md",
        Status:   StatusReady,
        Content:  "# Full Plan Content",
    }
    require.NoError(t, store.Create("proj", entry))
    content, err := store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, "# Full Plan Content", content)
}

func TestSQLiteStore_SetContent(t *testing.T) {
    store := newTestStore(t)
    entry := PlanEntry{Filename: "2026-02-28-test.md", Status: StatusReady}
    require.NoError(t, store.Create("proj", entry))
    require.NoError(t, store.SetContent("proj", "2026-02-28-test.md", "# Updated"))
    content, err := store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, "# Updated", content)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestSQLiteStore_CreateWithContent -v
go test ./config/planstore/... -run TestSQLiteStore_GetContent -v
go test ./config/planstore/... -run TestSQLiteStore_SetContent -v
```

expected: FAIL — `Content` field undefined, `GetContent`/`SetContent` methods undefined

**Step 3: write minimal implementation**

1. Add `Content string` field to `PlanEntry` in `store.go`.
2. Add `GetContent(project, filename string) (string, error)` and `SetContent(project, filename, content string) error` to the `Store` interface.
3. Add schema migration in `sqlite.go`: `ALTER TABLE plans ADD COLUMN content TEXT NOT NULL DEFAULT ''` (run after initial `CREATE TABLE`, guarded by column-exists check so existing DBs are upgraded).
4. Update `Create`, `Get`, `Update` to include the `content` column.
5. Implement `GetContent` (SELECT content WHERE project=? AND filename=?) and `SetContent` (UPDATE plans SET content=? WHERE project=? AND filename=?).
6. Update `scanPlanEntry` and `scanPlanEntries` to scan the content column.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/store.go config/planstore/sqlite.go config/planstore/sqlite_test.go
git commit -m "feat(planstore): add content column to store interface and SQLite schema"
```

### Task 2: Add Content Methods to HTTP Client and Server

**Files:**
- Modify: `config/planstore/http.go`
- Modify: `config/planstore/server.go`
- Modify: `config/planstore/http_test.go`
- Modify: `config/planstore/server_test.go`

**Step 1: write the failing test**

```go
func TestHTTPStore_ContentRoundTrip(t *testing.T) {
    store := newTestHTTPStore(t)
    entry := PlanEntry{
        Filename: "2026-02-28-test.md",
        Status:   StatusReady,
        Content:  "# My Plan\n\nDetails here.",
    }
    require.NoError(t, store.Create("proj", entry))

    content, err := store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, "# My Plan\n\nDetails here.", content)

    require.NoError(t, store.SetContent("proj", "2026-02-28-test.md", "# Updated Plan"))
    content, err = store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, "# Updated Plan", content)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestHTTPStore_ContentRoundTrip -v
```

expected: FAIL — `GetContent`/`SetContent` not implemented on `HTTPStore`

**Step 3: write minimal implementation**

1. Add two new HTTP endpoints to `server.go`:
   - `GET /v1/projects/{project}/plans/{filename}/content` → returns raw markdown text (Content-Type: text/markdown)
   - `PUT /v1/projects/{project}/plans/{filename}/content` → accepts raw body, calls `store.SetContent()`
2. Implement `GetContent` on `HTTPStore`: GET the content endpoint, read body as string.
3. Implement `SetContent` on `HTTPStore`: PUT with raw body to the content endpoint.
4. Include `Content` in the existing `Create`/`Get`/`Update` JSON payloads (backward compatible — empty string is the zero value).

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/http.go config/planstore/server.go config/planstore/http_test.go config/planstore/server_test.go
git commit -m "feat(planstore): add content endpoints to HTTP client and server"
```

### Task 3: Add Content Methods to planstate

**Files:**
- Modify: `config/planstate/planstate.go`
- Test: `config/planstate/planstate_test.go`

**Step 1: write the failing test**

```go
func TestPlanState_CreateWithContent(t *testing.T) {
    store := planstore.NewTestStore(t) // uses in-memory SQLite
    ps, err := LoadWithStore(store, "proj", t.TempDir())
    require.NoError(t, err)

    content := "# Auth Refactor\n\n## Wave 1\n"
    err = ps.CreateWithContent("2026-02-28-auth.md", "auth refactor", "plan/auth", "", time.Now(), content)
    require.NoError(t, err)

    got, err := store.GetContent("proj", "2026-02-28-auth.md")
    require.NoError(t, err)
    assert.Equal(t, content, got)
}

func TestPlanState_GetContent(t *testing.T) {
    store := planstore.NewTestStore(t)
    ps, err := LoadWithStore(store, "proj", t.TempDir())
    require.NoError(t, err)

    content := "# Plan Content"
    require.NoError(t, ps.CreateWithContent("test.md", "", "", "", time.Now(), content))

    got, err := ps.GetContent("test.md")
    require.NoError(t, err)
    assert.Equal(t, content, got)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstate/... -run TestPlanState_CreateWithContent -v
go test ./config/planstate/... -run TestPlanState_GetContent -v
```

expected: FAIL — methods undefined

**Step 3: write minimal implementation**

1. Add `CreateWithContent(filename, description, branch, topic string, createdAt time.Time, content string) error` to `PlanState`. Creates the plan entry with content set, then calls `store.SetContent()` to persist the markdown body.
2. Add `GetContent(filename string) (string, error)` to `PlanState`. Delegates to `store.GetContent()`.
3. Add `SetContent(filename, content string) error` to `PlanState`. Delegates to `store.SetContent()`.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstate/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstate/planstate.go config/planstate/planstate_test.go
git commit -m "feat(planstate): add content storage methods delegating to planstore"
```

## Wave 2: Embedded Server

> **depends on wave 1:** The content column and store API extensions must exist before the embedded server can serve content endpoints.

### Task 4: Embed HTTP Server in TUI Lifecycle

**Files:**
- Create: `config/planstore/embedded.go`
- Test: `config/planstore/embedded_test.go`
- Modify: `app/app.go`

**Step 1: write the failing test**

```go
func TestEmbeddedServer_StartsAndStops(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    srv, err := StartEmbedded(dbPath, 0) // port 0 = auto-assign
    require.NoError(t, err)
    defer srv.Stop()

    assert.NotEmpty(t, srv.URL())

    // Verify the server is reachable
    client := NewHTTPStore(srv.URL(), "test")
    require.NoError(t, client.Ping())
}

func TestEmbeddedServer_StopIsIdempotent(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    srv, err := StartEmbedded(dbPath, 0)
    require.NoError(t, err)
    srv.Stop()
    srv.Stop() // should not panic
}

func TestEmbeddedServer_ContentEndpoint(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    srv, err := StartEmbedded(dbPath, 0)
    require.NoError(t, err)
    defer srv.Stop()

    client := NewHTTPStore(srv.URL(), "test")
    require.NoError(t, client.Create("proj", PlanEntry{
        Filename: "test.md", Status: StatusReady, Content: "# Hello",
    }))

    content, err := client.GetContent("proj", "test.md")
    require.NoError(t, err)
    assert.Equal(t, "# Hello", content)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestEmbeddedServer -v
```

expected: FAIL — `StartEmbedded` undefined

**Step 3: write minimal implementation**

Create `config/planstore/embedded.go`:

```go
// EmbeddedServer wraps an HTTP server + SQLite store that runs in-process.
// The TUI starts this on boot and stops it on exit.
type EmbeddedServer struct {
    store    *SQLiteStore
    server   *http.Server
    url      string
    stopped  sync.Once
}

// StartEmbedded opens the SQLite DB, creates the HTTP handler, and starts
// listening on 127.0.0.1:port. Use port 0 for auto-assignment.
// Returns the running server and its base URL.
func StartEmbedded(dbPath string, port int) (*EmbeddedServer, error) { ... }

// URL returns the base URL (e.g. "http://127.0.0.1:7433").
func (s *EmbeddedServer) URL() string { return s.url }

// Store returns the underlying SQLite store for direct access (audit log, etc.).
func (s *EmbeddedServer) Store() *SQLiteStore { return s.store }

// Stop gracefully shuts down the HTTP server and closes the DB.
func (s *EmbeddedServer) Stop() { ... }
```

Then modify `app/app.go` `newHome()`:
1. Always start an embedded server using `planstore.StartEmbedded(planstore.ResolvedDBPath(), 0)`.
2. Create the `HTTPStore` client pointing at the embedded server's URL.
3. Store the `*EmbeddedServer` on `home` struct so `Run()` can call `Stop()` in its defer.
4. If `appConfig.PlanStore` is set (remote URL), use that URL for the HTTP client instead of the embedded server's URL. The embedded server still starts for audit log DB access.
5. Remove the `planStore == nil` fallback paths — the store is always available.
6. The `kas serve` CLI command remains for standalone/remote use cases (e.g. multi-machine access over tailscale).

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestEmbeddedServer -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/embedded.go config/planstore/embedded_test.go app/app.go
git commit -m "feat(planstore): embed HTTP server in TUI lifecycle"
```

## Wave 3: Remove JSON Fallback + Read from DB

> **depends on wave 2:** The embedded server must be running before we can remove the JSON fallback — all code paths need a guaranteed store.

### Task 5: Remove JSON Fallback from planstate and planfsm

**Files:**
- Modify: `config/planstate/planstate.go`
- Modify: `config/planfsm/fsm.go`
- Modify: `config/planfsm/lock.go`
- Modify: `config/planfsm/lock_windows.go`
- Modify: `config/planstate/planstate_test.go`
- Modify: `config/planfsm/fsm_test.go`

**Step 1: write the failing test**

```go
func TestPlanState_LoadRequiresStore(t *testing.T) {
    store := planstore.NewTestStore(t)
    require.NoError(t, store.Create("proj", planstore.PlanEntry{
        Filename: "test.md", Status: planstore.StatusReady,
    }))

    // Load should work with a store and no plan-state.json on disk
    ps, err := Load(store, "proj", t.TempDir())
    require.NoError(t, err)
    assert.Len(t, ps.Plans, 1)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstate/... -run TestPlanState_LoadRequiresStore -v
```

expected: FAIL — `Load` signature doesn't accept a store

**Step 3: write minimal implementation**

1. **`planstate.go`**: Change `Load(dir string)` to `Load(store planstore.Store, project, dir string)`. This is now the only load path — it always reads from the store. Remove `LoadWithStore` (merge into `Load`). Remove the JSON file reading path entirely. Remove `save()` method's JSON writing branch. Remove `wrappedFormat`, `stateFile` const, and all JSON marshal/unmarshal code. Remove `reconcileFilenames` (DB is canonical, no filesystem filenames to reconcile). The `dir` parameter is retained solely for `.signals/` path derivation.
2. **`fsm.go`**: Remove the `withLock` call path from `Transition()`. Always use the store-backed path. Simplify `New()` to always require a store. Remove the `dir`-only constructor.
3. **`lock.go` / `lock_windows.go`**: Delete the flock implementation — no longer needed since SQLite handles concurrency.
4. Update all callers of `Load()` and `LoadWithStore()` across the codebase to use the new `Load(store, project, dir)` signature.
5. Update all tests to use the store-backed path.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstate/... -v
go test ./config/planfsm/... -v
go test ./app/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstate/planstate.go config/planfsm/fsm.go config/planfsm/lock.go config/planfsm/lock_windows.go config/planstate/planstate_test.go config/planfsm/fsm_test.go
git commit -m "refactor(planstate): remove JSON fallback, store is always required"
```

### Task 6: Read Plan Content from DB Instead of Filesystem

**Files:**
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Test: `app/app_plan_actions_test.go`

**Step 1: write the failing test**

```go
func TestViewSelectedPlan_ReadsFromStore(t *testing.T) {
    store := planstore.NewTestStore(t)
    content := "# My Plan\n\n## Wave 1\n\n### Task 1: Do thing\n"
    require.NoError(t, store.Create("proj", planstore.PlanEntry{
        Filename: "2026-02-28-test.md",
        Status:   planstore.StatusReady,
        Content:  content,
    }))

    ps, err := planstate.Load(store, "proj", t.TempDir())
    require.NoError(t, err)

    h := &home{
        planState:        ps,
        planStore:        store,
        planStoreProject: "proj",
    }

    // Verify content is retrievable from store (not filesystem)
    got, err := store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, content, got)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestViewSelectedPlan_ReadsFromStore -v
```

expected: FAIL — viewSelectedPlan still reads from filesystem

**Step 3: write minimal implementation**

1. **`viewSelectedPlan`** in `app_state.go`: Replace `os.ReadFile(planPath)` with `m.planStore.GetContent(m.planStoreProject, planFile)`. Remove the `planPath` filesystem join.
2. **`rebuildOrphanedOrchestrators`**: Replace `os.ReadFile(filepath.Join(m.planStateDir, planFile))` with `m.planStore.GetContent(m.planStoreProject, planFile)`.
3. **`finalizePlanCreation`**: Replace `os.WriteFile(planPath, ...)` with `m.planState.SetContent(planFile, content)`. Remove `os.MkdirAll(m.planStateDir, ...)`. Keep the git commit step but write a temp file for git to commit (or skip committing the plan file since it's in the DB now).
4. **`importClickUpTask`** in `app_state.go`: Write scaffold content to store instead of filesystem.
5. **Wave orchestration plan parsing** in `app.go` metadata tick: Replace `os.ReadFile` with `m.planStore.GetContent()`.
6. **Agent prompts** in `app_state.go`: For agents that need filesystem access to the plan file, write a temporary copy to the worktree's `docs/plans/` on demand before spawning the agent. The TUI writes the file from DB content; agents read it from the worktree.

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_state.go app/app_actions.go app/app_plan_actions_test.go
git commit -m "refactor(app): read plan content from DB instead of filesystem"
```

## Wave 4: Agent Integration + Migration + Cleanup

> **depends on wave 3:** Agents need the content API and DB-backed reads working before we can migrate signals and add the JSON migration script.

### Task 7: Migrate Signals Directory and Ingest Plan Content on Completion

**Files:**
- Modify: `config/planfsm/signals.go`
- Modify: `config/planfsm/wave_signal.go`
- Modify: `app/app_state.go`
- Modify: `internal/initcmd/scaffold/scaffold.go`
- Modify: `contracts/planner_prompt_contract_test.go`
- Test: `config/planfsm/signals_test.go`

**Step 1: write the failing test**

```go
func TestScanSignals_KasmosSignalsDir(t *testing.T) {
    signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
    require.NoError(t, os.MkdirAll(signalsDir, 0o755))
    require.NoError(t, os.WriteFile(
        filepath.Join(signalsDir, "planner-finished-test.md"),
        []byte(""), 0o644,
    ))

    signals := ScanSignals(signalsDir)
    require.Len(t, signals, 1)
    assert.Equal(t, PlannerFinished, signals[0].Event)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planfsm/... -run TestScanSignals_KasmosSignalsDir -v
```

expected: PASS (ScanSignals already takes a dir parameter — this validates the new convention works)

**Step 3: write minimal implementation**

1. **Signals directory**: Change the signals directory from `docs/plans/.signals/` to `.kasmos/signals/` (project-local, already gitignored via `.kasmos/`). Update all callers in `app.go` that pass `planStateDir` to `ScanSignals`/`ScanWaveSignals` to use the new `signalsDir` path instead.
2. **Plan content ingestion**: When the TUI processes a `PlannerFinished` signal, read the plan `.md` file from the agent's worktree (using the plan's branch worktree path) and store its content in the DB via `store.SetContent()`. This bridges the gap: agents write files to their worktree, TUI ingests them into the DB.
3. **Agent prompts**: Update `buildWaveAnnotationPrompt` to reference `.kasmos/signals/` instead of `docs/plans/.signals/`. Update `buildPlanPrompt` and `buildImplementPrompt` to tell agents the plan file path in the worktree (agents still write to `docs/plans/` in their worktree for compatibility — the TUI ingests on completion).
4. **Scaffold**: Remove `docs/plans` and `docs/plans/.signals` from the scaffold directory list in `scaffold.go`. Add `.kasmos/signals` instead.
5. **Update contract tests** to reference the new signals path.

**Step 4: run test to verify it passes**

```bash
go test ./config/planfsm/... -v
go test ./contracts/... -v
go test ./internal/initcmd/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planfsm/signals.go config/planfsm/wave_signal.go app/app_state.go internal/initcmd/scaffold/scaffold.go contracts/planner_prompt_contract_test.go config/planfsm/signals_test.go
git commit -m "refactor: migrate signals to .kasmos/signals/ and ingest plan content on planner-finished"
```

### Task 8: Migration Script + Gitignore docs/plans/

**Files:**
- Create: `config/planstore/migrate.go`
- Test: `config/planstore/migrate_test.go`
- Modify: `.gitignore`

**Step 1: write the failing test**

```go
func TestMigrateFromJSON(t *testing.T) {
    store := newTestStore(t)
    plansDir := t.TempDir()

    stateJSON := `{
        "plans": {
            "2026-02-28-test.md": {
                "status": "ready",
                "description": "test plan",
                "branch": "plan/test"
            }
        },
        "topics": {
            "tools": {"created_at": "2026-02-28T00:00:00Z"}
        }
    }`
    require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(stateJSON), 0o644))
    require.NoError(t, os.WriteFile(filepath.Join(plansDir, "2026-02-28-test.md"), []byte("# Test Plan"), 0o644))

    migrated, err := MigrateFromJSON(store, "proj", plansDir)
    require.NoError(t, err)
    assert.Equal(t, 1, migrated)

    entry, err := store.Get("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, StatusReady, entry.Status)
    assert.Equal(t, "test plan", entry.Description)

    content, err := store.GetContent("proj", "2026-02-28-test.md")
    require.NoError(t, err)
    assert.Equal(t, "# Test Plan", content)

    topics, err := store.ListTopics("proj")
    require.NoError(t, err)
    assert.Len(t, topics, 1)
}

func TestMigrateFromJSON_Idempotent(t *testing.T) {
    store := newTestStore(t)
    plansDir := t.TempDir()

    stateJSON := `{"plans": {"test.md": {"status": "done"}}}`
    require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(stateJSON), 0o644))

    _, err := MigrateFromJSON(store, "proj", plansDir)
    require.NoError(t, err)

    _, err = MigrateFromJSON(store, "proj", plansDir)
    require.NoError(t, err) // second run should not error
}

func TestMigrateFromJSON_NoFile(t *testing.T) {
    store := newTestStore(t)
    migrated, err := MigrateFromJSON(store, "proj", t.TempDir())
    require.NoError(t, err)
    assert.Equal(t, 0, migrated)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestMigrateFromJSON -v
```

expected: FAIL — `MigrateFromJSON` undefined

**Step 3: write minimal implementation**

1. Create `config/planstore/migrate.go` with `MigrateFromJSON(store Store, project, plansDir string) (int, error)`:
   - Read `plan-state.json` from `plansDir` (if missing, return 0, nil).
   - Parse the wrapped format (plans + topics).
   - For each plan entry, call `store.Create()` (skip if already exists — idempotent).
   - For each `.md` file on disk matching a plan entry, read its content and call `store.SetContent()`.
   - For each topic, call `store.CreateTopic()` (skip if already exists).
   - Return count of migrated plans.
2. Call `MigrateFromJSON` in `newHome()` after the embedded server starts, before loading plan state. Only run if `plan-state.json` exists on disk (one-time migration).
3. After successful migration, rename `plan-state.json` to `plan-state.json.migrated` to prevent re-migration and log the migration count.
4. Add `docs/plans/plan-state.json` and `docs/plans/.plan-state.lock` to `.gitignore`. Keep the plan `.md` files tracked for now (they serve as a git history reference) but they are no longer read by the app.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestMigrateFromJSON -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/migrate.go config/planstore/migrate_test.go .gitignore
git commit -m "feat(planstore): add JSON-to-DB migration and gitignore plan-state.json"
```
