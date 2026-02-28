package auditlog

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

const auditSchema = `
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
);

CREATE INDEX IF NOT EXISTS idx_audit_project_ts ON audit_events(project, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_plan ON audit_events(plan_file, timestamp DESC);
`

const maxQueryLimit = 500

// SQLiteLogger is a Logger backed by a SQLite database.
type SQLiteLogger struct {
	db *sql.DB
}

// NewSQLiteLogger opens (or creates) a SQLite database at dbPath, runs the
// audit_events schema, and returns a ready-to-use logger.
// Use ":memory:" for an in-memory database (useful in tests).
func NewSQLiteLogger(dbPath string) (*SQLiteLogger, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db for audit log: %w", err)
	}

	if _, err := db.Exec(auditSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run audit log schema: %w", err)
	}

	return &SQLiteLogger{db: db}, nil
}

// Emit inserts an audit event into the database. If the event's Timestamp is
// zero, it is set to time.Now(). Emit is synchronous and safe to call from the
// bubbletea Update goroutine.
func (l *SQLiteLogger) Emit(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	const q = `
		INSERT INTO audit_events
			(kind, timestamp, project, plan_file, instance_title, agent_type,
			 wave_number, task_number, message, detail, level)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	level := e.Level
	if level == "" {
		level = "info"
	}

	_, _ = l.db.Exec(q,
		string(e.Kind),
		auditFormatTime(e.Timestamp),
		e.Project,
		e.PlanFile,
		e.InstanceTitle,
		e.AgentType,
		e.WaveNumber,
		e.TaskNumber,
		e.Message,
		e.Detail,
		level,
	)
}

// Query returns events matching the filter, ordered newest-first.
// Limit is capped at 500.
func (l *SQLiteLogger) Query(f QueryFilter) ([]Event, error) {
	limit := f.Limit
	if limit <= 0 || limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	var conditions []string
	var args []any

	if f.Project != "" {
		conditions = append(conditions, "project = ?")
		args = append(args, f.Project)
	}
	if f.PlanFile != "" {
		conditions = append(conditions, "plan_file = ?")
		args = append(args, f.PlanFile)
	}
	if f.InstanceTitle != "" {
		conditions = append(conditions, "instance_title = ?")
		args = append(args, f.InstanceTitle)
	}
	if len(f.Kinds) > 0 {
		placeholders := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			placeholders[i] = "?"
			args = append(args, string(k))
		}
		conditions = append(conditions, "kind IN ("+strings.Join(placeholders, ", ")+")")
	}
	if !f.After.IsZero() {
		conditions = append(conditions, "timestamp > ?")
		args = append(args, auditFormatTime(f.After))
	}
	if !f.Before.IsZero() {
		conditions = append(conditions, "timestamp < ?")
		args = append(args, auditFormatTime(f.Before))
	}

	q := `
		SELECT id, kind, timestamp, project, plan_file, instance_title,
		       agent_type, wave_number, task_number, message, detail, level
		FROM audit_events
	`
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT %d", limit)

	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var ts string
		if err := rows.Scan(
			&e.ID,
			(*string)(&e.Kind),
			&ts,
			&e.Project,
			&e.PlanFile,
			&e.InstanceTitle,
			&e.AgentType,
			&e.WaveNumber,
			&e.TaskNumber,
			&e.Message,
			&e.Detail,
			&e.Level,
		); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		e.Timestamp = auditParseTime(ts)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

// Close releases the database connection.
func (l *SQLiteLogger) Close() error {
	return l.db.Close()
}

// auditFormatTime formats a time.Time as RFC3339Nano for storage.
// Zero time returns empty string.
func auditFormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// auditParseTime parses an RFC3339Nano string.
// Returns zero time on empty or invalid input.
func auditParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
