package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUiToTmuxStatusBarData_FieldMapping verifies that all fields are copied
// correctly from ui.StatusBarData to tmux.StatusBarData.
func TestUiToTmuxStatusBarData_FieldMapping(t *testing.T) {
	src := ui.StatusBarData{
		Branch:           "main",
		Version:          "v1.2.3",
		PlanName:         "my-plan",
		PlanStatus:       "implementing",
		WaveLabel:        "wave 2/4",
		TaskGlyphs:       []ui.TaskGlyph{ui.TaskGlyphComplete, ui.TaskGlyphRunning, ui.TaskGlyphPending},
		TmuxSessionCount: 3,
		ProjectDir:       "kasmos",
		PRState:          "approved",
		PRChecks:         "passing",
	}

	got := uiToTmuxStatusBarData(src)

	assert.Equal(t, src.Branch, got.Branch)
	assert.Equal(t, src.Version, got.Version)
	assert.Equal(t, src.PlanName, got.PlanName)
	assert.Equal(t, src.PlanStatus, got.PlanStatus)
	assert.Equal(t, src.WaveLabel, got.WaveLabel)
	assert.Equal(t, src.TmuxSessionCount, got.TmuxSessionCount)
	assert.Equal(t, src.ProjectDir, got.ProjectDir)
	assert.Equal(t, src.PRState, got.PRState)
	assert.Equal(t, src.PRChecks, got.PRChecks)

	require.Len(t, got.TaskGlyphs, 3)
	assert.Equal(t, tmux.TaskGlyphComplete, got.TaskGlyphs[0])
	assert.Equal(t, tmux.TaskGlyphRunning, got.TaskGlyphs[1])
	assert.Equal(t, tmux.TaskGlyphPending, got.TaskGlyphs[2])
}

// TestUiToTmuxStatusBarData_GlyphIntegerValues confirms the iota-order assumption:
// ui.TaskGlyph and tmux.TaskGlyph share the same integer values.
func TestUiToTmuxStatusBarData_GlyphIntegerValues(t *testing.T) {
	cases := []struct {
		ui   ui.TaskGlyph
		tmux tmux.TaskGlyph
	}{
		{ui.TaskGlyphComplete, tmux.TaskGlyphComplete},
		{ui.TaskGlyphRunning, tmux.TaskGlyphRunning},
		{ui.TaskGlyphFailed, tmux.TaskGlyphFailed},
		{ui.TaskGlyphPending, tmux.TaskGlyphPending},
	}
	for _, tc := range cases {
		assert.Equal(t, int(tc.tmux), int(tc.ui), "glyph integer mismatch")
	}
}

// TestUiToTmuxStatusBarData_EmptyGlyphs verifies nil/empty slice handling.
func TestUiToTmuxStatusBarData_EmptyGlyphs(t *testing.T) {
	src := ui.StatusBarData{}
	got := uiToTmuxStatusBarData(src)
	assert.Empty(t, got.TaskGlyphs)
}

// newMinimalHome builds the smallest valid *home for status-bar unit tests.
func newMinimalHome(t *testing.T) *home {
	t.Helper()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&sp),
		menu:         ui.NewMenu(),
		toastManager: overlay.NewToastManager(&sp),
		overlays:     overlay.NewManager(),
	}
}

// TestUpdateTmuxStatusBarCmd_NoLayoutSession returns nil when layoutSessionName is empty.
func TestUpdateTmuxStatusBarCmd_NoLayoutSession(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "" // not inside the two-pane layout

	cmd := m.updateTmuxStatusBarCmd(ui.StatusBarData{Branch: "main"})
	assert.Nil(t, cmd)
}

// TestUpdateTmuxStatusBarCmd_NoLayoutEnv returns nil when KASMOS_LAYOUT != "1".
func TestUpdateTmuxStatusBarCmd_NoLayoutEnv(t *testing.T) {
	os.Unsetenv("KASMOS_LAYOUT")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	cmd := m.updateTmuxStatusBarCmd(ui.StatusBarData{Branch: "main"})
	assert.Nil(t, cmd)
}

// TestUpdateTmuxStatusBarCmd_CacheSkipsDuplicate returns nil on the second call
// when the rendered strings are identical (cache hit).
func TestUpdateTmuxStatusBarCmd_CacheSkipsDuplicate(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	data := ui.StatusBarData{Branch: "feat/x", Version: "v0.1.0"}

	// First call: cache is empty → should return a non-nil cmd.
	cmd1 := m.updateTmuxStatusBarCmd(data)
	assert.NotNil(t, cmd1, "first call should produce a cmd")

	// Second call with identical data: cache hit → should return nil.
	cmd2 := m.updateTmuxStatusBarCmd(data)
	assert.Nil(t, cmd2, "second call with same data should be a no-op")
}

// TestUpdateTmuxStatusBarCmd_CacheUpdatesOnChange returns a new cmd when data changes.
func TestUpdateTmuxStatusBarCmd_CacheUpdatesOnChange(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	data1 := ui.StatusBarData{Branch: "main"}
	data2 := ui.StatusBarData{Branch: "feat/new-feature"}

	cmd1 := m.updateTmuxStatusBarCmd(data1)
	assert.NotNil(t, cmd1)

	cmd2 := m.updateTmuxStatusBarCmd(data2)
	assert.NotNil(t, cmd2, "changed data should produce a new cmd")
}

// TestUpdateTmuxStatusBarCmd_CacheFieldsPopulated verifies the cache fields are
// set after the first call so subsequent identical calls are skipped.
func TestUpdateTmuxStatusBarCmd_CacheFieldsPopulated(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	data := ui.StatusBarData{Branch: "main", Version: "v1.0.0"}
	_ = m.updateTmuxStatusBarCmd(data)

	// After the first call the cache must be non-empty.
	assert.NotEmpty(t, m.lastTmuxStatusLeft)
	assert.NotEmpty(t, m.lastTmuxStatusRight)
}

// TestUpdateTmuxStatusBarCmd_ReturnedCmdIsCallable verifies the returned tea.Cmd
// is a valid function (not nil) and can be invoked without panicking.
// The actual tmux subprocess will fail in CI (no tmux), but the cmd must not panic.
func TestUpdateTmuxStatusBarCmd_ReturnedCmdIsCallable(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	data := ui.StatusBarData{Branch: "main", Version: "v1.0.0"}
	cmd := m.updateTmuxStatusBarCmd(data)
	require.NotNil(t, cmd)

	// Calling the cmd should not panic; it will log an error (no real tmux) and
	// return nil.
	var msg tea.Msg
	assert.NotPanics(t, func() { msg = cmd() })
	assert.Nil(t, msg)
}

// TestUpdateTmuxStatusBarCmd_RenderedStringsContainBranch is a light integration
// check: the rendered left string should contain the branch name.
func TestUpdateTmuxStatusBarCmd_RenderedStringsContainBranch(t *testing.T) {
	t.Setenv("KASMOS_LAYOUT", "1")
	m := newMinimalHome(t)
	m.layoutSessionName = "kas_main_myrepo"

	data := ui.StatusBarData{Branch: "my-branch", Version: "v2.0.0"}
	_ = m.updateTmuxStatusBarCmd(data)

	assert.True(t, strings.Contains(m.lastTmuxStatusLeft, "my-branch") ||
		strings.Contains(m.lastTmuxStatusRight, "my-branch"),
		"rendered status bar should contain the branch name")
}
