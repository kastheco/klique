// Package opencodesession manages opencode session titles for kasmos-spawned agents.
package opencodesession

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

// kasPrefix is the prefix applied to all kasmos-managed session titles.
// It distinguishes our titles from opencode's auto-generated ones.
const kasPrefix = "kas: "

// TitleOpts holds the metadata used to build a human-readable session title.
type TitleOpts struct {
	// PlanName is the plan slug (e.g. "automatic-session-naming").
	PlanName string
	// AgentType is the role of the agent (e.g. "planner", "coder", "reviewer", "fixer").
	AgentType string
	// WaveNumber and TaskNumber, when non-zero, append wave/task context to the title.
	WaveNumber int
	TaskNumber int
	// InstanceTitle is used as the subject for ad-hoc sessions or when PlanName is empty.
	InstanceTitle string
	// ReviewCycle is the 1-indexed review cycle number. When non-zero and AgentType is
	// "reviewer", the title includes "#N" to disambiguate multiple review rounds.
	ReviewCycle int
}

// agentVerbs maps agent type identifiers to their title verb prefix.
var agentVerbs = map[string]string{
	"planner":  "plan",
	"coder":    "implement",
	"reviewer": "review",
	"fixer":    "fix",
}

// BuildTitle returns a kasmos-managed title string for an opencode session.
// All titles are prefixed with "kas: " to distinguish them from opencode's
// auto-generated titles.
func BuildTitle(opts TitleOpts) string {
	// Determine the subject — prefer PlanName, fall back to InstanceTitle.
	subject := opts.PlanName
	if subject == "" {
		subject = opts.InstanceTitle
	}

	// Look up the verb for the agent type.
	verb, hasVerb := agentVerbs[opts.AgentType]

	// Ad-hoc session with no recognized agent type: just prefix the subject.
	if !hasVerb {
		return kasPrefix + subject
	}

	// For reviewers with a non-zero cycle, include the cycle number in the title.
	var title string
	if opts.AgentType == "reviewer" && opts.ReviewCycle > 0 {
		title = fmt.Sprintf("%sreview #%d %s", kasPrefix, opts.ReviewCycle, subject)
	} else {
		title = fmt.Sprintf("%s%s %s", kasPrefix, verb, subject)
	}

	// Append wave/task context when both are specified.
	if opts.WaveNumber > 0 && opts.TaskNumber > 0 {
		title += fmt.Sprintf(" w%d/t%d", opts.WaveNumber, opts.TaskNumber)
	}

	return title
}

// ClaimAndSetTitle atomically claims the first unclaimed opencode session that
// matches the given working directory and was created after beforeStart, then
// sets its title to the provided value.
//
// "Unclaimed" means the title does NOT start with "kas: ". This prevents two
// concurrent goroutines from claiming the same session in a parallel wave.
//
// It is a best-effort operation: if no matching session is found, it returns nil.
func ClaimAndSetTitle(db *sql.DB, workDir string, beforeStart time.Time, title string) error {
	beforeStartMs := beforeStart.UnixMilli()

	// Find all unclaimed sessions in this directory created after beforeStart.
	rows, err := db.Query(
		`SELECT id FROM session
		 WHERE directory = ? AND time_created >= ? AND title NOT LIKE 'kas: %'
		 ORDER BY time_created ASC`,
		workDir, beforeStartMs,
	)
	if err != nil {
		return fmt.Errorf("opencodesession: query sessions: %w", err)
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("opencodesession: scan session id: %w", err)
		}
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("opencodesession: iterate sessions: %w", err)
	}
	rows.Close() // Close before attempting updates.

	nowMs := time.Now().UnixMilli()

	// Attempt an atomic claim on each candidate in ascending time_created order.
	for _, id := range candidates {
		res, err := db.Exec(
			`UPDATE session SET title = ?, time_updated = ?
			 WHERE id = ? AND title NOT LIKE 'kas: %'`,
			title, nowMs, id,
		)
		if err != nil {
			return fmt.Errorf("opencodesession: update session title: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("opencodesession: rows affected: %w", err)
		}
		if affected == 1 {
			// Successfully claimed this session.
			return nil
		}
		// Another goroutine claimed this candidate; try the next one.
	}

	// No candidates remain — best-effort, not an error.
	return nil
}

// resolveDBPath returns the path to the opencode SQLite database.
func resolveDBPath() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative path as last resort.
		return filepath.Join(".local", "share", "opencode", "opencode.db")
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

// SetTitleDirect opens the opencode SQLite DB and claims+sets the given
// pre-built title on the matching session. Use this when the title has already
// been constructed (e.g. via BuildTitle) and you only need the DB write.
//
// DB path resolution order:
//  1. $XDG_DATA_HOME/opencode/opencode.db
//  2. ~/.local/share/opencode/opencode.db
func SetTitleDirect(workDir string, beforeStart time.Time, title string) error {
	dbPath := resolveDBPath()
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return fmt.Errorf("opencodesession: open db %s: %w", dbPath, err)
	}
	defer db.Close()
	return ClaimAndSetTitle(db, workDir, beforeStart, title)
}
