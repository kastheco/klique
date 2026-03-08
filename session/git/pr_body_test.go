package git

import (
	"strings"
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

func TestBuildPRTitle_SingleSentenceUnderLimit(t *testing.T) {
	// Short single sentence: returned as-is (no period to strip, under 72 chars).
	assert.Equal(t, "Make kas check and kas skills sync actually useful", BuildPRTitle("Make kas check and kas skills sync actually useful", "check-cli"))
}

func TestBuildPRTitle_MultipleSentencesTruncatesToFirst(t *testing.T) {
	// Two-sentence description: only the first sentence is kept.
	desc := "Fix the login flow. Make sure tokens are refreshed on expiry."
	got := BuildPRTitle(desc, "fix-login")
	assert.Equal(t, "Fix the login flow", got)
}

func TestBuildPRTitle_LongFirstSentenceTruncatesAtWordBoundary(t *testing.T) {
	// First sentence > 72 chars: truncated at word boundary with "...".
	desc := "check-cli-useless the 'kas setup' and 'kas check' don't show you enough information. more detail here."
	got := BuildPRTitle(desc, "check-cli-useless")
	assert.LessOrEqual(t, len(got), maxTitleLen+3) // +3 for "..."
	assert.True(t, strings.HasSuffix(got, "..."), "expected ellipsis suffix, got: %q", got)
	// Must not contain the second sentence.
	assert.NotContains(t, got, "more detail here")
}

func TestBuildPRTitle_StripsTrailingPunctuation(t *testing.T) {
	assert.Equal(t, "Update authentication middleware", BuildPRTitle("Update authentication middleware.", "auth"))
}
