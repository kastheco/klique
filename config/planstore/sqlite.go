package planstore

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

const schema = `
CREATE TABLE IF NOT EXISTS plans (
	id          INTEGER PRIMARY KEY,
	project     TEXT    NOT NULL,
	filename    TEXT    NOT NULL,
	status      TEXT    NOT NULL DEFAULT 'ready',
	description TEXT    NOT NULL DEFAULT '',
	branch      TEXT    NOT NULL DEFAULT '',
	topic       TEXT    NOT NULL DEFAULT '',
	created_at  TEXT    NOT NULL DEFAULT '',
	implemented TEXT    NOT NULL DEFAULT '',
	UNIQUE(project, filename)
);

CREATE TABLE IF NOT EXISTS topics (
	id         INTEGER PRIMARY KEY,
	project    TEXT NOT NULL,
	name       TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT '',
	UNIQUE(project, name)
);
`

// contentMigration adds the content column to existing databases that predate it.
const contentMigration = `ALTER TABLE plans ADD COLUMN content TEXT NOT NULL DEFAULT ''`

// clickupTaskIDMigration adds the clickup_task_id column to existing databases.
const clickupTaskIDMigration = `ALTER TABLE plans ADD COLUMN clickup_task_id TEXT NOT NULL DEFAULT ''`

// SQLiteStore is a Store implementation backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and runs
// schema migrations. Use ":memory:" for an in-memory database (useful in tests).
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if dbPath != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("set WAL mode: %w", err)
		}
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}

	// Add content column if it doesn't exist (upgrade existing databases).
	if err := migrateAddContentColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate content column: %w", err)
	}

	// Add clickup_task_id column if it doesn't exist (upgrade existing databases).
	if err := migrateAddColumn(db, "clickup_task_id", clickupTaskIDMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate clickup_task_id column: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// migrateAddContentColumn adds the content column to the plans table if it
// doesn't already exist. This upgrades databases created before the column
// was introduced.
func migrateAddContentColumn(db *sql.DB) error {
	return migrateAddColumn(db, "content", contentMigration)
}

// migrateAddColumn adds a column to the plans table if it doesn't already
// exist, running the provided ALTER TABLE statement when needed.
func migrateAddColumn(db *sql.DB, columnName, alterSQL string) error {
	rows, err := db.Query("PRAGMA table_info(plans)")
	if err != nil {
		return fmt.Errorf("query table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if name == columnName {
			return nil // column already exists
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info: %w", err)
	}

	// Column doesn't exist — add it.
	if _, err := db.Exec(alterSQL); err != nil {
		return fmt.Errorf("add %s column: %w", columnName, err)
	}
	return nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *SQLiteStore) Ping() error {
	return s.db.Ping()
}

// Create inserts a new plan entry for the given project.
// Returns an error if a plan with the same filename already exists in the project.
func (s *SQLiteStore) Create(project string, entry PlanEntry) error {
	const q = `
		INSERT INTO plans (project, filename, status, description, branch, topic, created_at, implemented, content, clickup_task_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(q,
		project,
		entry.Filename,
		string(entry.Status),
		entry.Description,
		entry.Branch,
		entry.Topic,
		formatTime(entry.CreatedAt),
		entry.Implemented,
		entry.Content,
		entry.ClickUpTaskID,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("plan already exists: %s/%s", project, entry.Filename)
		}
		return fmt.Errorf("create plan: %w", err)
	}
	return nil
}

// Get retrieves a plan entry by project and filename.
// Returns an error if the plan is not found.
func (s *SQLiteStore) Get(project, filename string) (PlanEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, content, clickup_task_id
		FROM plans
		WHERE project = ? AND filename = ?
	`
	row := s.db.QueryRow(q, project, filename)
	return scanPlanEntry(row)
}

// Update replaces all fields of an existing plan entry.
// Returns an error if the plan is not found.
func (s *SQLiteStore) Update(project, filename string, entry PlanEntry) error {
	const q = `
		UPDATE plans
		SET status = ?, description = ?, branch = ?, topic = ?, created_at = ?, implemented = ?, content = ?, clickup_task_id = ?
		WHERE project = ? AND filename = ?
	`
	result, err := s.db.Exec(q,
		string(entry.Status),
		entry.Description,
		entry.Branch,
		entry.Topic,
		formatTime(entry.CreatedAt),
		entry.Implemented,
		entry.Content,
		entry.ClickUpTaskID,
		project,
		filename,
	)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update plan rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// Rename changes the filename of an existing plan entry.
// Returns an error if the old filename is not found or the new filename already exists.
func (s *SQLiteStore) Rename(project, oldFilename, newFilename string) error {
	const q = `
		UPDATE plans
		SET filename = ?
		WHERE project = ? AND filename = ?
	`
	result, err := s.db.Exec(q, newFilename, project, oldFilename)
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("plan already exists: %s/%s", project, newFilename)
		}
		return fmt.Errorf("rename plan: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rename plan rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, oldFilename)
	}
	return nil
}

// List returns all plan entries for the given project, sorted by filename.
func (s *SQLiteStore) List(project string) ([]PlanEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, content, clickup_task_id
		FROM plans
		WHERE project = ?
		ORDER BY filename ASC
	`
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	return scanPlanEntries(rows)
}

// ListByStatus returns all plan entries for the given project matching any of
// the provided statuses, sorted by filename.
func (s *SQLiteStore) ListByStatus(project string, statuses ...Status) ([]PlanEntry, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]any, 0, len(statuses)+1)
	args = append(args, project)
	for i, s := range statuses {
		placeholders[i] = "?"
		args = append(args, string(s))
	}

	q := fmt.Sprintf(`
		SELECT filename, status, description, branch, topic, created_at, implemented, content, clickup_task_id
		FROM plans
		WHERE project = ? AND status IN (%s)
		ORDER BY filename ASC
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list plans by status: %w", err)
	}
	defer rows.Close()
	return scanPlanEntries(rows)
}

// ListByTopic returns all plan entries for the given project and topic,
// sorted by filename.
func (s *SQLiteStore) ListByTopic(project, topic string) ([]PlanEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, content, clickup_task_id
		FROM plans
		WHERE project = ? AND topic = ?
		ORDER BY filename ASC
	`
	rows, err := s.db.Query(q, project, topic)
	if err != nil {
		return nil, fmt.Errorf("list plans by topic: %w", err)
	}
	defer rows.Close()
	return scanPlanEntries(rows)
}

// ListTopics returns all topic entries for the given project, sorted by name.
func (s *SQLiteStore) ListTopics(project string) ([]TopicEntry, error) {
	const q = `
		SELECT name, created_at
		FROM topics
		WHERE project = ?
		ORDER BY name ASC
	`
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()

	var topics []TopicEntry
	for rows.Next() {
		var name, createdAt string
		if err := rows.Scan(&name, &createdAt); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, TopicEntry{
			Name:      name,
			CreatedAt: parseTime(createdAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics: %w", err)
	}
	return topics, nil
}

// CreateTopic inserts a new topic entry for the given project.
// Returns an error if a topic with the same name already exists in the project.
func (s *SQLiteStore) CreateTopic(project string, entry TopicEntry) error {
	const q = `
		INSERT INTO topics (project, name, created_at)
		VALUES (?, ?, ?)
	`
	_, err := s.db.Exec(q, project, entry.Name, formatTime(entry.CreatedAt))
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("topic already exists: %s/%s", project, entry.Name)
		}
		return fmt.Errorf("create topic: %w", err)
	}
	return nil
}

// GetContent retrieves only the content field for a plan entry.
// Returns an error if the plan is not found.
func (s *SQLiteStore) GetContent(project, filename string) (string, error) {
	const q = `SELECT content FROM plans WHERE project = ? AND filename = ?`
	var content string
	err := s.db.QueryRow(q, project, filename).Scan(&content)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("plan not found: %s/%s", project, filename)
		}
		return "", fmt.Errorf("get content: %w", err)
	}
	return content, nil
}

// SetContent updates only the content field for an existing plan entry.
// Returns an error if the plan is not found.
func (s *SQLiteStore) SetContent(project, filename, content string) error {
	const q = `UPDATE plans SET content = ? WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, content, project, filename)
	if err != nil {
		return fmt.Errorf("set content: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set content rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// SetClickUpTaskID sets the ClickUp task ID for an existing plan entry.
// Returns an error if the plan is not found.
func (s *SQLiteStore) SetClickUpTaskID(project, filename, taskID string) error {
	const q = `UPDATE plans SET clickup_task_id = ? WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, taskID, project, filename)
	if err != nil {
		return fmt.Errorf("set clickup_task_id: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set clickup_task_id rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// scanPlanEntry scans a single row into a PlanEntry.
func scanPlanEntry(row *sql.Row) (PlanEntry, error) {
	var filename, status, description, branch, topic, createdAt, implemented, content, clickupTaskID string
	if err := row.Scan(&filename, &status, &description, &branch, &topic, &createdAt, &implemented, &content, &clickupTaskID); err != nil {
		if err == sql.ErrNoRows {
			return PlanEntry{}, fmt.Errorf("plan not found")
		}
		return PlanEntry{}, fmt.Errorf("scan plan: %w", err)
	}
	return PlanEntry{
		Filename:      filename,
		Status:        Status(status),
		Description:   description,
		Branch:        branch,
		Topic:         topic,
		CreatedAt:     parseTime(createdAt),
		Implemented:   implemented,
		Content:       content,
		ClickUpTaskID: clickupTaskID,
	}, nil
}

// scanPlanEntries scans multiple rows into a slice of PlanEntry.
func scanPlanEntries(rows *sql.Rows) ([]PlanEntry, error) {
	var entries []PlanEntry
	for rows.Next() {
		var filename, status, description, branch, topic, createdAt, implemented, content, clickupTaskID string
		if err := rows.Scan(&filename, &status, &description, &branch, &topic, &createdAt, &implemented, &content, &clickupTaskID); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		entries = append(entries, PlanEntry{
			Filename:      filename,
			Status:        Status(status),
			Description:   description,
			Branch:        branch,
			Topic:         topic,
			CreatedAt:     parseTime(createdAt),
			Implemented:   implemented,
			Content:       content,
			ClickUpTaskID: clickupTaskID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}
	return entries, nil
}

// formatTime formats a time.Time as RFC3339 for storage. Zero time returns empty string.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime parses an RFC3339 string. Returns zero time on empty or invalid input.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// isUniqueConstraintError returns true if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
