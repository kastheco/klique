package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stripANSI(s string) string {
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
	assert.Contains(t, result, "implementing")
}

func TestStatusBar_StatusLeftAlignedAfterLogo(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
	})
	plain := stripANSI(sb.String())

	appIdx := strings.Index(plain, "kasmos")
	branchIdx := strings.Index(plain, "plan/auth-refactor")
	statusIdx := strings.Index(plain, "implementing")
	repoIdx := strings.LastIndex(plain, "kasmos")

	require.NotEqual(t, -1, appIdx)
	require.NotEqual(t, -1, branchIdx)
	require.NotEqual(t, -1, statusIdx)
	require.NotEqual(t, -1, repoIdx)
	assert.Greater(t, statusIdx, appIdx, "status must appear after app logo")
	assert.Greater(t, branchIdx, statusIdx, "branch should no longer be grouped with status")
	assert.Greater(t, repoIdx, statusIdx, "repo name should be positioned to the right")
}

func TestStatusBar_BranchGroupCenteredAndRepoRightAligned(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:   "my-repo",
		Branch:     "main",
		PlanStatus: "reviewing",
	})

	plain := stripANSI(sb.String())

	trimmedRight := strings.TrimRight(plain, " ")
	assert.True(t, strings.HasSuffix(trimmedRight, "my-repo"),
		"repo name should be right-aligned at end of status bar")

	branchIdx := strings.Index(plain, "main")
	require.NotEqual(t, -1, branchIdx)
	branchCenter := branchIdx + len("main")/2
	assert.InDelta(t, 50, branchCenter, 6,
		"branch group should be centered in the status bar")
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

func TestStatusBar_WaveGlyphsAreSpaced(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:  "kasmos",
		Branch:    "plan/auth-refactor",
		WaveLabel: "wave 1/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphRunning,
			TaskGlyphPending,
			TaskGlyphPending,
		},
	})

	plain := stripANSI(sb.String())
	assert.Contains(t, plain, "● ○ ○",
		"wave task glyphs should have spacing to avoid cramped status bar rendering")
}

func TestStatusBar_WaveGlyphsAppearBeforeWaveLabel(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:  "kasmos",
		Branch:    "plan/auth-refactor",
		WaveLabel: "wave 1/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphRunning,
			TaskGlyphPending,
			TaskGlyphPending,
		},
	})

	plain := stripANSI(sb.String())
	glyphIdx := strings.Index(plain, "● ○ ○")
	labelIdx := strings.Index(plain, "wave 1/4")
	require.NotEqual(t, -1, glyphIdx)
	require.NotEqual(t, -1, labelIdx)
	assert.Less(t, glyphIdx, labelIdx,
		"wave glyphs should appear before the wave label in the status bar")
}

func TestStatusBar_WaveProgressLeftAlignedAfterLogo(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:  "kasmos",
		Branch:    "plan/auth-refactor",
		WaveLabel: "wave 1/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphRunning,
			TaskGlyphPending,
			TaskGlyphPending,
		},
	})

	plain := stripANSI(sb.String())
	appIdx := strings.Index(plain, "kasmos")
	waveIdx := strings.Index(plain, "wave 1/4")
	branchIdx := strings.Index(plain, "plan/auth-refactor")

	require.NotEqual(t, -1, appIdx)
	require.NotEqual(t, -1, waveIdx)
	require.NotEqual(t, -1, branchIdx)
	assert.Greater(t, waveIdx, appIdx, "wave progress should appear after app logo")
	assert.Greater(t, branchIdx, waveIdx, "branch should remain centered, not grouped with wave progress")
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

func TestStatusBar_TmuxSessionCount(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "main",
		TmuxSessionCount: 3,
	})

	plain := stripANSI(sb.String())
	assert.Contains(t, plain, "tmux:3")
}

func TestStatusBar_TmuxSessionCountZeroHidden(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "main",
		TmuxSessionCount: 0,
	})

	plain := stripANSI(sb.String())
	assert.NotContains(t, plain, "tmux:")
}

func TestStatusBar_TmuxSessionCountWithPlanStatus(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "feat/auth",
		PlanStatus:       "implementing",
		TmuxSessionCount: 5,
	})

	plain := stripANSI(sb.String())
	assert.Contains(t, plain, "implementing")
	assert.Contains(t, plain, "tmux:5")
}

func TestStatusBar_FocusModeNoLongerShowsPill(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:  "kasmos",
		Branch:    "main",
		FocusMode: true,
	})

	result := sb.String()
	assert.NotContains(t, result, "interactive",
		"interactive indicator moved to bottom menu bar")
}
