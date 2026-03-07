package taskstore

import "testing"

// NewTestSQLiteStore creates an in-memory SQLiteStore for use in tests.
// It registers a cleanup function to close the store when the test completes.
// This is exported so external packages can use it in their tests.
func NewTestSQLiteStore(t testing.TB) Store {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewTestSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// NewTestStore creates an in-memory Store for use in tests.
// Alias for NewTestSQLiteStore.
func NewTestStore(t testing.TB) Store {
	return NewTestSQLiteStore(t)
}
