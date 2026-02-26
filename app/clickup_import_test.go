package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportClickUpTask_WritesScaffold(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	task := &clickup.Task{
		ID:          "abc123",
		Name:        "Design Auth Flow",
		Description: "Implement OAuth2 for the API gateway",
		Status:      "In Progress",
		URL:         "https://app.clickup.com/t/abc123",
		Subtasks: []clickup.Subtask{
			{Name: "Add login endpoint", Status: "open"},
			{Name: "Add token refresh", Status: "open"},
		},
	}

	scaffold := clickup.ScaffoldPlan(*task)
	filename := clickup.ScaffoldFilename(task.Name, "2026-02-24")
	planPath := filepath.Join(plansDir, filename)
	require.NoError(t, os.WriteFile(planPath, []byte(scaffold), 0o644))

	// Verify file exists and contains expected content
	data, err := os.ReadFile(planPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "**Goal:** Implement OAuth2")
	assert.Contains(t, content, "**Source:** ClickUp abc123")
	assert.Contains(t, content, "- [ ] Add login endpoint")
	assert.Contains(t, content, "- [ ] Add token refresh")
}

func TestScaffoldFilename_Dedup(t *testing.T) {
	dir := t.TempDir()
	base := clickup.ScaffoldFilename("Test Task", "2026-02-24")

	// No collision keeps original filename.
	assert.Equal(t, base, dedupePlanFilename(dir, base))

	// First collision returns -2 suffix.
	require.NoError(t, os.WriteFile(filepath.Join(dir, base), []byte("x"), 0o644))
	name2 := dedupePlanFilename(dir, base)
	assert.Equal(t, "2026-02-24-test-task-2.md", name2)

	// Second collision returns -3 suffix.
	require.NoError(t, os.WriteFile(filepath.Join(dir, name2), []byte("x"), 0o644))
	assert.Equal(t, "2026-02-24-test-task-3.md", dedupePlanFilename(dir, base))
}
