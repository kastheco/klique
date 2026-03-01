package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSQLitePermissionStore_Forget(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	store.Remember("test-project", "/opt/*")
	assert.True(t, store.IsAllowedAlways("test-project", "/opt/*"))

	store.Forget("test-project", "/opt/*")
	assert.False(t, store.IsAllowedAlways("test-project", "/opt/*"))
}

func TestSQLitePermissionStore_ForgetNonExistent(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Forget on a non-existent entry should not panic or error
	store.Forget("test-project", "/opt/*")
	assert.False(t, store.IsAllowedAlways("test-project", "/opt/*"))
}

func TestSQLitePermissionStore_ListPatterns(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Empty list for unknown project
	patterns := store.ListPatterns("test-project")
	assert.Empty(t, patterns)

	store.Remember("test-project", "/opt/*")
	store.Remember("test-project", "/tmp/*")
	store.Remember("other-project", "/var/*")

	patterns = store.ListPatterns("test-project")
	assert.Equal(t, []string{"/opt/*", "/tmp/*"}, patterns)

	otherPatterns := store.ListPatterns("other-project")
	assert.Equal(t, []string{"/var/*"}, otherPatterns)
}

func TestSQLitePermissionStore_CloseIdempotent(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)

	// First close should succeed
	err = store.Close()
	assert.NoError(t, err)

	// Second close should not panic (may return an error, that's acceptable)
	_ = store.Close()
}

func TestSQLitePermissionStore_SchemaCreatesTable(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// If the table was created, we should be able to use it without error
	store.Remember("proj", "/path/*")
	assert.True(t, store.IsAllowedAlways("proj", "/path/*"))
}

func TestSQLitePermissionStore_RememberIdempotent(t *testing.T) {
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Remembering the same pattern twice should not error
	store.Remember("test-project", "/opt/*")
	store.Remember("test-project", "/opt/*")
	assert.True(t, store.IsAllowedAlways("test-project", "/opt/*"))

	// Should still only appear once in list
	patterns := store.ListPatterns("test-project")
	assert.Equal(t, []string{"/opt/*"}, patterns)
}

func TestSQLitePermissionStore_Interface(t *testing.T) {
	// Verify SQLitePermissionStore satisfies the PermissionStore interface
	store, err := NewSQLitePermissionStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	var _ PermissionStore = store
}
