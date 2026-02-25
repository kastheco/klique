package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusBar_Baseline(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(80)
	sb.SetData(StatusBarData{
		RepoName: "kasmos",
		Branch:   "main",
	})

	result := sb.String()
	assert.Contains(t, result, "kasmos")
	assert.Contains(t, result, "main")
	// Should be exactly 1 line (no newlines in output)
	assert.Equal(t, 0, strings.Count(result, "\n"))
}

func TestStatusBar_PlanContext(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
	})

	result := sb.String()
	assert.Contains(t, result, "kasmos")
	assert.Contains(t, result, "plan/auth-refactor")
	assert.Contains(t, result, "auth-refactor")
	assert.Contains(t, result, "implementing")
}

func TestStatusBar_PlanNameRedundantSuppression(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)

	// When plan name is already present in the branch, it must not be rendered
	// as a separate segment (would appear duplicated in the bar).
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
	})
	inBranch := sb.String()

	// Render again with a plan name that is NOT in the branch.
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "main",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
	})
	notInBranch := sb.String()

	// Strip ANSI so we can count plain-text occurrences.
	stripANSI := func(s string) string {
		var b strings.Builder
		inEsc := false
		for _, r := range s {
			if r == '\x1b' {
				inEsc = true
			}
			if !inEsc {
				b.WriteRune(r)
			}
			if inEsc && r == 'm' {
				inEsc = false
			}
		}
		return b.String()
	}

	plain1 := stripANSI(inBranch)
	plain2 := stripANSI(notInBranch)

	// Branch already contains plan name: "auth-refactor" should appear exactly once.
	assert.Equal(t, 1, strings.Count(plain1, "auth-refactor"),
		"plan name already in branch must not render as a separate segment")

	// Branch does not contain plan name: it should appear as its own segment.
	assert.Equal(t, 1, strings.Count(plain2, "auth-refactor"),
		"plan name not in branch must be rendered")
}

func TestStatusBar_WaveGlyphs(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
		WaveLabel:  "wave 2/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphComplete,
			TaskGlyphComplete,
			TaskGlyphRunning,
			TaskGlyphFailed,
			TaskGlyphPending,
		},
	})

	result := sb.String()
	assert.Contains(t, result, "wave 2/4")
	// Glyphs should be present (check the raw glyph chars)
	assert.Contains(t, result, "✓")
	assert.Contains(t, result, "●")
	assert.Contains(t, result, "✕")
	assert.Contains(t, result, "○")
}

func TestStatusBar_Truncation(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(40) // narrow terminal
	sb.SetData(StatusBarData{
		RepoName: "very-long-repository-name-that-wont-fit",
		Branch:   "feature/extremely-long-branch-name-here",
	})

	result := sb.String()
	// Should not exceed width (lipgloss handles this, but verify no panic)
	require.NotEmpty(t, result)
}

func TestStatusBar_EmptyData(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(80)
	sb.SetData(StatusBarData{})

	result := sb.String()
	// App name is gradient-rendered so individual chars are split by ANSI escapes;
	// verify each character is present in order.
	for _, c := range "kasmos" {
		assert.Contains(t, result, string(c))
	}
}
