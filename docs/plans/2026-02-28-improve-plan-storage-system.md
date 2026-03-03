# Improve Plan Storage System

**Goal:** Replace the git-tracked `plan-state.json` with a SQLite-backed HTTP server (`kas serve`) so plan state is a single network-accessible source of truth — eliminating merge conflicts, worktree races, and cross-machine desync.

**Architecture:** A new `planstore` package provides a `Store` interface with two implementations: `HTTPStore` (client that talks to the server) and `SQLiteStore` (direct DB access used by the server). The server is a lightweight `net/http` handler embedded in `kas serve`. The existing `planstate` package is refactored to delegate to the `Store` interface. The TUI, CLI, and FSM all continue using `planstate` — they don't know the backend changed. Config gains a `plan_store` field in TOML (`plan_store = "http://athena:7433"`) that selects the backend. When unconfigured or unreachable, operations fail gracefully with a toast/error — no local fallback.

**Tech Stack:** Go 1.24, `modernc.org/sqlite` (pure Go, no CGO), `net/http` stdlib, existing `config/planstate` + `config/planfsm` packages, `cobra` for `kas serve` subcommand.

**Size:** Large (estimated ~8 hours, 8 tasks, 3 waves)

---

## Wave 1: Storage Backend

> Wave 1 builds the SQLite store, HTTP server, and HTTP client as standalone packages with no coupling to the existing TUI. Everything is testable in isolation.

### Task 1: SQLite Store Implementation

**Files:**
- Create: `config/planstore/store.go`
- Create: `config/planstore/sqlite.go`
- Create: `config/planstore/sqlite_test.go`

**Step 1: write the failing test**

Define the `Store` interface and write tests against it using a SQLite implementation. The interface mirrors the existing `planstate.PlanState` method set but operates on a project-scoped basis:

```go
// store.go
type Store interface {
    // Plan CRUD
    Create(project string, entry PlanEntry) error
    Get(project, filename string) (PlanEntry, error)
    Update(project, filename string, entry PlanEntry) error
    Rename(project, oldFilename, newFilename string) error

    // Queries
    List(project string) ([]PlanEntry, error)
    ListByStatus(project string, statuses ...Status) ([]PlanEntry, error)
    ListByTopic(project, topic string) ([]PlanEntry, error)

    // Topics
    ListTopics(project string) ([]TopicEntry, error)
    CreateTopic(project string, entry TopicEntry) error

    // Health
    Ping() error
}
```

```go
// sqlite_test.go
func TestSQLiteStore_CreateAndGet(t *testing.T) {
    store := newTestStore(t)
    entry := PlanEntry{
        Filename: "2026-02-28-test-plan.md",
        Status:   StatusReady,
        Description: "test plan",
        Branch: "plan/test-plan",
        CreatedAt: time.Now().UTC(),
    }
    require.NoError(t, store.Create("kasmos", entry))

    got, err := store.Get("kasmos", "2026-02-28-test-plan.md")
    require.NoError(t, err)
    assert.Equal(t, StatusReady, got.Status)
    assert.Equal(t, "test plan", got.Description)
}

func TestSQLiteStore_ListByStatus(t *testing.T) {
    store := newTestStore(t)
    store.Create("kasmos", PlanEntry{Filename: "a.md", Status: StatusReady})
    store.Create("kasmos", PlanEntry{Filename: "b.md", Status: StatusDone})
    store.Create("kasmos", PlanEntry{Filename: "c.md", Status: StatusReady})

    plans, err := store.ListByStatus("kasmos", StatusReady)
    require.NoError(t, err)
    assert.Len(t, plans, 2)
}

func TestSQLiteStore_ProjectIsolation(t *testing.T) {
    store := newTestStore(t)
    store.Create("project-a", PlanEntry{Filename: "x.md", Status: StatusReady})
    store.Create("project-b", PlanEntry{Filename: "y.md", Status: StatusReady})

    plans, err := store.List("project-a")
    require.NoError(t, err)
    assert.Len(t, plans, 1)
    assert.Equal(t, "x.md", plans[0].Filename)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestSQLiteStore -v
```

expected: FAIL — package doesn't exist yet

**Step 3: write minimal implementation**

Implement `SQLiteStore` using `modernc.org/sqlite` via `database/sql`:

- Schema: `plans` table with columns `(id INTEGER PRIMARY KEY, project TEXT, filename TEXT, status TEXT, description TEXT, branch TEXT, topic TEXT, created_at TEXT, implemented TEXT, UNIQUE(project, filename))` and `topics` table with `(id INTEGER PRIMARY KEY, project TEXT, name TEXT, created_at TEXT, UNIQUE(project, name))`.
- `NewSQLiteStore(dbPath string)` opens/creates the DB, runs migrations via `CREATE TABLE IF NOT EXISTS`.
- All methods use parameterized queries. `List` sorts by filename. `ListByStatus` accepts variadic statuses.
- `newTestStore(t)` helper creates an in-memory SQLite DB (`":memory:"`).
- Reuse the existing `Status` type constants from `planstate` — import them or redefine identical constants in `planstore` to avoid circular imports. Prefer redefining to keep `planstore` self-contained.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestSQLiteStore -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/store.go config/planstore/sqlite.go config/planstore/sqlite_test.go go.mod go.sum
git commit -m "feat(planstore): add Store interface and SQLite implementation"
```

### Task 2: HTTP Server Handler

**Files:**
- Create: `config/planstore/server.go`
- Create: `config/planstore/server_test.go`

**Step 1: write the failing test**

Test the HTTP handler using `httptest.NewServer` backed by a real SQLite store:

```go
func TestServer_CreateAndGetPlan(t *testing.T) {
    store := newTestStore(t)
    srv := httptest.NewServer(NewHandler(store))
    defer srv.Close()

    body := `{"filename":"test.md","status":"ready","description":"test"}`
    resp, err := http.Post(srv.URL+"/v1/projects/kasmos/plans", "application/json", strings.NewReader(body))
    require.NoError(t, err)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)

    resp, err = http.Get(srv.URL + "/v1/projects/kasmos/plans/test.md")
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)

    var got PlanEntry
    json.NewDecoder(resp.Body).Decode(&got)
    assert.Equal(t, StatusReady, got.Status)
}

func TestServer_ListByStatus(t *testing.T) {
    store := newTestStore(t)
    srv := httptest.NewServer(NewHandler(store))
    defer srv.Close()

    // Create plans with different statuses
    for _, p := range []PlanEntry{
        {Filename: "a.md", Status: StatusReady},
        {Filename: "b.md", Status: StatusDone},
    } {
        store.Create("kasmos", p)
    }

    resp, err := http.Get(srv.URL + "/v1/projects/kasmos/plans?status=ready")
    require.NoError(t, err)
    var plans []PlanEntry
    json.NewDecoder(resp.Body).Decode(&plans)
    assert.Len(t, plans, 1)
}

func TestServer_Ping(t *testing.T) {
    store := newTestStore(t)
    srv := httptest.NewServer(NewHandler(store))
    defer srv.Close()

    resp, err := http.Get(srv.URL + "/v1/ping")
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestServer -v
```

expected: FAIL — `NewHandler` undefined

**Step 3: write minimal implementation**

Implement `NewHandler(store Store) http.Handler` using stdlib `net/http` with a `http.ServeMux`:

- Routes:
  - `GET /v1/ping` → `store.Ping()`, returns 200
  - `POST /v1/projects/{project}/plans` → `store.Create()`, returns 201
  - `GET /v1/projects/{project}/plans` → `store.List()` (with optional `?status=` filter), returns 200
  - `GET /v1/projects/{project}/plans/{filename}` → `store.Get()`, returns 200 or 404
  - `PUT /v1/projects/{project}/plans/{filename}` → `store.Update()`, returns 200
  - `POST /v1/projects/{project}/plans/{filename}/rename` → `store.Rename()`, returns 200
  - `GET /v1/projects/{project}/topics` → `store.ListTopics()`, returns 200
  - `POST /v1/projects/{project}/topics` → `store.CreateTopic()`, returns 201
- JSON request/response bodies. Errors return `{"error": "message"}` with appropriate status codes.
- Use Go 1.22+ `http.ServeMux` pattern matching (`GET /v1/projects/{project}/plans/{filename}`).

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestServer -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/server.go config/planstore/server_test.go
git commit -m "feat(planstore): add HTTP server handler for plan store API"
```

### Task 3: HTTP Client Implementation

**Files:**
- Create: `config/planstore/http.go`
- Create: `config/planstore/http_test.go`

**Step 1: write the failing test**

Test the HTTP client against a real `httptest.NewServer` to verify the full round-trip:

```go
func TestHTTPStore_RoundTrip(t *testing.T) {
    backend := newTestStore(t)
    srv := httptest.NewServer(NewHandler(backend))
    defer srv.Close()

    client := NewHTTPStore(srv.URL, "kasmos")

    // Create
    entry := PlanEntry{Filename: "test.md", Status: StatusReady, Description: "test"}
    require.NoError(t, client.Create("kasmos", entry))

    // Get
    got, err := client.Get("kasmos", "test.md")
    require.NoError(t, err)
    assert.Equal(t, "test", got.Description)

    // Update
    got.Status = StatusImplementing
    require.NoError(t, client.Update("kasmos", "test.md", got))

    // List
    plans, err := client.List("kasmos")
    require.NoError(t, err)
    assert.Len(t, plans, 1)
    assert.Equal(t, StatusImplementing, plans[0].Status)
}

func TestHTTPStore_ServerUnreachable(t *testing.T) {
    client := NewHTTPStore("http://127.0.0.1:1", "kasmos")
    _, err := client.List("kasmos")
    require.Error(t, err)
    // Error should be recognizable as a connectivity issue
    assert.Contains(t, err.Error(), "plan store unreachable")
}

func TestHTTPStore_Ping(t *testing.T) {
    backend := newTestStore(t)
    srv := httptest.NewServer(NewHandler(backend))
    defer srv.Close()

    client := NewHTTPStore(srv.URL, "kasmos")
    require.NoError(t, client.Ping())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestHTTPStore -v
```

expected: FAIL — `NewHTTPStore` undefined

**Step 3: write minimal implementation**

Implement `HTTPStore` struct:

- `NewHTTPStore(baseURL, project string) *HTTPStore` — creates client with a `http.Client` (5s timeout).
- All `Store` interface methods make HTTP requests to the server, marshal/unmarshal JSON.
- Connection errors are wrapped with `"plan store unreachable: %w"` so callers can detect and surface gracefully.
- `Ping()` calls `GET /v1/ping` with a 2s timeout.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestHTTPStore -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/http.go config/planstore/http_test.go
git commit -m "feat(planstore): add HTTP client implementing Store interface"
```

## Wave 2: Server Command and Config Integration

> **depends on wave 1:** The `kas serve` command needs the SQLite store and HTTP handler from wave 1. Config integration needs the `Store` interface to select the backend.

### Task 4: `kas serve` Cobra Command

**Files:**
- Create: `cmd/serve.go`
- Create: `cmd/serve_test.go`
- Modify: `cmd/cmd.go`

**Step 1: write the failing test**

```go
func TestServeCmd_Exists(t *testing.T) {
    rootCmd := NewRootCmd()
    // Verify the serve subcommand is registered
    cmd, _, err := rootCmd.Find([]string{"serve"})
    require.NoError(t, err)
    assert.Equal(t, "serve", cmd.Name())
}

func TestServeCmd_DefaultPort(t *testing.T) {
    cmd := NewServeCmd()
    assert.Contains(t, cmd.UseLine(), "serve")
    // Verify default flag values
    port, _ := cmd.Flags().GetInt("port")
    assert.Equal(t, 7433, port)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestServeCmd -v
```

expected: FAIL — `NewServeCmd` undefined

**Step 3: write minimal implementation**

Create `cmd/serve.go`:

- `NewServeCmd() *cobra.Command` — `kas serve` with flags:
  - `--port` (default 7433)
  - `--db` (default `~/.config/kasmos/planstore.db`)
  - `--bind` (default `0.0.0.0`)
- `RunE` opens `SQLiteStore`, wraps in `NewHandler`, starts `http.Server` with graceful shutdown on SIGINT/SIGTERM.
- Prints startup banner: `plan store listening on http://0.0.0.0:7433 (db: ~/.config/kasmos/planstore.db)`.
- Register in `cmd.go`'s root command via `rootCmd.AddCommand(NewServeCmd())`.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run TestServeCmd -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/serve.go cmd/serve_test.go cmd/cmd.go
git commit -m "feat(cmd): add 'kas serve' command for plan store server"
```

### Task 5: TOML Config + Store Factory

**Files:**
- Modify: `config/toml.go`
- Modify: `config/config.go`
- Create: `config/planstore/factory.go`
- Create: `config/planstore/factory_test.go`

**Step 1: write the failing test**

```go
// factory_test.go
func TestNewStoreFromConfig_HTTP(t *testing.T) {
    backend := newTestStore(t)
    srv := httptest.NewServer(NewHandler(backend))
    defer srv.Close()

    store, err := NewStoreFromConfig(srv.URL, "test-project")
    require.NoError(t, err)
    require.NoError(t, store.Ping())
}

func TestNewStoreFromConfig_Empty(t *testing.T) {
    store, err := NewStoreFromConfig("", "test-project")
    require.NoError(t, err)
    // Returns nil store — caller should fall back to legacy behavior
    assert.Nil(t, store)
}

func TestNewStoreFromConfig_Unreachable(t *testing.T) {
    store, err := NewStoreFromConfig("http://127.0.0.1:1", "test-project")
    // Factory succeeds (lazy connect) but Ping fails
    require.NoError(t, err)
    require.Error(t, store.Ping())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestNewStoreFromConfig -v
```

expected: FAIL — `NewStoreFromConfig` undefined

**Step 3: write minimal implementation**

1. Add `PlanStore string` field to `TOMLConfig`:
   ```toml
   plan_store = "http://athena:7433"
   ```
   And to `Config` struct: `PlanStore string`.

2. Create `config/planstore/factory.go`:
   ```go
   func NewStoreFromConfig(planStoreURL, project string) (Store, error) {
       if planStoreURL == "" {
           return nil, nil // no remote store configured
       }
       return NewHTTPStore(planStoreURL, project), nil
   }
   ```

3. Wire the TOML field through `LoadTOMLConfig` → `Config` so it's available at app init time.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestNewStoreFromConfig -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/toml.go config/config.go config/planstore/factory.go config/planstore/factory_test.go
git commit -m "feat(config): add plan_store TOML config and store factory"
```

### Task 6: Adapt `planstate` to Use Store Backend

**Files:**
- Modify: `config/planstate/planstate.go`
- Modify: `config/planstate/planstate_test.go`

**Step 1: write the failing test**

```go
func TestPlanState_WithRemoteStore(t *testing.T) {
    backend := planstore.NewTestSQLiteStore(t)
    srv := httptest.NewServer(planstore.NewHandler(backend))
    defer srv.Close()

    store := planstore.NewHTTPStore(srv.URL, "test-project")

    // Create via store
    require.NoError(t, store.Create("test-project", planstore.PlanEntry{
        Filename: "test.md", Status: "ready", Description: "remote plan",
    }))

    // Load PlanState with remote store
    ps, err := LoadWithStore(store, "test-project", "/tmp/unused")
    require.NoError(t, err)
    assert.Len(t, ps.Plans, 1)
    assert.Equal(t, StatusReady, ps.Plans["test.md"].Status)
}

func TestPlanState_FallbackToJSON(t *testing.T) {
    // When store is nil, Load() works exactly as before (JSON file)
    dir := t.TempDir()
    ps, err := Load(dir)
    require.NoError(t, err)
    assert.NotNil(t, ps)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstate/... -run TestPlanState_With -v
```

expected: FAIL — `LoadWithStore` undefined

**Step 3: write minimal implementation**

Add `LoadWithStore(store planstore.Store, project, dir string) (*PlanState, error)`:

- When `store != nil`: populate `PlanState.Plans` and `PlanState.TopicEntries` from the remote store. Set `PlanState.store` (new private field) so subsequent mutations (`Create`, `Save`, `SetBranch`, `ForceSetStatus`, `Rename`) write through to the remote store instead of JSON.
- When `store == nil`: existing `Load(dir)` behavior is unchanged (reads `plan-state.json`).
- The `save()` method checks `ps.store != nil` — if set, writes to remote; otherwise writes JSON.
- All existing public methods (`Create`, `Register`, `SetBranch`, `ForceSetStatus`, `Rename`, `Save`) are updated to write through to the store when available. Read methods (`List`, `Unfinished`, `Finished`, etc.) continue operating on the in-memory map — the map is populated from the store on load.
- Error wrapping: store errors are wrapped with `"plan store: %w"` for graceful handling upstream.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstate/... -v
```

expected: PASS (all existing tests still pass, new tests pass)

**Step 5: commit**

```bash
git add config/planstate/planstate.go config/planstate/planstate_test.go
git commit -m "feat(planstate): add remote store backend with JSON fallback"
```

## Wave 3: TUI + FSM Integration

> **depends on wave 2:** The TUI needs the adapted `planstate` with store support and the config field to select the backend.

### Task 7: Wire Store into TUI App Initialization

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Modify: `config/planfsm/fsm.go`
- Modify: `config/planfsm/fsm_test.go`

**Step 1: write the failing test**

```go
// fsm_test.go — add test for store-backed transitions
func TestFSM_TransitionWithStore(t *testing.T) {
    backend := planstore.NewTestSQLiteStore(t)
    srv := httptest.NewServer(planstore.NewHandler(backend))
    defer srv.Close()

    store := planstore.NewHTTPStore(srv.URL, "test-project")
    store.Create("test-project", planstore.PlanEntry{
        Filename: "test.md", Status: "ready",
    })

    fsm := NewWithStore(store, "test-project", t.TempDir())
    require.NoError(t, fsm.Transition("test.md", PlanStart))

    // Verify the store was updated
    entry, err := store.Get("test-project", "test.md")
    require.NoError(t, err)
    assert.Equal(t, "planning", string(entry.Status))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planfsm/... -run TestFSM_TransitionWithStore -v
```

expected: FAIL — `NewWithStore` undefined

**Step 3: write minimal implementation**

1. **`planfsm`**: Add `NewWithStore(store planstore.Store, project, dir string) *PlanStateMachine`. When store is set, `Transition()` loads via `planstate.LoadWithStore()` instead of `planstate.Load()`. The flock-based locking is skipped when using the remote store (the server handles concurrency via SQLite's own locking).

2. **`app/app.go`**: At initialization (`newHome`), check `appConfig.PlanStore`:
   - If set, create a store via `planstore.NewStoreFromConfig()`, call `store.Ping()`.
   - If ping fails, show a toast warning: `"plan store unreachable — changes won't persist"` and continue with `store = nil` (JSON fallback).
   - If ping succeeds, pass the store to `planstate.LoadWithStore()` and `planfsm.NewWithStore()`.
   - Derive `project` from `filepath.Base(activeRepoPath)`.

3. **`app/app_state.go`**: Update `loadPlanState()` to use `planstate.LoadWithStore()` when a store is available. Update `createPlanEntry()`, `createPlanRecord()`, and `finalizePlanCreation()` — these already go through `planstate` methods which now write-through to the store, so minimal changes needed. Add error handling that shows a toast on store errors instead of silently failing.

4. **`app/app.go` metadata tick**: The periodic `planstate.Load()` call in the metadata tick goroutine should use `LoadWithStore()` when a store is configured, keeping the in-memory state fresh from the server.

**Step 4: run test to verify it passes**

```bash
go test ./config/planfsm/... -v
go test ./app/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_state.go config/planfsm/fsm.go config/planfsm/fsm_test.go
git commit -m "feat(app): wire plan store into TUI initialization and FSM"
```

### Task 8: CLI Commands + Agent Signal Compatibility

**Files:**
- Modify: `cmd/plan.go`
- Modify: `cmd/plan_test.go`
- Modify: `config/planfsm/signals.go`

**Step 1: write the failing test**

```go
// plan_test.go — verify CLI commands work with store backend
func TestPlanList_WithStore(t *testing.T) {
    backend := planstore.NewTestSQLiteStore(t)
    srv := httptest.NewServer(planstore.NewHandler(backend))
    defer srv.Close()

    backend.Create("test-project", planstore.PlanEntry{
        Filename: "test.md", Status: "ready", Description: "test plan",
    })

    // executePlanList should work with store
    output := executePlanListWithStore(srv.URL, "test-project")
    assert.Contains(t, output, "test.md")
    assert.Contains(t, output, "ready")
}

// signals.go — verify signals still trigger store-backed transitions
func TestSignals_WithStoreFSM(t *testing.T) {
    backend := planstore.NewTestSQLiteStore(t)
    srv := httptest.NewServer(planstore.NewHandler(backend))
    defer srv.Close()

    store := planstore.NewHTTPStore(srv.URL, "test-project")
    store.Create("test-project", planstore.PlanEntry{
        Filename: "test.md", Status: "planning",
    })

    // Write a sentinel file
    signalsDir := filepath.Join(t.TempDir(), ".signals")
    os.MkdirAll(signalsDir, 0o755)
    os.WriteFile(filepath.Join(signalsDir, "planner-finished-test.md"), nil, 0o644)

    signals := planfsm.ScanSignals(t.TempDir())
    require.Len(t, signals, 1)
    assert.Equal(t, planfsm.PlannerFinished, signals[0].Event)

    // Apply via store-backed FSM
    fsm := planfsm.NewWithStore(store, "test-project", t.TempDir())
    require.NoError(t, fsm.Transition("test.md", signals[0].Event))

    entry, _ := store.Get("test-project", "test.md")
    assert.Equal(t, "ready", string(entry.Status))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestPlanList_WithStore -v
go test ./config/planfsm/... -run TestSignals_WithStoreFSM -v
```

expected: FAIL — `executePlanListWithStore` undefined

**Step 3: write minimal implementation**

1. **`cmd/plan.go`**: Update CLI commands to check for `plan_store` config:
   - Load TOML config at command init. If `PlanStore` is set, create a store and use `planstate.LoadWithStore()` / `planfsm.NewWithStore()`.
   - Fallback to existing `planstate.Load(plansDir)` when unconfigured.
   - Add `executePlanListWithStore(storeURL, project string) string` helper.

2. **`config/planfsm/signals.go`**: No changes needed — sentinel files are a filesystem mechanism that the TUI's metadata tick already processes. The tick calls `fsm.Transition()` which now writes through to the store. The signal system is decoupled from storage — it just triggers FSM events.

3. **Verify backward compatibility**: When `plan_store` is empty/unset, all CLI commands work exactly as before against `plan-state.json`. The JSON file remains the default for projects that don't configure a remote store.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v
go test ./config/planfsm/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/plan.go cmd/plan_test.go config/planfsm/signals.go
git commit -m "feat(cmd): update CLI plan commands to support remote store backend"
```

---

## Migration Notes

- **No data migration required.** The JSON file remains the default. Users opt in to the server by adding `plan_store = "http://athena:7433"` to their TOML config and running `kas serve` on the target machine.
- **Existing `plan-state.json` can be imported** into the SQLite store via a one-time `kas plan import` command (deferred to a follow-up plan — manual SQL insert or a simple script suffices for now).
- **Agent compatibility**: Agents that touch `plan-state.json` directly (planner sentinel files) continue working — the TUI's metadata tick processes sentinels and writes through to the store. The sentinel file system is unchanged.
- **Systemd unit**: Users should create a simple systemd user service for `kas serve` on their server machine. Example:
  ```ini
  [Unit]
  Description=kasmos plan store
  After=network.target

  [Service]
  ExecStart=%h/go/bin/kas serve
  Restart=always

  [Install]
  WantedBy=default.target
  ```
