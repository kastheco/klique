package taskstore

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id          INTEGER PRIMARY KEY,
	project     TEXT    NOT NULL,
	filename    TEXT    NOT NULL,
	status      TEXT    NOT NULL DEFAULT 'ready',
	description TEXT    NOT NULL DEFAULT '',
	branch      TEXT    NOT NULL DEFAULT '',
	topic       TEXT    NOT NULL DEFAULT '',
	created_at  TEXT    NOT NULL DEFAULT '',
	implemented TEXT    NOT NULL DEFAULT '',
	planning_at  TEXT    NOT NULL DEFAULT '',
	implementing_at TEXT NOT NULL DEFAULT '',
	reviewing_at TEXT    NOT NULL DEFAULT '',
	done_at     TEXT    NOT NULL DEFAULT '',
	goal                TEXT    NOT NULL DEFAULT '',
	pr_url              TEXT    NOT NULL DEFAULT '',
	pr_review_decision  TEXT    NOT NULL DEFAULT '',
	pr_check_status     TEXT    NOT NULL DEFAULT '',
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

// prReviewsTableMigration creates the pr_reviews table for tracking processed PR review comments.
const prReviewsTableMigration = `
	CREATE TABLE IF NOT EXISTS pr_reviews (
		id               INTEGER PRIMARY KEY,
		project          TEXT    NOT NULL,
		plan_filename    TEXT    NOT NULL,
		review_id        INTEGER NOT NULL,
		review_state     TEXT    NOT NULL DEFAULT '',
		review_body      TEXT    NOT NULL DEFAULT '',
		reviewer_login   TEXT    NOT NULL DEFAULT '',
		reaction_posted  INTEGER NOT NULL DEFAULT 0,
		fixer_dispatched INTEGER NOT NULL DEFAULT 0,
		created_at       TEXT    NOT NULL DEFAULT '',
		UNIQUE(project, plan_filename, review_id)
	)
`

// subtasksTableMigration creates the subtasks table for persisted plan subtasks.
const subtasksTableMigration = `
	CREATE TABLE IF NOT EXISTS subtasks (
		id INTEGER PRIMARY KEY,
		project TEXT NOT NULL,
		plan_filename TEXT NOT NULL,
		task_number INTEGER NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		UNIQUE(project, plan_filename, task_number),
		FOREIGN KEY (project, plan_filename) REFERENCES tasks(project, filename) ON DELETE CASCADE
	)
`

// contentMigration adds the content column to existing databases that predate it.
const contentMigration = `ALTER TABLE tasks ADD COLUMN content TEXT NOT NULL DEFAULT ''`

// planningAtMigration adds planning_at to existing databases.
const planningAtMigration = `ALTER TABLE tasks ADD COLUMN planning_at TEXT NOT NULL DEFAULT ''`

// implementingAtMigration adds implementing_at to existing databases.
const implementingAtMigration = `ALTER TABLE tasks ADD COLUMN implementing_at TEXT NOT NULL DEFAULT ''`

// reviewingAtMigration adds reviewing_at to existing databases.
const reviewingAtMigration = `ALTER TABLE tasks ADD COLUMN reviewing_at TEXT NOT NULL DEFAULT ''`

// doneAtMigration adds done_at to existing databases.
const doneAtMigration = `ALTER TABLE tasks ADD COLUMN done_at TEXT NOT NULL DEFAULT ''`

// goalMigration adds goal to existing databases.
const goalMigration = `ALTER TABLE tasks ADD COLUMN goal TEXT NOT NULL DEFAULT ''`

// clickupTaskIDMigration adds the clickup_task_id column to existing databases.
const clickupTaskIDMigration = `ALTER TABLE tasks ADD COLUMN clickup_task_id TEXT NOT NULL DEFAULT ''`

// reviewCycleMigration adds the review_cycle column to existing databases.
const reviewCycleMigration = `ALTER TABLE tasks ADD COLUMN review_cycle INTEGER NOT NULL DEFAULT 0`

// prURLMigration adds the pr_url column to existing databases.
const prURLMigration = `ALTER TABLE tasks ADD COLUMN pr_url TEXT NOT NULL DEFAULT ''`

// prReviewDecisionMigration adds the pr_review_decision column to existing databases.
const prReviewDecisionMigration = `ALTER TABLE tasks ADD COLUMN pr_review_decision TEXT NOT NULL DEFAULT ''`

// prCheckStatusMigration adds the pr_check_status column to existing databases.
const prCheckStatusMigration = `ALTER TABLE tasks ADD COLUMN pr_check_status TEXT NOT NULL DEFAULT ''`

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

	// Migrate: rename plans → tasks (if old table exists).
	// This MUST run before the schema CREATE TABLE so that existing data in the
	// plans table is preserved. If we create an empty tasks table first, the
	// rename migration sees tasks already exists and skips, orphaning old data.
	migrateRenameTable(db, "plans", "tasks")

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}

	// Migrate data from legacy planstore.db if it exists in the same directory
	// and the new tasks table is empty. This handles the rename-plan-to-task
	// transition where the DB filename changed from planstore.db to taskstore.db.
	if dbPath != ":memory:" {
		if err := migrateFromPlanstoreDB(db, dbPath); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate from planstore.db: %w", err)
		}
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

	// Add review_cycle column if it doesn't exist (upgrade existing databases).
	if err := migrateAddColumn(db, "review_cycle", reviewCycleMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate review_cycle column: %w", err)
	}

	// Add new task lifecycle phase timestamp and goal columns.
	if err := migrateAddColumn(db, "planning_at", planningAtMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate planning_at column: %w", err)
	}
	if err := migrateAddColumn(db, "implementing_at", implementingAtMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate implementing_at column: %w", err)
	}
	if err := migrateAddColumn(db, "reviewing_at", reviewingAtMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate reviewing_at column: %w", err)
	}
	if err := migrateAddColumn(db, "done_at", doneAtMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate done_at column: %w", err)
	}
	if err := migrateAddColumn(db, "goal", goalMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate goal column: %w", err)
	}

	// Add PR metadata columns if they don't exist (upgrade existing databases).
	if err := migrateAddColumn(db, "pr_url", prURLMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate pr_url column: %w", err)
	}
	if err := migrateAddColumn(db, "pr_review_decision", prReviewDecisionMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate pr_review_decision column: %w", err)
	}
	if err := migrateAddColumn(db, "pr_check_status", prCheckStatusMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate pr_check_status column: %w", err)
	}

	// Create subtasks table if missing.
	if _, err := db.Exec(subtasksTableMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("create subtasks table: %w", err)
	}

	// Create pr_reviews table if missing.
	if _, err := db.Exec(prReviewsTableMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("create pr_reviews table: %w", err)
	}

	if err := migrateStripMdSuffix(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate strip .md suffix: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// migrateAddContentColumn adds the content column to the tasks table if it
// doesn't already exist. This upgrades databases created before the column
// was introduced.
func migrateAddContentColumn(db *sql.DB) error {
	return migrateAddColumn(db, "content", contentMigration)
}

// migrateStripMdSuffix removes a trailing '.md' suffix from task and subtask
// plan filenames. This keeps existing task/subtask references in sync after the
// transition to extension-less plan filenames. OR IGNORE skips any row where
// stripping '.md' would collide with an already-existing bare-slug entry, so
// the migration is safe to run on databases that were partially updated.
func migrateStripMdSuffix(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin strip .md suffix transaction: %w", err)
	}

	if _, err = tx.Exec("PRAGMA defer_foreign_keys = ON"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("defer foreign keys for strip .md migration: %w", err)
	}

	if _, err = tx.Exec("UPDATE OR IGNORE tasks SET filename = SUBSTR(filename, 1, LENGTH(filename) - 3) WHERE filename LIKE '%.md'"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("strip .md suffix from tasks: %w", err)
	}

	if _, err = tx.Exec("UPDATE OR IGNORE subtasks SET plan_filename = SUBSTR(plan_filename, 1, LENGTH(plan_filename) - 3) WHERE plan_filename LIKE '%.md'"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("strip .md suffix from subtasks: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit strip .md suffix migration: %w", err)
	}

	return nil
}

// migrateAddColumn adds a column to the tasks table if it doesn't already
// exist, running the provided ALTER TABLE statement when needed.
func migrateAddColumn(db *sql.DB, columnName, alterSQL string) error {
	rows, err := db.Query("PRAGMA table_info(tasks)")
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

// migrateRenameTable renames oldName to newName if the old table exists and
// the new table does not. This is idempotent: subsequent runs are no-ops.
func migrateRenameTable(db *sql.DB, oldName, newName string) {
	// Check if old table exists.
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", oldName).Scan(&count)
	if err != nil || count == 0 {
		return // old table doesn't exist, nothing to migrate
	}
	// Check if new table already exists.
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", newName).Scan(&count)
	if err != nil || count > 0 {
		return // new table already exists
	}
	_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName))
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *SQLiteStore) Ping() error {
	return s.db.Ping()
}

// Create inserts a new task entry for the given project.
// Returns an error if a task with the same filename already exists in the project.
func (s *SQLiteStore) Create(project string, entry TaskEntry) error {
	const q = `
		INSERT INTO tasks (project, filename, status, description, branch, topic, created_at, implemented, planning_at, implementing_at, reviewing_at, done_at, goal, content, clickup_task_id, review_cycle, pr_url, pr_review_decision, pr_check_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		formatTime(entry.PlanningAt),
		formatTime(entry.ImplementingAt),
		formatTime(entry.ReviewingAt),
		formatTime(entry.DoneAt),
		entry.Goal,
		entry.Content,
		entry.ClickUpTaskID,
		entry.ReviewCycle,
		entry.PRURL,
		entry.PRReviewDecision,
		entry.PRCheckStatus,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("plan already exists: %s/%s", project, entry.Filename)
		}
		return fmt.Errorf("create plan: %w", err)
	}
	return nil
}

// Get retrieves a task entry by project and filename.
// Returns an error if the task is not found.
func (s *SQLiteStore) Get(project, filename string) (TaskEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, planning_at, implementing_at, reviewing_at, done_at, goal, content, clickup_task_id, review_cycle, pr_url, pr_review_decision, pr_check_status
		FROM tasks
		WHERE project = ? AND filename = ?
	`
	row := s.db.QueryRow(q, project, filename)
	return scanTaskEntry(row)
}

// Update replaces all fields of an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) Update(project, filename string, entry TaskEntry) error {
	const q = `
		UPDATE tasks
		SET status = ?, description = ?, branch = ?, topic = ?, created_at = ?, implemented = ?, planning_at = ?, implementing_at = ?, reviewing_at = ?, done_at = ?, goal = ?, clickup_task_id = ?, review_cycle = ?
		WHERE project = ? AND filename = ?
	`
	result, err := s.db.Exec(q,
		string(entry.Status),
		entry.Description,
		entry.Branch,
		entry.Topic,
		formatTime(entry.CreatedAt),
		entry.Implemented,
		formatTime(entry.PlanningAt),
		formatTime(entry.ImplementingAt),
		formatTime(entry.ReviewingAt),
		formatTime(entry.DoneAt),
		entry.Goal,
		entry.ClickUpTaskID,
		entry.ReviewCycle,
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

// Rename changes the filename of an existing task entry.
// Returns an error if the old filename is not found or the new filename already exists.
func (s *SQLiteStore) Rename(project, oldFilename, newFilename string) error {
	const q = `
		UPDATE tasks
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

// List returns all task entries for the given project, sorted by filename.
func (s *SQLiteStore) List(project string) ([]TaskEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, planning_at, implementing_at, reviewing_at, done_at, goal, content, clickup_task_id, review_cycle, pr_url, pr_review_decision, pr_check_status
		FROM tasks
		WHERE project = ?
		ORDER BY filename ASC
	`
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTaskEntries(rows)
}

// ListByStatus returns all task entries for the given project matching any of
// the provided statuses, sorted by filename.
func (s *SQLiteStore) ListByStatus(project string, statuses ...Status) ([]TaskEntry, error) {
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
		SELECT filename, status, description, branch, topic, created_at, implemented, planning_at, implementing_at, reviewing_at, done_at, goal, content, clickup_task_id, review_cycle, pr_url, pr_review_decision, pr_check_status
		FROM tasks
		WHERE project = ? AND status IN (%s)
		ORDER BY filename ASC
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks by status: %w", err)
	}
	defer rows.Close()
	return scanTaskEntries(rows)
}

// ListByTopic returns all task entries for the given project and topic,
// sorted by filename.
func (s *SQLiteStore) ListByTopic(project, topic string) ([]TaskEntry, error) {
	const q = `
		SELECT filename, status, description, branch, topic, created_at, implemented, planning_at, implementing_at, reviewing_at, done_at, goal, content, clickup_task_id, review_cycle, pr_url, pr_review_decision, pr_check_status
		FROM tasks
		WHERE project = ? AND topic = ?
		ORDER BY filename ASC
	`
	rows, err := s.db.Query(q, project, topic)
	if err != nil {
		return nil, fmt.Errorf("list tasks by topic: %w", err)
	}
	defer rows.Close()
	return scanTaskEntries(rows)
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

// GetContent retrieves only the content field for a task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) GetContent(project, filename string) (string, error) {
	const q = `SELECT content FROM tasks WHERE project = ? AND filename = ?`
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

// SetContent updates only the content field for an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) SetContent(project, filename, content string) error {
	const q = `UPDATE tasks SET content = ? WHERE project = ? AND filename = ?`
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

// SetClickUpTaskID sets the ClickUp task ID for an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) SetClickUpTaskID(project, filename, taskID string) error {
	const q = `UPDATE tasks SET clickup_task_id = ? WHERE project = ? AND filename = ?`
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

// IncrementReviewCycle atomically increments the review_cycle counter for an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) IncrementReviewCycle(project, filename string) error {
	const q = `UPDATE tasks SET review_cycle = review_cycle + 1 WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, project, filename)
	if err != nil {
		return fmt.Errorf("increment review_cycle: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("increment review_cycle rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// SetSubtasks replaces all subtasks for a plan in a transaction.
// Existing subtasks are removed before inserting the supplied rows.
func (s *SQLiteStore) SetSubtasks(project, filename string, subtasks []SubtaskEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin subtasks transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec("DELETE FROM subtasks WHERE project = ? AND plan_filename = ?", project, filename); err != nil {
		return fmt.Errorf("delete subtasks: %w", err)
	}

	for _, st := range subtasks {
		if _, err = tx.Exec(
			"INSERT INTO subtasks (project, plan_filename, task_number, title, status) VALUES (?, ?, ?, ?, ?)",
			project, filename, st.TaskNumber, st.Title, string(st.Status),
		); err != nil {
			return fmt.Errorf("insert subtask: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit subtasks: %w", err)
	}
	return nil
}

// GetSubtasks returns all subtasks for a plan, sorted by task_number.
func (s *SQLiteStore) GetSubtasks(project, filename string) ([]SubtaskEntry, error) {
	rows, err := s.db.Query(
		`SELECT task_number, title, status FROM subtasks WHERE project = ? AND plan_filename = ? ORDER BY task_number ASC`,
		project,
		filename,
	)
	if err != nil {
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	defer rows.Close()

	var subtasks []SubtaskEntry
	for rows.Next() {
		var taskNumber int
		var title, status string
		if err := rows.Scan(&taskNumber, &title, &status); err != nil {
			return nil, fmt.Errorf("scan subtask: %w", err)
		}
		subtasks = append(subtasks, SubtaskEntry{
			TaskNumber: taskNumber,
			Title:      title,
			Status:     SubtaskStatus(status),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subtasks: %w", err)
	}
	return subtasks, nil
}

// UpdateSubtaskStatus updates the status of a specific subtask.
func (s *SQLiteStore) UpdateSubtaskStatus(project, filename string, taskNumber int, status SubtaskStatus) error {
	const q = `
		UPDATE subtasks
		SET status = ?
		WHERE project = ? AND plan_filename = ? AND task_number = ?
	`
	result, err := s.db.Exec(q, string(status), project, filename, taskNumber)
	if err != nil {
		return fmt.Errorf("update subtask status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update subtask status rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("subtask not found: %s/%s#%d", project, filename, taskNumber)
	}
	return nil
}

// SetPhaseTimestamp sets the timestamp for the requested lifecycle phase.
// Known phases are: planning, implementing, reviewing, done.
func (s *SQLiteStore) SetPhaseTimestamp(project, filename, phase string, ts time.Time) error {
	var column string
	switch phase {
	case "planning":
		column = "planning_at"
	case "implementing":
		column = "implementing_at"
	case "reviewing":
		column = "reviewing_at"
	case "done":
		column = "done_at"
	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}

	query := fmt.Sprintf("UPDATE tasks SET %s = ? WHERE project = ? AND filename = ?", column)
	result, err := s.db.Exec(query, formatTime(ts), project, filename)
	if err != nil {
		return fmt.Errorf("set phase timestamp: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set phase timestamp rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// SetPlanGoal sets the goal text for a plan.
func (s *SQLiteStore) SetPlanGoal(project, filename, goal string) error {
	const q = `UPDATE tasks SET goal = ? WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, goal, project, filename)
	if err != nil {
		return fmt.Errorf("set plan goal: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set plan goal rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// SetPRURL sets the pull request URL for an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) SetPRURL(project, filename, url string) error {
	const q = `UPDATE tasks SET pr_url = ? WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, url, project, filename)
	if err != nil {
		return fmt.Errorf("set pr_url: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set pr_url rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// SetPRState sets the review decision and check status for an existing task entry.
// Returns an error if the task is not found.
func (s *SQLiteStore) SetPRState(project, filename, reviewDecision, checkStatus string) error {
	const q = `UPDATE tasks SET pr_review_decision = ?, pr_check_status = ? WHERE project = ? AND filename = ?`
	result, err := s.db.Exec(q, reviewDecision, checkStatus, project, filename)
	if err != nil {
		return fmt.Errorf("set pr_state: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set pr_state rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("plan not found: %s/%s", project, filename)
	}
	return nil
}

// RecordPRReview inserts a new PR review record. INSERT OR IGNORE ensures
// repeated polls for the same review ID are idempotent — only the first record wins.
func (s *SQLiteStore) RecordPRReview(project, filename string, reviewID int, state, body, reviewer string) error {
	const q = `
		INSERT OR IGNORE INTO pr_reviews (project, plan_filename, review_id, review_state, review_body, reviewer_login, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(q, project, filename, reviewID, state, body, reviewer, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("record pr review: %w", err)
	}
	return nil
}

// IsReviewProcessed returns true if a review record exists for the given reviewID.
// Returns false on any error or if the row is not found.
func (s *SQLiteStore) IsReviewProcessed(project, filename string, reviewID int) bool {
	const q = `SELECT COUNT(*) FROM pr_reviews WHERE project = ? AND plan_filename = ? AND review_id = ?`
	var count int
	err := s.db.QueryRow(q, project, filename, reviewID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// MarkReviewReacted sets reaction_posted = 1 for the given review.
// Returns an error if the review row is not found.
func (s *SQLiteStore) MarkReviewReacted(project, filename string, reviewID int) error {
	const q = `UPDATE pr_reviews SET reaction_posted = 1 WHERE project = ? AND plan_filename = ? AND review_id = ?`
	result, err := s.db.Exec(q, project, filename, reviewID)
	if err != nil {
		return fmt.Errorf("mark review reacted: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark review reacted rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("pr review not found: %s/%s#%d", project, filename, reviewID)
	}
	return nil
}

// MarkReviewFixerDispatched sets fixer_dispatched = 1 for the given review.
// Returns an error if the review row is not found.
func (s *SQLiteStore) MarkReviewFixerDispatched(project, filename string, reviewID int) error {
	const q = `UPDATE pr_reviews SET fixer_dispatched = 1 WHERE project = ? AND plan_filename = ? AND review_id = ?`
	result, err := s.db.Exec(q, project, filename, reviewID)
	if err != nil {
		return fmt.Errorf("mark review fixer dispatched: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark review fixer dispatched rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("pr review not found: %s/%s#%d", project, filename, reviewID)
	}
	return nil
}

// ListPendingReviews returns all review entries where fixer_dispatched = 0,
// ordered by review_id ascending. Returns an empty (non-nil) slice when there are no rows.
func (s *SQLiteStore) ListPendingReviews(project, filename string) ([]PRReviewEntry, error) {
	const q = `
		SELECT review_id, review_state, review_body, reviewer_login, reaction_posted, fixer_dispatched, created_at
		FROM pr_reviews
		WHERE project = ? AND plan_filename = ? AND fixer_dispatched = 0
		ORDER BY review_id ASC
	`
	rows, err := s.db.Query(q, project, filename)
	if err != nil {
		return nil, fmt.Errorf("list pending pr reviews: %w", err)
	}
	defer rows.Close()

	entries := []PRReviewEntry{} // non-nil empty slice
	for rows.Next() {
		var e PRReviewEntry
		var reactionPosted, fixerDispatched int
		var createdAt string
		if err := rows.Scan(&e.ReviewID, &e.ReviewState, &e.ReviewBody, &e.ReviewerLogin, &reactionPosted, &fixerDispatched, &createdAt); err != nil {
			return nil, fmt.Errorf("list pending pr reviews: %w", err)
		}
		e.ReactionPosted = reactionPosted != 0
		e.FixerDispatched = fixerDispatched != 0
		e.CreatedAt = parseTime(createdAt)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending pr reviews: %w", err)
	}
	return entries, nil
}

// scanTaskEntry scans a single row into a TaskEntry.
func scanTaskEntry(row *sql.Row) (TaskEntry, error) {
	var filename, status, description, branch, topic, createdAt, implemented, planningAt, implementingAt, reviewingAt, doneAt, goal, content, clickupTaskID string
	var reviewCycle int
	var prURL, prReviewDecision, prCheckStatus string
	if err := row.Scan(
		&filename,
		&status,
		&description,
		&branch,
		&topic,
		&createdAt,
		&implemented,
		&planningAt,
		&implementingAt,
		&reviewingAt,
		&doneAt,
		&goal,
		&content,
		&clickupTaskID,
		&reviewCycle,
		&prURL,
		&prReviewDecision,
		&prCheckStatus,
	); err != nil {
		if err == sql.ErrNoRows {
			return TaskEntry{}, fmt.Errorf("plan not found")
		}
		return TaskEntry{}, fmt.Errorf("scan plan: %w", err)
	}
	return TaskEntry{
		Filename:         filename,
		Status:           Status(status),
		Description:      description,
		Branch:           branch,
		Topic:            topic,
		CreatedAt:        parseTime(createdAt),
		Implemented:      implemented,
		PlanningAt:       parseTime(planningAt),
		ImplementingAt:   parseTime(implementingAt),
		ReviewingAt:      parseTime(reviewingAt),
		DoneAt:           parseTime(doneAt),
		Goal:             goal,
		Content:          content,
		ClickUpTaskID:    clickupTaskID,
		ReviewCycle:      reviewCycle,
		PRURL:            prURL,
		PRReviewDecision: prReviewDecision,
		PRCheckStatus:    prCheckStatus,
	}, nil
}

// scanTaskEntries scans multiple rows into a slice of TaskEntry.
func scanTaskEntries(rows *sql.Rows) ([]TaskEntry, error) {
	var entries []TaskEntry
	for rows.Next() {
		var filename, status, description, branch, topic, createdAt, implemented, planningAt, implementingAt, reviewingAt, doneAt, goal, content, clickupTaskID string
		var reviewCycle int
		var prURL, prReviewDecision, prCheckStatus string
		if err := rows.Scan(
			&filename,
			&status,
			&description,
			&branch,
			&topic,
			&createdAt,
			&implemented,
			&planningAt,
			&implementingAt,
			&reviewingAt,
			&doneAt,
			&goal,
			&content,
			&clickupTaskID,
			&reviewCycle,
			&prURL,
			&prReviewDecision,
			&prCheckStatus,
		); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		entries = append(entries, TaskEntry{
			Filename:         filename,
			Status:           Status(status),
			Description:      description,
			Branch:           branch,
			Topic:            topic,
			CreatedAt:        parseTime(createdAt),
			Implemented:      implemented,
			PlanningAt:       parseTime(planningAt),
			ImplementingAt:   parseTime(implementingAt),
			ReviewingAt:      parseTime(reviewingAt),
			DoneAt:           parseTime(doneAt),
			Goal:             goal,
			Content:          content,
			ClickUpTaskID:    clickupTaskID,
			ReviewCycle:      reviewCycle,
			PRURL:            prURL,
			PRReviewDecision: prReviewDecision,
			PRCheckStatus:    prCheckStatus,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
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
