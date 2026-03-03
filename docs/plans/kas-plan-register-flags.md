# Add --topic and --description flags to kas plan register

**Goal:** Add `--topic` and `--description` flags to `kas plan register` so plans can be registered with topic grouping and custom descriptions from the CLI, eliminating the need for raw HTTP calls to the planstore API.

**Architecture:** `executePlanRegister` in `cmd/plan.go` currently calls `ps.Register()` (no topic support). Switch to `ps.Create()` which already handles topic + auto-creates topic entries. Add two cobra string flags and thread them through. No store or planstate changes needed.

**Tech Stack:** Go, cobra, planstate, planstore (SQLite + HTTP)

**Size:** Trivial (estimated ~20 min, 1 task, 1 wave)

---

## Wave 1: Add flags to register command

### Task 1: Add --topic and --description flags to executePlanRegister

**Files:**
- Modify: `cmd/plan.go`
- Test: `cmd/plan_test.go`

**Step 1: write the failing test**

Add two tests to `cmd/plan_test.go`:

```go
func TestPlanRegister(t *testing.T) {
	store, dir := setupTestPlanState(t)
	project := projectFromPlansDir(dir)

	// Write a plan file on disk
	planFile := "2026-03-02-new-feature.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, planFile),
		[]byte("# New Feature Plan\n\nSome content."),
		0o644,
	))

	err := executePlanRegister(dir, planFile, "", "", "", store)
	require.NoError(t, err)

	ps, err := planstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReady, entry.Status)
	assert.Equal(t, "New Feature Plan", entry.Description)
	assert.Equal(t, "plan/new-feature", entry.Branch)
	assert.Equal(t, "", entry.Topic)
}

func TestPlanRegister_WithTopicAndDescription(t *testing.T) {
	store, dir := setupTestPlanState(t)
	project := projectFromPlansDir(dir)

	planFile := "2026-03-02-stub-plan.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, planFile),
		[]byte("# Stub Plan\n"),
		0o644,
	))

	err := executePlanRegister(dir, planFile, "", "brain phase 1", "Implement circuit breaker", store)
	require.NoError(t, err)

	ps, err := planstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReady, entry.Status)
	assert.Equal(t, "Implement circuit breaker", entry.Description)
	assert.Equal(t, "brain phase 1", entry.Topic)

	// Topic should be auto-created
	topics := ps.Topics()
	topicNames := make([]string, len(topics))
	for i, ti := range topics {
		topicNames[i] = ti.Name
	}
	assert.Contains(t, topicNames, "brain phase 1")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run 'TestPlanRegister$|TestPlanRegister_WithTopicAndDescription' -v
```

expected: FAIL — `executePlanRegister` signature mismatch (too many arguments)

**Step 3: write minimal implementation**

In `cmd/plan.go`, change `executePlanRegister` signature and body to accept `topic` and `description` parameters, and switch from `ps.Register()` to `ps.Create()`:

```go
func executePlanRegister(plansDir, planFile, branch, topic, description string, store planstore.Store) error {
	fullPath := filepath.Join(plansDir, planFile)
	if _, err := os.Stat(fullPath); err != nil {
		return fmt.Errorf("plan file not found on disk: %s", fullPath)
	}
	ps, err := loadPlanState(plansDir, store)
	if err != nil {
		return err
	}
	// Extract description from first H1 line if not provided via flag.
	if description == "" {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		description = strings.TrimSuffix(planFile, ".md")
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# ") {
				description = strings.TrimPrefix(line, "# ")
				break
			}
		}
	}
	// Default branch: plan/<slug> where slug strips the date prefix.
	if branch == "" {
		slug := planFile
		if len(slug) > 11 && slug[4] == '-' && slug[7] == '-' && slug[10] == '-' {
			slug = slug[11:]
		}
		slug = strings.TrimSuffix(slug, ".md")
		branch = "plan/" + slug
	}
	info, _ := os.Stat(fullPath)
	createdAt := info.ModTime()
	return ps.Create(planFile, description, branch, topic, createdAt)
}
```

Then update the cobra command wiring in `NewPlanCmd` to declare and pass the two new flags:

```go
var branchFlag, topicFlag, descriptionFlag string
registerCmd := &cobra.Command{
	Use:   "register <plan-file>",
	Short: "register an untracked plan file (sets status to ready)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plansDir, err := resolvePlansDir()
		if err != nil {
			return err
		}
		if err := executePlanRegister(plansDir, args[0], branchFlag, topicFlag, descriptionFlag, resolveStore(plansDir)); err != nil {
			return err
		}
		fmt.Printf("registered: %s → ready\n", args[0])
		return nil
	},
}
registerCmd.Flags().StringVar(&branchFlag, "branch", "", "override branch name (default: plan/<slug>)")
registerCmd.Flags().StringVar(&topicFlag, "topic", "", "assign plan to a topic group (auto-creates topic if needed)")
registerCmd.Flags().StringVar(&descriptionFlag, "description", "", "override description (default: extracted from first # heading)")
```

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run 'TestPlanRegister$|TestPlanRegister_WithTopicAndDescription' -v
```

expected: PASS

**Step 5: run full test suite**

```bash
go test ./... -v
```

expected: all PASS (no other callers of `executePlanRegister` exist outside `cmd/plan.go`)

**Step 6: commit**

```bash
git add cmd/plan.go cmd/plan_test.go
git commit -m "feat: add --topic and --description flags to kas plan register"
```
