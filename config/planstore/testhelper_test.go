package planstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestStore creates an in-memory SQLiteStore for use in internal package tests.
// It registers a cleanup function to close the store when the test completes.
func newTestStore(t *testing.T) Store {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}
