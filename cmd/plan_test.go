package cmd

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestPlanState creates an in-memory SQLite store pre-populated with two
// test plans and returns the store and a temp plans directory.
// The plans directory is structured as <root>/docs/plans so that
// projectFromPlansDir returns a stable project name derived from <root>.
func setupTestPlanState(t *testing.T) (planstore.Store, string) {
	t.Helper()
	store := planstore.NewTestSQLiteStore(t)

	// Build a plans dir with the expected structure so projectFromPlansDir
	// returns a known project name.
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	project := projectFromPlansDir(plansDir)

	require.NoError(t, store.Create(project, planstore.PlanEntry{
		Filename:    "2026-02-20-test-plan.md",
		Status:      planstore.StatusReady,
		Description: "test plan",
		Branch:      "plan/test-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, planstore.PlanEntry{
		Filename:    "2026-02-20-implementing-plan.md",
		Status:      planstore.Status("implementing"),
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
		CreatedAt:   time.Now(),
	}))

	return store, plansDir
}

func TestPlanList(t *testing.T) {
	store, dir := setupTestPlanState(t)

	tests := []struct {
		name           string
		statusFilter   string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "all plans",
			wantContains: []string{"2026-02-20-test-plan.md", "2026-02-20-implementing-plan.md"},
		},
		{
			name:           "filter by ready",
			statusFilter:   "ready",
			wantContains:   []string{"2026-02-20-test-plan.md"},
			wantNotContain: []string{"2026-02-20-implementing-plan.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := executePlanList(dir, tt.statusFilter, store)
			for _, want := range tt.wantContains {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}

func TestPlanSetStatus(t *testing.T) {
	store, dir := setupTestPlanState(t)

	// Requires --force
	err := executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", false, store)
	assert.Error(t, err, "should require --force flag")

	// Valid override
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", true, store)
	require.NoError(t, err)

	ps, err := planstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, ok := ps.Entry("2026-02-20-test-plan.md")
	require.True(t, ok)
	assert.Equal(t, planstate.Status("done"), entry.Status)

	// Invalid status
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "bogus", true, store)
	assert.Error(t, err, "should reject invalid status")
}

func TestPlanTransition(t *testing.T) {
	store, dir := setupTestPlanState(t)

	// Valid transition: ready → planning via plan_start
	newStatus, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)

	// Invalid transition (plan is now in "planning" state)
	_, err = executePlanTransition(dir, "2026-02-20-test-plan.md", "review_approved", store)
	assert.Error(t, err)
}

func TestPlanCLI_EndToEnd(t *testing.T) {
	store, dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// List all
	output := executePlanList(dir, "", store)
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "implementing")

	// Transition ready → planning
	status, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", status)

	// Force set back to ready
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "ready", true, store)
	require.NoError(t, err)

	// Implement with wave signal
	err = executePlanImplement(dir, "2026-02-20-test-plan.md", 2, store)
	require.NoError(t, err)

	// Verify signal file
	entries, _ := os.ReadDir(signalsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "implement-wave-2-2026-02-20-test-plan.md")

	// Verify final status
	ps, err := planstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.Status("implementing"), entry.Status)
}

func TestPlanImplement(t *testing.T) {
	store, dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	err := executePlanImplement(dir, "2026-02-20-test-plan.md", 1, store)
	require.NoError(t, err)

	// Verify plan transitioned to implementing
	ps, err := planstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.Status("implementing"), entry.Status)

	// Verify signal file created
	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	var found bool
	for _, e := range entries {
		if e.Name() == "implement-wave-1-2026-02-20-test-plan.md" {
			found = true
		}
	}
	assert.True(t, found, "signal file should exist")
}

// TestPlanList_WithStore verifies that executePlanListWithStore works with a
// store-backed HTTP server, returning plan entries from the remote store.
func TestPlanList_WithStore(t *testing.T) {
	backend := planstore.NewTestSQLiteStore(t)
	srv := httptest.NewServer(planstore.NewHandler(backend))
	defer srv.Close()

	err := backend.Create("test-project", planstore.PlanEntry{
		Filename: "test.md", Status: "ready", Description: "test plan",
	})
	require.NoError(t, err)

	output := executePlanListWithStore(srv.URL, "test-project")
	assert.Contains(t, output, "test.md")
	assert.Contains(t, output, "ready")
}
