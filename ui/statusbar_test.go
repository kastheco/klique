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
	// Should still render the app name
	assert.Contains(t, result, "kasmos")
}
