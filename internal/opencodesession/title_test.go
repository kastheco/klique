package opencodesession

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		name string
		opts TitleOpts
		want string
	}{
		{
			name: "planner session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "planner"},
			want: "kas: plan automatic-session-naming",
		},
		{
			name: "coder session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "coder"},
			want: "kas: implement automatic-session-naming",
		},
		{
			name: "reviewer session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "reviewer"},
			want: "kas: review automatic-session-naming",
		},
		{
			name: "wave task session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "coder", WaveNumber: 2, TaskNumber: 3},
			want: "kas: implement automatic-session-naming w2/t3",
		},
		{
			name: "fixer session",
			opts: TitleOpts{InstanceTitle: "fix-login-bug", AgentType: "fixer"},
			want: "kas: fix fix-login-bug",
		},
		{
			name: "ad-hoc session no agent type",
			opts: TitleOpts{InstanceTitle: "my-session"},
			want: "kas: my-session",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTitle(tt.opts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE session (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		title TEXT NOT NULL,
		directory TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	return db
}

func TestClaimAndSetTitle_SingleSession(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_1', 'proj_1', 'auto-generated garbage', '/work/dir', ?, ?)`, now, now)
	require.NoError(t, err)

	beforeStart := time.UnixMilli(now - 100)
	err = ClaimAndSetTitle(db, "/work/dir", beforeStart, "kas: plan my-feature")
	require.NoError(t, err)

	var title string
	err = db.QueryRow(`SELECT title FROM session WHERE id = 'ses_1'`).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "kas: plan my-feature", title)
}

func TestClaimAndSetTitle_SkipsAlreadyClaimed(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	// First session already claimed by kasmos
	_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_1', 'proj_1', 'kas: implement other-plan', '/work/dir', ?, ?)`, now-50, now-50)
	require.NoError(t, err)
	// Second session unclaimed
	_, err = db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_2', 'proj_1', 'I will load skills first', '/work/dir', ?, ?)`, now, now)
	require.NoError(t, err)

	beforeStart := time.UnixMilli(now - 100)
	err = ClaimAndSetTitle(db, "/work/dir", beforeStart, "kas: implement my-feature w1/t2")
	require.NoError(t, err)

	var title string
	err = db.QueryRow(`SELECT title FROM session WHERE id = 'ses_2'`).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "kas: implement my-feature w1/t2", title)
}

func TestClaimAndSetTitle_NoMatchReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	// No sessions in DB
	err := ClaimAndSetTitle(db, "/work/dir", time.Now(), "kas: plan foo")
	assert.NoError(t, err) // best-effort, no error on miss
}

func TestClaimAndSetTitle_ParallelClaims(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	// Three sessions created nearly simultaneously (simulating parallel wave tasks)
	for i, id := range []string{"ses_1", "ses_2", "ses_3"} {
		_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
			VALUES (?, 'proj_1', 'garbage title', '/shared/worktree', ?, ?)`,
			id, now+int64(i*100), now+int64(i*100))
		require.NoError(t, err)
	}

	beforeStart := time.UnixMilli(now - 50)

	// Simulate three goroutines claiming sequentially (real parallelism tested by SQLite's single-writer lock)
	err := ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t1")
	require.NoError(t, err)
	err = ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t2")
	require.NoError(t, err)
	err = ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t3")
	require.NoError(t, err)

	// Each session should have a unique title
	titles := make(map[string]bool)
	rows, err := db.Query(`SELECT title FROM session WHERE directory = '/shared/worktree' ORDER BY time_created ASC`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var title string
		require.NoError(t, rows.Scan(&title))
		titles[title] = true
	}
	assert.Len(t, titles, 3)
	assert.True(t, titles["kas: implement plan w1/t1"])
	assert.True(t, titles["kas: implement plan w1/t2"])
	assert.True(t, titles["kas: implement plan w1/t3"])
}
