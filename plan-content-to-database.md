# Migrate Plan Content to Database Implementation Plan

**Goal:** Eliminate remaining disk reads of plan `.md` files in the TUI app layer, making the task store database the single source of truth for plan content. Three production callsites in `app/` still read plan content from `docs/plans/` on disk instead of the database — migrate them to use `m.taskStore.GetContent()`.

**Architecture:** The task store (SQLite via `config/taskstore`) already has a `content` column and `GetContent`/`SetContent` methods. Most callsites already use the DB — the preview renderer, wave signal handler, and orphaned orchestrator rebuilder all call `m.taskStore.GetContent()`. Three callsites remain on disk: the "implement" action, `validatePlanHasWaves`, and the "solo" action's disk-existence check. Additionally, `kas task register` (CLI) creates entries without ingesting content. This plan migrates all remaining disk reads and adds content ingestion to the register flow.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), config/taskstore, config/taskstate, config/taskparser, testify

**Size:** Small (estimated ~1.5 hours, 2 tasks, 1 wave)

---

## Wave 1: Migrate Disk Reads to Database

### Task 1: Migrate App Layer Plan Reads from Disk to Database

**Files:**
- Modify: `app/app_actions.go`
- Modify: `app/app_wave_validation_test.go`
- Test: `app/app_task_actions_test.go`

**Step 1: write the failing test**

Add a test in `app/app_task_actions_test.go` that verifies the "implement" action reads plan content from the task store, not from disk. The test should:
- Create a task entry in the store with content containing valid wave headers
- NOT write any `.md` file to disk in `docs/plans/`
- Trigger the implement action and verify it succeeds (proving it read from DB, not disk)

Also add a test for the "solo" action that verifies it checks for content in the store rather than checking disk file existence.

```go
func TestImplementActionReadsFromStore(t *testing.T) {
    // Setup: create task entry with content in store, no file on disk
    // Trigger implement action
    // Assert: orchestrator was created successfully (content was read from DB)
}

func TestSoloActionChecksStoreNotDisk(t *testing.T) {
    // Setup: create task entry with content in store, no file on disk
    // Trigger solo action
    // Assert: prompt includes plan file reference (content was found in DB)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestImplementActionReadsFromStore -v
go test ./app/... -run TestSoloActionChecksStoreNotDisk -v
```

expected: FAIL — the implement action reads from disk (finds no file, returns error); the solo action checks `os.Stat` on disk

**Step 3: write minimal implementation**

**3a. Migrate "implement" action** in `app/app_actions.go` (around line 837-856):

Replace the disk read:
```go
plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
content, err := os.ReadFile(filepath.Join(plansDir, planFile))
```
with a database read:
```go
content, err := m.taskStore.GetContent(m.taskStoreProject, planFile)
```

This matches the pattern already used at `app/app.go:1013` for wave signal processing.

**3b. Migrate "solo" action** in `app/app_actions.go` (around line 828-834):

Replace the `os.Stat` disk check:
```go
planPath := filepath.Join(m.activeRepoPath, "docs", "plans", planFile)
refFile := ""
if _, err := os.Stat(planPath); err == nil {
    refFile = planFile
}
```
with a store content check:
```go
refFile := ""
if m.taskStore != nil {
    if c, err := m.taskStore.GetContent(m.taskStoreProject, planFile); err == nil && c != "" {
        refFile = planFile
    }
}
```

**3c. Replace `validatePlanHasWaves`** in `app/app_actions.go` (line 893-902):

The function currently reads from disk. It is only called from `app/app_wave_validation_test.go`. Replace it with a content-accepting variant:

```go
func validatePlanContent(content string) error {
    _, err := taskparser.Parse(content)
    return err
}
```

Update `app/app_wave_validation_test.go` to pass content strings directly to `validatePlanContent` instead of writing temp files and calling `validatePlanHasWaves`.

**3d. Clean up dead imports** — remove `os` and `filepath` imports if no longer needed in the modified sections. (`os` may still be needed elsewhere in the file — check before removing.)

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run "TestImplementActionReadsFromStore|TestSoloActionChecksStoreNotDisk|TestValidatePlan" -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_actions.go app/app_wave_validation_test.go app/app_task_actions_test.go
git commit -m "feat: migrate implement/solo actions and wave validation from disk to database"
```

### Task 2: Ingest Content During CLI Registration

**Files:**
- Modify: `cmd/task.go`
- Test: `cmd/task_test.go`

**Step 1: write the failing test**

Add a test in `cmd/task_test.go` that verifies `executeTaskRegister` ingests the `.md` file content into the store's content field after creating the entry:

```go
func TestExecuteTaskRegisterIngestsContent(t *testing.T) {
    // Setup: create a temp plansDir, write a .md file with known content
    store, _ := taskstore.NewSQLiteStore(":memory:")
    defer store.Close()

    planContent := "# My Plan\n\n## Wave 1\n\n### Task 1: Do something\n\nDo it.\n"
    planFile := "my-plan.md"
    os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644)

    err := executeTaskRegister(plansDir, planFile, "", "", "", store)
    require.NoError(t, err)

    // Verify content was ingested
    got, err := store.GetContent(projectFromPlansDir(plansDir), planFile)
    require.NoError(t, err)
    assert.Equal(t, planContent, got)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestExecuteTaskRegisterIngestsContent -v
```

expected: FAIL — current `executeTaskRegister` calls `ps.Create()` without ingesting content

**Step 3: write minimal implementation**

Modify `executeTaskRegister` in `cmd/task.go` to always read the file content and use `CreateWithContent` instead of `Create`:

```go
func executeTaskRegister(plansDir, planFile, branch, topic, description string, store taskstore.Store) error {
    fullPath := filepath.Join(plansDir, planFile)
    data, err := os.ReadFile(fullPath)
    if err != nil {
        return fmt.Errorf("task file not found on disk: %s", fullPath)
    }
    ps, err := loadTaskState(plansDir, store)
    if err != nil {
        return err
    }
    if description == "" {
        description = strings.TrimSuffix(planFile, ".md")
        for _, line := range strings.Split(string(data), "\n") {
            if strings.HasPrefix(line, "# ") {
                description = strings.TrimPrefix(line, "# ")
                break
            }
        }
    }
    if branch == "" {
        slug := strings.TrimSuffix(planFile, ".md")
        branch = "plan/" + slug
    }
    info, _ := os.Stat(fullPath)
    createdAt := info.ModTime()
    return ps.CreateWithContent(planFile, description, branch, topic, createdAt, string(data))
}
```

The key change: replace `ps.Create(...)` with `ps.CreateWithContent(...)` which stores both the metadata and the file content in the database in a single operation. The `CreateWithContent` method already exists in `config/taskstate/taskstate.go` (line 328).

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run TestExecuteTaskRegisterIngestsContent -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/task.go cmd/task_test.go
git commit -m "feat: ingest plan file content into database during kas task register"
```
