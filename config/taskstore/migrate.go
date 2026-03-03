package taskstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jsonTaskEntry is the on-disk format for a single plan in plan-state.json.
type jsonTaskEntry struct {
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Topic       string `json:"topic,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Implemented string `json:"implemented,omitempty"`
}

// jsonTopicEntry is the on-disk format for a single topic in plan-state.json.
type jsonTopicEntry struct {
	CreatedAt string `json:"created_at"`
}

// jsonTaskState is the top-level structure of plan-state.json.
type jsonTaskState struct {
	Plans  map[string]jsonTaskEntry  `json:"plans"`
	Topics map[string]jsonTopicEntry `json:"topics"`
}

// MigrateFromJSON reads plan-state.json from plansDir and imports all plans
// and topics into the store under the given project. If plan-state.json does
// not exist, it returns (0, nil) — a no-op. The migration is idempotent:
// plans and topics that already exist in the store are silently skipped.
// For each plan entry that has a corresponding .md file in plansDir, the file
// content is also imported via SetContent.
//
// Returns the number of plans successfully migrated (newly created).
func MigrateFromJSON(store Store, project, plansDir string) (int, error) {
	stateFile := filepath.Join(plansDir, "plan-state.json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read plan-state.json: %w", err)
	}

	var state jsonTaskState
	if err := json.Unmarshal(data, &state); err != nil {
		return 0, fmt.Errorf("parse plan-state.json: %w", err)
	}

	migrated := 0

	// Migrate plans.
	for filename, jp := range state.Plans {
		entry := TaskEntry{
			Filename:    filename,
			Status:      Status(jp.Status),
			Description: jp.Description,
			Branch:      jp.Branch,
			Topic:       jp.Topic,
			Implemented: jp.Implemented,
		}
		if jp.CreatedAt != "" {
			entry.CreatedAt = parseTime(jp.CreatedAt)
		}

		if err := store.Create(project, entry); err != nil {
			// Skip if already exists (idempotent).
			if strings.Contains(err.Error(), "plan already exists") {
				continue
			}
			return migrated, fmt.Errorf("migrate plan %s: %w", filename, err)
		}
		migrated++

		// Import .md file content if it exists on disk.
		mdPath := filepath.Join(plansDir, filename)
		content, err := os.ReadFile(mdPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return migrated, fmt.Errorf("read plan content %s: %w", filename, err)
			}
			// No .md file — that's fine, content stays empty.
		} else {
			if err := store.SetContent(project, filename, string(content)); err != nil {
				return migrated, fmt.Errorf("set content for %s: %w", filename, err)
			}
		}
	}

	// Migrate topics.
	for name, jt := range state.Topics {
		var createdAt time.Time
		if jt.CreatedAt != "" {
			createdAt = parseTime(jt.CreatedAt)
		}
		entry := TopicEntry{
			Name:      name,
			CreatedAt: createdAt,
		}
		if err := store.CreateTopic(project, entry); err != nil {
			// Skip if already exists (idempotent).
			if strings.Contains(err.Error(), "topic already exists") {
				continue
			}
			return migrated, fmt.Errorf("migrate topic %s: %w", name, err)
		}
	}

	return migrated, nil
}

// migrateFromPlanstoreDB copies data from a legacy planstore.db file into the
// current taskstore.db. This handles the rename-plan-to-task transition where
// the DB filename changed but existing users still have their data in the old
// file.
//
// The migration is idempotent: it only runs when planstore.db exists in the
// same directory as dbPath AND the tasks table in the current DB is empty.
// It copies tasks (from the plans table), topics, and audit_events.
func migrateFromPlanstoreDB(db *sql.DB, dbPath string) error {
	dir := filepath.Dir(dbPath)
	oldDBPath := filepath.Join(dir, "planstore.db")

	// Check if old DB exists.
	if _, err := os.Stat(oldDBPath); err != nil {
		return nil // no old DB — nothing to migrate
	}

	// Check if the new tasks table already has data (skip if so).
	var taskCount int
	if err := db.QueryRow("SELECT count(*) FROM tasks").Scan(&taskCount); err != nil {
		return nil // table might not exist yet — schema hasn't run; caller handles
	}
	if taskCount > 0 {
		return nil // already has data — don't overwrite
	}

	// Attach the old database and copy data.
	attachSQL := fmt.Sprintf("ATTACH DATABASE %q AS old", oldDBPath)
	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach planstore.db: %w", err)
	}
	defer db.Exec("DETACH DATABASE old") //nolint:errcheck

	// Copy plans → tasks (the old table is named "plans").
	if tableExistsInAttached(db, "old", "plans") {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO tasks (project, filename, status, description, branch, topic, created_at, implemented)
			SELECT project, filename, status, description, branch, topic, created_at, implemented
			FROM old.plans
		`); err != nil {
			return fmt.Errorf("copy plans to tasks: %w", err)
		}

		// Copy content column if it exists in the old table.
		if columnExistsInAttached(db, "old", "plans", "content") {
			if _, err := db.Exec(`
				UPDATE tasks SET content = (
					SELECT old.plans.content FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
				WHERE EXISTS (
					SELECT 1 FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
			`); err != nil {
				// Non-fatal — content is optional.
				_ = err
			}
		}

		// Copy clickup_task_id if it exists.
		if columnExistsInAttached(db, "old", "plans", "clickup_task_id") {
			if _, err := db.Exec(`
				UPDATE tasks SET clickup_task_id = (
					SELECT old.plans.clickup_task_id FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
				WHERE EXISTS (
					SELECT 1 FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
			`); err != nil {
				_ = err
			}
		}

		// Copy review_cycle if it exists.
		if columnExistsInAttached(db, "old", "plans", "review_cycle") {
			if _, err := db.Exec(`
				UPDATE tasks SET review_cycle = (
					SELECT old.plans.review_cycle FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
				WHERE EXISTS (
					SELECT 1 FROM old.plans
					WHERE old.plans.project = tasks.project AND old.plans.filename = tasks.filename
				)
			`); err != nil {
				_ = err
			}
		}
	}

	// Copy topics.
	if tableExistsInAttached(db, "old", "topics") {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO topics (project, name, created_at)
			SELECT project, name, created_at
			FROM old.topics
		`); err != nil {
			return fmt.Errorf("copy topics: %w", err)
		}
	}

	// Copy audit_events (the auditlog package shares the same DB file).
	if tableExistsInAttached(db, "old", "audit_events") {
		// Ensure audit_events table exists in the new DB.
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS audit_events (
				id             INTEGER PRIMARY KEY,
				kind           TEXT    NOT NULL,
				timestamp      TEXT    NOT NULL,
				project        TEXT    NOT NULL DEFAULT '',
				plan_file      TEXT    NOT NULL DEFAULT '',
				instance_title TEXT    NOT NULL DEFAULT '',
				agent_type     TEXT    NOT NULL DEFAULT '',
				wave_number    INTEGER NOT NULL DEFAULT 0,
				task_number    INTEGER NOT NULL DEFAULT 0,
				message        TEXT    NOT NULL DEFAULT '',
				detail         TEXT    NOT NULL DEFAULT '',
				level          TEXT    NOT NULL DEFAULT 'info'
			)
		`); err != nil {
			return fmt.Errorf("create audit_events table: %w", err)
		}

		if _, err := db.Exec(`
			INSERT INTO audit_events (kind, timestamp, project, plan_file, instance_title, agent_type, wave_number, task_number, message, detail, level)
			SELECT kind, timestamp, project, plan_file, instance_title, agent_type, wave_number, task_number, message, detail, level
			FROM old.audit_events
		`); err != nil {
			return fmt.Errorf("copy audit_events: %w", err)
		}
	}

	return nil
}

// tableExistsInAttached checks if a table exists in an attached database.
func tableExistsInAttached(db *sql.DB, schema, tableName string) bool {
	var count int
	err := db.QueryRow(
		fmt.Sprintf("SELECT count(*) FROM %s.sqlite_master WHERE type='table' AND name=?", schema),
		tableName,
	).Scan(&count)
	return err == nil && count > 0
}

// columnExistsInAttached checks if a column exists in a table in an attached database.
func columnExistsInAttached(db *sql.DB, schema, tableName, columnName string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, tableName))
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == columnName {
			return true
		}
	}
	return false
}
