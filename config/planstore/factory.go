package planstore

import (
	"os"
	"path/filepath"
)

// NewStoreFromConfig creates a Store from a plan store URL and project name.
// If planStoreURL is empty, it returns (nil, nil) — the caller should fall
// back to legacy plan-state.json behavior.
// The returned store uses lazy connection: the URL is validated syntactically
// but no network connection is made until the first operation (or Ping).
func NewStoreFromConfig(planStoreURL, project string) (Store, error) {
	if planStoreURL == "" {
		return nil, nil // no remote store configured
	}
	return NewHTTPStore(planStoreURL, project), nil
}

// ResolvedDBPath returns the filesystem path that the factory would use for a
// local SQLite planstore. It reads the XDG-compliant config directory
// (~/.config/kasmos/) and appends "planstore.db". This path is shared with
// the auditlog SQLiteLogger so both can coexist in the same database file
// (each using a separate table).
func ResolvedDBPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "planstore.db")
	}
	return filepath.Join(homeDir, ".config", "kasmos", "planstore.db")
}
