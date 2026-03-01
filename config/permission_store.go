package config

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

// permissionCacheFile is the legacy JSON file name for the permission cache.
// Kept here so permission_migrate.go can reference it after permission_cache.go is removed.
const permissionCacheFile = "permission-cache.json"

// CacheKey returns a non-empty key for permission caching.
// Prefers the pattern (e.g. "/opt/*"); falls back to the description
// (e.g. "Execute bash command") so that permission types without a
// Patterns section can still be cached.
func CacheKey(pattern, description string) string {
	if pattern != "" {
		return pattern
	}
	return description
}

const permissionSchema = `
CREATE TABLE IF NOT EXISTS permissions (
	id         INTEGER PRIMARY KEY,
	project    TEXT NOT NULL,
	pattern    TEXT NOT NULL,
	decision   TEXT NOT NULL DEFAULT 'allow_always',
	created_at TEXT NOT NULL,
	UNIQUE(project, pattern)
);
`

// PermissionStore is the interface for persisting "allow always" permission decisions.
type PermissionStore interface {
	IsAllowedAlways(project, pattern string) bool
	Remember(project, pattern string)
	Forget(project, pattern string)
	ListPatterns(project string) []string
	Close() error
}

// SQLitePermissionStore is a PermissionStore backed by a SQLite database.
type SQLitePermissionStore struct {
	db *sql.DB
}

// NewSQLitePermissionStore opens (or creates) a SQLite database at dbPath and
// runs schema migrations. Use ":memory:" for an in-memory database (useful in tests).
func NewSQLitePermissionStore(dbPath string) (*SQLitePermissionStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Enable WAL mode for better concurrent read performance (not applicable for :memory:).
	if dbPath != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("set WAL mode: %w", err)
		}
	}

	if _, err := db.Exec(permissionSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}

	return &SQLitePermissionStore{db: db}, nil
}

// Close releases the database connection.
func (s *SQLitePermissionStore) Close() error {
	return s.db.Close()
}

// IsAllowedAlways returns true if the pattern has been stored as "allow_always"
// for the given project.
func (s *SQLitePermissionStore) IsAllowedAlways(project, pattern string) bool {
	const q = `SELECT 1 FROM permissions WHERE project = ? AND pattern = ? AND decision = 'allow_always'`
	var dummy int
	err := s.db.QueryRow(q, project, pattern).Scan(&dummy)
	return err == nil
}

// Remember stores a pattern as "allow_always" for the given project.
// If the entry already exists, it is replaced (idempotent).
func (s *SQLitePermissionStore) Remember(project, pattern string) {
	const q = `INSERT OR REPLACE INTO permissions (project, pattern, decision, created_at) VALUES (?, ?, 'allow_always', ?)`
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = s.db.Exec(q, project, pattern, createdAt)
}

// Forget removes a pattern from the store for the given project.
// If the entry does not exist, this is a no-op.
func (s *SQLitePermissionStore) Forget(project, pattern string) {
	const q = `DELETE FROM permissions WHERE project = ? AND pattern = ?`
	_, _ = s.db.Exec(q, project, pattern)
}

// ListPatterns returns all stored patterns for the given project, sorted alphabetically.
func (s *SQLitePermissionStore) ListPatterns(project string) []string {
	const q = `SELECT pattern FROM permissions WHERE project = ? ORDER BY pattern`
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var patterns []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		patterns = append(patterns, p)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return patterns
}
