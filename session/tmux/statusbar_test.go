package tmux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- tmuxFG primitive ---

func TestTmuxFG_WrapsThenTerminates(t *testing.T) {
	result := tmuxFG("colour45", "hello")
	assert.Equal(t, "#[fg=colour45]hello#[default]", result)
}

func TestTmuxFG_EmptyText(t *testing.T) {
	result := tmuxFG("colour45", "")
	assert.Equal(t, "#[fg=colour45]#[default]", result)
}

// --- tmuxStatusColor ---

func TestTmuxStatusColor_Implementing(t *testing.T) {
	result := tmuxStatusColor("implementing")
	assert.Equal(t, "#[fg="+tmuxColorFoam+"]implementing#[default]", result)
}

func TestTmuxStatusColor_Planning(t *testing.T) {
	result := tmuxStatusColor("planning")
	assert.Equal(t, "#[fg="+tmuxColorFoam+"]planning#[default]", result)
}

func TestTmuxStatusColor_Reviewing(t *testing.T) {
	result := tmuxStatusColor("reviewing")
	assert.Equal(t, "#[fg="+tmuxColorRose+"]reviewing#[default]", result)
}

func TestTmuxStatusColor_Done(t *testing.T) {
	result := tmuxStatusColor("done")
	assert.Equal(t, "#[fg="+tmuxColorRose+"]done#[default]", result)
}

func TestTmuxStatusColor_Ready(t *testing.T) {
	result := tmuxStatusColor("ready")
	assert.Equal(t, "#[fg="+tmuxColorMuted+"]ready#[default]", result)
}

func TestTmuxStatusColor_Cancelled(t *testing.T) {
	result := tmuxStatusColor("cancelled")
	assert.Equal(t, "#[fg="+tmuxColorMuted+"]cancelled#[default]", result)
}

// --- tmuxTaskGlyph ---

func TestTmuxTaskGlyph_Complete(t *testing.T) {
	result := tmuxTaskGlyph(TaskGlyphComplete)
	assert.Equal(t, "#[fg="+tmuxColorFoam+"]✓#[default]", result)
}

func TestTmuxTaskGlyph_Running(t *testing.T) {
	result := tmuxTaskGlyph(TaskGlyphRunning)
	assert.Equal(t, "#[fg="+tmuxColorIris+"]●#[default]", result)
}

func TestTmuxTaskGlyph_Failed(t *testing.T) {
	result := tmuxTaskGlyph(TaskGlyphFailed)
	assert.Equal(t, "#[fg="+tmuxColorLove+"]✕#[default]", result)
}

func TestTmuxTaskGlyph_Pending(t *testing.T) {
	result := tmuxTaskGlyph(TaskGlyphPending)
	assert.Equal(t, "#[fg="+tmuxColorMuted+"]○#[default]", result)
}

func TestTmuxTaskGlyph_Unknown_ReturnsEmpty(t *testing.T) {
	result := tmuxTaskGlyph(TaskGlyph(99))
	assert.Equal(t, "", result)
}

// --- tmuxPRGroup ---

func TestTmuxPRGroup_Empty_NoPR(t *testing.T) {
	result := tmuxPRGroup(StatusBarData{})
	assert.Equal(t, "", result)
}

func TestTmuxPRGroup_Approved(t *testing.T) {
	result := tmuxPRGroup(StatusBarData{PRState: "approved", PRChecks: "passing"})
	assert.Equal(t, "#[fg="+tmuxColorFoam+"]✓ pr#[default]", result)
}

func TestTmuxPRGroup_ChangesRequested(t *testing.T) {
	result := tmuxPRGroup(StatusBarData{PRState: "changes_requested"})
	assert.Equal(t, "#[fg="+tmuxColorRose+"]● pr#[default]", result)
}

func TestTmuxPRGroup_FailingChecks_OverridesApproved(t *testing.T) {
	// Failing checks must be the strongest signal regardless of review state.
	result := tmuxPRGroup(StatusBarData{PRState: "approved", PRChecks: "failing"})
	assert.Equal(t, "#[fg="+tmuxColorLove+"]✕ pr#[default]", result)
	assert.NotContains(t, result, "✓")
}

func TestTmuxPRGroup_FailingChecks_OverridesChangesRequested(t *testing.T) {
	result := tmuxPRGroup(StatusBarData{PRState: "changes_requested", PRChecks: "failing"})
	assert.Equal(t, "#[fg="+tmuxColorLove+"]✕ pr#[default]", result)
}

func TestTmuxPRGroup_Pending(t *testing.T) {
	result := tmuxPRGroup(StatusBarData{PRState: "pending"})
	assert.Equal(t, "#[fg="+tmuxColorMuted+"]○ pr#[default]", result)
}

// --- RenderStatusBar left segment ---

func TestRenderStatusBar_AppNameAlwaysPresent(t *testing.T) {
	result := RenderStatusBar(StatusBarData{})
	assert.Contains(t, result.Left, "kasmos")
	assert.Contains(t, result.Left, "#[bold]")
	assert.Contains(t, result.Left, "#[default]")
}

func TestRenderStatusBar_EmptyData_LeftIsJustAppName(t *testing.T) {
	result := RenderStatusBar(StatusBarData{})
	assert.Equal(t, "#[bold]kasmos#[default]", result.Left)
}

func TestRenderStatusBar_EmptyData_RightIsEmpty(t *testing.T) {
	result := RenderStatusBar(StatusBarData{})
	assert.Equal(t, "", result.Right)
}

func TestRenderStatusBar_Version_AppearsInLeft(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Version: "v1.4.2"})
	assert.Contains(t, result.Left, "v1.4.2")
	assert.Contains(t, result.Left, tmuxColorMuted)
}

func TestRenderStatusBar_Version_AfterAppName(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Version: "v2.0.0"})
	appIdx := strings.Index(result.Left, "kasmos")
	verIdx := strings.Index(result.Left, "v2.0.0")
	assert.Greater(t, verIdx, appIdx, "version must appear after app name")
}

func TestRenderStatusBar_VersionAndStatus_ExactShape(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Version:    "v1.0.0",
		PlanStatus: "ready",
	})
	want := "#[bold]kasmos#[default] " +
		tmuxFG(tmuxColorMuted, "v1.0.0") +
		" · " +
		tmuxFG(tmuxColorMuted, "ready")
	assert.Equal(t, want, result.Left)
}

func TestRenderStatusBar_PlanStatus_Implementing(t *testing.T) {
	result := RenderStatusBar(StatusBarData{PlanStatus: "implementing"})
	assert.Contains(t, result.Left, "implementing")
	assert.Contains(t, result.Left, tmuxColorFoam)
	assert.Contains(t, result.Left, " · ")
}

func TestRenderStatusBar_PlanStatus_Reviewing(t *testing.T) {
	result := RenderStatusBar(StatusBarData{PlanStatus: "reviewing"})
	assert.Contains(t, result.Left, "reviewing")
	assert.Contains(t, result.Left, tmuxColorRose)
}

func TestRenderStatusBar_PlanStatus_Ready(t *testing.T) {
	result := RenderStatusBar(StatusBarData{PlanStatus: "ready"})
	assert.Contains(t, result.Left, "ready")
	assert.Contains(t, result.Left, tmuxColorMuted)
}

func TestRenderStatusBar_WaveGlyphs_AllFour(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		WaveLabel: "wave 2/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphComplete, TaskGlyphRunning, TaskGlyphFailed, TaskGlyphPending,
		},
	})
	assert.Contains(t, result.Left, "✓")
	assert.Contains(t, result.Left, "●")
	assert.Contains(t, result.Left, "✕")
	assert.Contains(t, result.Left, "○")
	assert.Contains(t, result.Left, "wave 2/4")
	assert.Contains(t, result.Left, tmuxColorSubtle)
}

func TestRenderStatusBar_WaveGlyphs_AreSpaceSeparated(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		WaveLabel:  "wave 1/4",
		TaskGlyphs: []TaskGlyph{TaskGlyphRunning, TaskGlyphPending, TaskGlyphPending},
	})
	// Each glyph is #[fg=...]GLYPH#[default], joined with " ". After stripping markup
	// the visible text should contain "● ○ ○".
	// Verify the raw format string contains the glyphs in the right relative order.
	runIdx := strings.Index(result.Left, "●")
	pendIdx := strings.Index(result.Left, "○")
	assert.Greater(t, pendIdx, runIdx, "pending glyph must appear after running glyph")
}

func TestRenderStatusBar_WaveGlyphs_LabelAfterGlyphs(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		WaveLabel:  "wave 1/3",
		TaskGlyphs: []TaskGlyph{TaskGlyphRunning},
	})
	glyphIdx := strings.Index(result.Left, "●")
	labelIdx := strings.Index(result.Left, "wave 1/3")
	assert.Greater(t, labelIdx, glyphIdx, "wave label must appear after glyphs")
}

func TestRenderStatusBar_WaveOverridesPlanStatus(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		PlanStatus: "implementing",
		WaveLabel:  "wave 1/3",
		TaskGlyphs: []TaskGlyph{TaskGlyphRunning},
	})
	assert.Contains(t, result.Left, "wave 1/3")
	// Plan status text must not appear when wave info is present.
	assert.NotContains(t, result.Left, "implementing")
}

func TestRenderStatusBar_WaveLabel_NoGlyphs_FallsBackToPlanStatus(t *testing.T) {
	// WaveLabel set but no TaskGlyphs — falls back to PlanStatus.
	result := RenderStatusBar(StatusBarData{
		PlanStatus: "planning",
		WaveLabel:  "wave 1/3",
		TaskGlyphs: nil,
	})
	assert.Contains(t, result.Left, "planning")
	assert.NotContains(t, result.Left, "wave 1/3")
}

// --- Separator collapse tests ---

func TestRenderStatusBar_NoDoubledSeparators_NoVersion(t *testing.T) {
	result := RenderStatusBar(StatusBarData{PlanStatus: "implementing"})
	assert.NotContains(t, result.Left, " ·  · ")
}

func TestRenderStatusBar_NoDoubledSeparators_NoStatus(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Version: "v1.0.0"})
	count := strings.Count(result.Left, " · ")
	assert.LessOrEqual(t, count, 1)
}

func TestRenderStatusBar_NoSeparatorWhenNoStatus(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Version: "v1.0.0"})
	assert.NotContains(t, result.Left, " · ")
}

// --- RenderStatusBar right segment ---

func TestRenderStatusBar_BranchInRight(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Branch: "feature/nav-redesign"})
	assert.Contains(t, result.Right, "feature/nav-redesign")
}

func TestRenderStatusBar_ProjectDirInRight(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:     "main",
		ProjectDir: "kasmos",
	})
	assert.Contains(t, result.Right, "kasmos")
	assert.Contains(t, result.Right, tmuxColorMuted)
}

func TestRenderStatusBar_NoSeparatorWithSingleRightField(t *testing.T) {
	result := RenderStatusBar(StatusBarData{Branch: "main"})
	assert.NotContains(t, result.Right, " · ")
}

func TestRenderStatusBar_RightTwoFields_OneSeparator(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:  "main",
		PRState: "approved",
	})
	assert.Equal(t, 1, strings.Count(result.Right, " · "))
}

func TestRenderStatusBar_RightThreeFields_TwoSeparators(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:     "main",
		PRState:    "approved",
		ProjectDir: "kasmos",
	})
	assert.Equal(t, 2, strings.Count(result.Right, " · "))
}

func TestRenderStatusBar_PRGroup_ApprovedInRight(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:   "main",
		PRState:  "approved",
		PRChecks: "passing",
	})
	assert.Contains(t, result.Right, "✓ pr")
	assert.Contains(t, result.Right, tmuxColorFoam)
}

func TestRenderStatusBar_PRGroup_ChangesRequestedInRight(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:  "plan/test",
		PRState: "changes_requested",
	})
	assert.Contains(t, result.Right, "● pr")
	assert.Contains(t, result.Right, tmuxColorRose)
}

func TestRenderStatusBar_PRGroup_FailingChecksInRight(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:   "plan/test",
		PRState:  "approved",
		PRChecks: "failing",
	})
	assert.Contains(t, result.Right, "✕ pr")
	assert.Contains(t, result.Right, tmuxColorLove)
	assert.NotContains(t, result.Right, "✓ pr")
}

func TestRenderStatusBar_PRGroup_EmptyWhenNoPRState(t *testing.T) {
	result := RenderStatusBar(StatusBarData{
		Branch:     "main",
		ProjectDir: "myproject",
	})
	assert.NotContains(t, result.Right, "✓ pr")
	assert.NotContains(t, result.Right, "● pr")
	assert.NotContains(t, result.Right, "✕ pr")
	assert.NotContains(t, result.Right, "○ pr")
}

// --- Style leak prevention ---

func TestRenderStatusBar_AllColorSegmentsTerminated(t *testing.T) {
	// Every #[fg=...] and #[bold] must be paired with a #[default].
	result := RenderStatusBar(StatusBarData{
		Version:    "v2.0.0",
		PlanStatus: "implementing",
		Branch:     "main",
		PRState:    "approved",
		ProjectDir: "kasmos",
	})
	openCount := strings.Count(result.Left, "#[fg=") +
		strings.Count(result.Left, "#[bold]") +
		strings.Count(result.Right, "#[fg=") +
		strings.Count(result.Right, "#[bold]")
	defCount := strings.Count(result.Left, "#[default]") +
		strings.Count(result.Right, "#[default]")
	assert.Equal(t, openCount, defCount,
		"every opening tmux style tag must be balanced with #[default]")
}

func TestRenderStatusBar_EmptyFields_NoStyleLeak(t *testing.T) {
	result := RenderStatusBar(StatusBarData{})
	// With empty data only bold is used; it must be terminated.
	boldCount := strings.Count(result.Left, "#[bold]")
	defCount := strings.Count(result.Left, "#[default]")
	assert.Equal(t, boldCount, defCount)
}
