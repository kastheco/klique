package cmd

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestPlanState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ps := &planstate.PlanState{
		Dir:          dir,
		Plans:        make(map[string]planstate.PlanEntry),
		TopicEntries: make(map[string]planstate.TopicEntry),
	}
	ps.Plans["2026-02-20-test-plan.md"] = planstate.PlanEntry{
		Status:      "ready",
		Description: "test plan",
		Branch:      "plan/test-plan",
	}
	ps.Plans["2026-02-20-implementing-plan.md"] = planstate.PlanEntry{
		Status:      "implementing",
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
	}
	require.NoError(t, ps.Save())
	return dir
}

func TestPlanList(t *testing.T) {
	dir := setupTestPlanState(t)

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
			output := executePlanList(dir, tt.statusFilter, nil)
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
	dir := setupTestPlanState(t)

	// Requires --force
	err := executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", false, nil)
	assert.Error(t, err, "should require --force flag")

	// Valid override
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", true, nil)
	require.NoError(t, err)

	ps, err := planstate.Load(dir)
	require.NoError(t, err)
	entry, ok := ps.Entry("2026-02-20-test-plan.md")
	require.True(t, ok)
	assert.Equal(t, planstate.Status("done"), entry.Status)

	// Invalid status
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "bogus", true, nil)
	assert.Error(t, err, "should reject invalid status")
}

func TestPlanTransition(t *testing.T) {
	dir := setupTestPlanState(t)

	// Valid transition: ready → planning via plan_start
	newStatus, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start", nil)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)

	// Invalid transition (plan is now in "planning" state)
	_, err = executePlanTransition(dir, "2026-02-20-test-plan.md", "review_approved", nil)
	assert.Error(t, err)
}

func TestPlanCLI_EndToEnd(t *testing.T) {
	dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// List all
	output := executePlanList(dir, "", nil)
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "implementing")

	// Transition ready → planning
	status, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start", nil)
	require.NoError(t, err)
	assert.Equal(t, "planning", status)

	// Force set back to ready
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "ready", true, nil)
	require.NoError(t, err)

	// Implement with wave signal
	err = executePlanImplement(dir, "2026-02-20-test-plan.md", 2, nil)
	require.NoError(t, err)

	// Verify signal file
	entries, _ := os.ReadDir(signalsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "implement-wave-2-2026-02-20-test-plan.md")

	// Verify final status
	ps, _ := planstate.Load(dir)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.Status("implementing"), entry.Status)
}

func TestPlanImplement(t *testing.T) {
	dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	err := executePlanImplement(dir, "2026-02-20-test-plan.md", 1, nil)
	require.NoError(t, err)

	// Verify plan transitioned to implementing
	ps, err := planstate.Load(dir)
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
