package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
