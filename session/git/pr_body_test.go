package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildPRBody_FullMetadata(t *testing.T) {
	meta := PRMetadata{
		Goal:         "add authentication middleware to all API routes",
		Description:  "Auth Middleware",
		Architecture: "Express middleware chain with JWT validation",
		TechStack:    "Go, net/http, jwt-go",
		ReviewCycle:  2,
		Subtasks: []PRSubtask{
			{Number: 1, Title: "JWT validation middleware", Status: "complete"},
			{Number: 2, Title: "Route integration", Status: "complete"},
		},
	}
	body := BuildPRBody(meta)

	assert.Contains(t, body, "## summary")
	assert.Contains(t, body, "add authentication middleware to all API routes")
	assert.Contains(t, body, "Auth Middleware")
	assert.Contains(t, body, "Express middleware chain with JWT validation")
	assert.Contains(t, body, "Go, net/http, jwt-go")
	assert.Contains(t, body, "JWT validation middleware")
	assert.Contains(t, body, "Route integration")
	assert.Contains(t, body, "review cycle: 2")
	assert.Contains(t, body, "- [x] 1. JWT validation middleware")
	assert.Contains(t, body, "- [x] 2. Route integration")
}

func TestBuildPRBody_MinimalMetadata(t *testing.T) {
	meta := PRMetadata{
		Description: "quick fix",
	}
	body := BuildPRBody(meta)

	assert.Contains(t, body, "quick fix")
	assert.NotContains(t, body, "**goal:**")
	assert.NotContains(t, body, "## tasks")
}

func TestBuildPRBody_WithGitSections(t *testing.T) {
	meta := PRMetadata{
		Goal:       "fix bug",
		GitChanges: "file1.go\nfile2.go",
		GitCommits: "abc1234 fix: resolve nil pointer\ndef5678 test: add regression test",
		GitStats:   "2 files changed, 15 insertions(+), 3 deletions(-)",
	}
	body := BuildPRBody(meta)

	assert.Contains(t, body, "file1.go")
	assert.Contains(t, body, "fix: resolve nil pointer")
	assert.Contains(t, body, "2 files changed")
	assert.Contains(t, body, "## changes")
	assert.Contains(t, body, "## commits")
	assert.Contains(t, body, "## stats")
}

func TestBuildPRBody_ReviewerSummary(t *testing.T) {
	meta := PRMetadata{
		Description:     "feature x",
		ReviewerSummary: "all tests pass, code is clean, approved.",
	}
	body := BuildPRBody(meta)

	assert.Contains(t, body, "all tests pass, code is clean, approved.")
	assert.Contains(t, body, "## reviewer notes")
}

func TestBuildPRTitle_UsesDescription(t *testing.T) {
	assert.Equal(t, "Auth Middleware", BuildPRTitle("Auth Middleware", "auth-middleware"))
}

func TestBuildPRTitle_FallsBackToSlug(t *testing.T) {
	assert.Equal(t, "auth-middleware", BuildPRTitle("", "auth-middleware"))
}

func TestBuildPRTitle_EmptyBoth(t *testing.T) {
	title := BuildPRTitle("", "")
	assert.NotEmpty(t, title)
	assert.Equal(t, "update", title)
}
