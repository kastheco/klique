package planstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
