package taskstore

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

const signalsSchema = `
CREATE TABLE IF NOT EXISTS signals (
	id           INTEGER PRIMARY KEY,
	project      TEXT    NOT NULL DEFAULT '',
	plan_file    TEXT    NOT NULL DEFAULT '',
	signal_type  TEXT    NOT NULL DEFAULT '',
	payload      TEXT    NOT NULL DEFAULT '',
	status       TEXT    NOT NULL DEFAULT 'pending',
	created_at   TEXT    NOT NULL DEFAULT '',
	claimed_by   TEXT    NOT NULL DEFAULT '',
	claimed_at   TEXT    NOT NULL DEFAULT '',
	processed_at TEXT    NOT NULL DEFAULT '',
	result       TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_signals_project_status_created_at
	ON signals(project, status, created_at, id);
`

// SQLiteSignalGateway is a SignalGateway backed by a SQLite database.
type SQLiteSignalGateway struct {
	db *sql.DB
}

// NewSQLiteSignalGateway opens (or creates) a SQLite database at dbPath and
// initialises the signals schema. Use ":memory:" for an in-memory database.
func NewSQLiteSignalGateway(dbPath string) (*SQLiteSignalGateway, error) {
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

	if _, err := db.Exec(signalsSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create signals schema: %w", err)
	}

	return &SQLiteSignalGateway{db: db}, nil
}

// Create inserts a new pending signal for the given project.
func (g *SQLiteSignalGateway) Create(project string, entry SignalEntry) error {
	const q = `
		INSERT INTO signals (project, plan_file, signal_type, payload, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)
	`
	_, err := g.db.Exec(q, project, entry.PlanFile, entry.SignalType, entry.Payload, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("create signal: %w", err)
	}
	return nil
}

// List returns all signals for the given project matching any of the provided
// statuses. Returns nil, nil when no statuses are provided.
func (g *SQLiteSignalGateway) List(project string, statuses ...SignalStatus) ([]SignalEntry, error) {
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
		SELECT id, project, plan_file, signal_type, payload, status,
		       created_at, claimed_by, claimed_at, processed_at, result
		FROM signals
		WHERE project = ? AND status IN (%s)
		ORDER BY created_at ASC, id ASC
	`, strings.Join(placeholders, ", "))

	rows, err := g.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list signals: %w", err)
	}
	defer rows.Close()
	return scanSignalEntries(rows)
}

// Claim atomically claims the oldest pending signal for the given project.
// Returns nil, nil when no pending signal is available.
func (g *SQLiteSignalGateway) Claim(project, claimedBy string) (*SignalEntry, error) {
	tx, err := g.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("claim signal: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	claimedAt := formatTime(time.Now().UTC())

	const updateQ = `
		UPDATE signals
		SET status = 'processing', claimed_by = ?, claimed_at = ?
		WHERE id = (
			SELECT id FROM signals
			WHERE project = ? AND status = 'pending'
			ORDER BY created_at ASC, id ASC
			LIMIT 1
		)
	`
	res, err := tx.Exec(updateQ, claimedBy, claimedAt, project)
	if err != nil {
		return nil, fmt.Errorf("claim signal: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("claim signal: rows affected: %w", err)
	}
	if n == 0 {
		return nil, nil //nolint:nilnil
	}

	const selectQ = `
		SELECT id, project, plan_file, signal_type, payload, status,
		       created_at, claimed_by, claimed_at, processed_at, result
		FROM signals
		WHERE project = ? AND status = 'processing' AND claimed_by = ? AND claimed_at = ?
		ORDER BY claimed_at ASC, id ASC
		LIMIT 1
	`
	row := tx.QueryRow(selectQ, project, claimedBy, claimedAt)
	entry, err := scanSignalEntry(row)
	if err != nil {
		return nil, fmt.Errorf("claim signal: re-select: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("claim signal: commit: %w", err)
	}
	return entry, nil
}

// MarkProcessed sets the final status, result, and processed_at on a signal.
func (g *SQLiteSignalGateway) MarkProcessed(id int64, status SignalStatus, result string) error {
	const q = `
		UPDATE signals
		SET status = ?, result = ?, processed_at = ?
		WHERE id = ?
	`
	res, err := g.db.Exec(q, string(status), result, formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("mark processed signal %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark processed signal %d: rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("signal not found: %d", id)
	}
	return nil
}

// ResetStuck resets signals stuck in "processing" for longer than olderThan,
// returning them to "pending" so they can be reclaimed.
func (g *SQLiteSignalGateway) ResetStuck(olderThan time.Duration) (int, error) {
	cutoff := formatTime(time.Now().UTC().Add(-olderThan))
	const q = `
		UPDATE signals
		SET status = 'pending', claimed_by = '', claimed_at = ''
		WHERE status = 'processing' AND claimed_at < ? AND claimed_at != ''
	`
	res, err := g.db.Exec(q, cutoff)
	if err != nil {
		return 0, fmt.Errorf("reset stuck signals: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reset stuck signals: rows affected: %w", err)
	}
	return int(n), nil
}

// BackdateClaimedAt is an exported test helper that sets claimed_at to
// time.Now().UTC().Add(-age) for the signal with the given ID.
func (g *SQLiteSignalGateway) BackdateClaimedAt(id int64, age time.Duration) error {
	backdated := formatTime(time.Now().UTC().Add(-age))
	const q = `UPDATE signals SET claimed_at = ? WHERE id = ?`
	res, err := g.db.Exec(q, backdated, id)
	if err != nil {
		return fmt.Errorf("backdate claimed_at for signal %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("backdate claimed_at for signal %d: rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("signal not found: %d", id)
	}
	return nil
}

// Close releases the underlying database connection.
func (g *SQLiteSignalGateway) Close() error {
	return g.db.Close()
}

// scanSignalEntries scans all rows into a slice of SignalEntry.
func scanSignalEntries(rows *sql.Rows) ([]SignalEntry, error) {
	var entries []SignalEntry
	for rows.Next() {
		var (
			id                                                   int64
			project, planFile, signalType, payload, status       string
			createdAt, claimedBy, claimedAt, processedAt, result string
		)
		if err := rows.Scan(
			&id, &project, &planFile, &signalType, &payload, &status,
			&createdAt, &claimedBy, &claimedAt, &processedAt, &result,
		); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		entries = append(entries, SignalEntry{
			ID:          id,
			Project:     project,
			PlanFile:    planFile,
			SignalType:  signalType,
			Payload:     payload,
			Status:      SignalStatus(status),
			CreatedAt:   parseTime(createdAt),
			ClaimedBy:   claimedBy,
			ClaimedAt:   parseTime(claimedAt),
			ProcessedAt: parseTime(processedAt),
			Result:      result,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate signals: %w", err)
	}
	return entries, nil
}

// scanSignalEntry scans a single row into a SignalEntry.
func scanSignalEntry(row *sql.Row) (*SignalEntry, error) {
	var (
		id                                                   int64
		project, planFile, signalType, payload, status       string
		createdAt, claimedBy, claimedAt, processedAt, result string
	)
	if err := row.Scan(
		&id, &project, &planFile, &signalType, &payload, &status,
		&createdAt, &claimedBy, &claimedAt, &processedAt, &result,
	); err != nil {
		return nil, fmt.Errorf("scan signal row: %w", err)
	}
	return &SignalEntry{
		ID:          id,
		Project:     project,
		PlanFile:    planFile,
		SignalType:  signalType,
		Payload:     payload,
		Status:      SignalStatus(status),
		CreatedAt:   parseTime(createdAt),
		ClaimedBy:   claimedBy,
		ClaimedAt:   parseTime(claimedAt),
		ProcessedAt: parseTime(processedAt),
		Result:      result,
	}, nil
}
