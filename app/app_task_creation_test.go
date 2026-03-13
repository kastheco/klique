package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanFilename(t *testing.T) {
	got := buildPlanFilename("Auth Refactor", time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	want := "auth-refactor"
	if got != want {
		t.Fatalf("buildPlanFilename() = %q, want %q", got, want)
	}
}

func TestRenderPlanStub(t *testing.T) {
	stub := renderPlanStub("Auth Refactor", "Refactor JWT auth", "auth-refactor.md")
	if !strings.Contains(stub, "# Auth Refactor") {
		t.Fatalf("stub missing title: %s", stub)
	}
	if !strings.Contains(stub, "Refactor JWT auth") {
		t.Fatalf("stub missing description")
	}
}

func TestCreatePlanRecord(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	h := &home{taskStateDir: plansDir, taskState: ps}

	planFile := "auth-refactor"
	branch := "plan/auth-refactor"
	err = h.createPlanRecord(planFile, "Refactor JWT auth", branch, time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	entry, ok := h.taskState.Entry(planFile)
	require.True(t, ok)
	if entry.Branch != branch {
		t.Fatalf("entry.Branch = %q, want %q", entry.Branch, branch)
	}
}

func TestHandleDefaultStateStartsDescriptionOverlay(t *testing.T) {
	h := &home{
		state:        stateDefault,
		keySent:      true,
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		overlays:     overlay.NewManager(),
	}

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'n', Text: "n"})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlan, updated.state)
	require.True(t, updated.overlays.IsActive())
}

func TestHandleKeyPressNewPlanWithoutOverlayReturnsDefault(t *testing.T) {
	h := &home{state: stateNewPlan}

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
}

func TestNewPlanSubmitShowsTopicPicker(t *testing.T) {
	tio1 := overlay.NewTextInputOverlay("new plan", "refactor auth module")
	tio1.SetMultiline(true)
	mgr1 := overlay.NewManager()
	mgr1.Show(tio1)
	h := &home{
		state:    stateNewPlan,
		overlays: mgr1,
	}

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	// After submit, we enter the deriving state (not topic picker yet)
	require.Equal(t, stateNewPlanDeriving, updated.state)
	require.NotEmpty(t, updated.pendingPlanName)
	require.Equal(t, "refactor auth module", updated.pendingPlanDesc)
	// cmd should be the AI title derivation command (non-nil)
	require.NotNil(t, cmd)
}

func TestHandleKeyPressNewPlanTopicWithoutPickerClearsPendingValues(t *testing.T) {
	h := &home{
		state:           stateNewPlanTopic,
		pendingPlanName: "auth-refactor",
		pendingPlanDesc: "Refactor JWT auth",
	}

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
	require.Empty(t, updated.pendingPlanName)
	require.Empty(t, updated.pendingPlanDesc)
}

func TestNewPlanTopicPickerShowsPendingPlanName(t *testing.T) {
	tio2 := overlay.NewTextInputOverlay("new plan", "auth refactor")
	tio2.SetMultiline(true)
	mgr2 := overlay.NewManager()
	mgr2.Show(tio2)
	h := &home{
		state:    stateNewPlan,
		overlays: mgr2,
	}

	// Tab to button, then Enter to submit — enters deriving state
	h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyTab})
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)

	// Simulate AI title arriving — transitions to topic picker
	model2, _ := updated.Update(planTitleMsg{title: "auth refactor"})
	updated2, ok := model2.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated2.state)
	require.True(t, updated2.overlays.IsActive())
	po2, ok2 := updated2.overlays.Current().(*overlay.PickerOverlay)
	require.True(t, ok2, "current overlay must be a PickerOverlay")
	// Check both words are present (may be split across lines by lipgloss wrapping).
	viewLower := strings.ToLower(po2.View())
	require.Contains(t, viewLower, "auth")
	require.Contains(t, viewLower, "refactor")
}

func TestNewPlanSubmitEntersDerivingState(t *testing.T) {
	tio3 := overlay.NewTextInputOverlay("new plan", "refactor auth module")
	tio3.SetMultiline(true)
	mgr3 := overlay.NewManager()
	mgr3.Show(tio3)
	h := &home{
		state:    stateNewPlan,
		overlays: mgr3,
	}

	h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)
	require.Equal(t, "refactor auth module", updated.pendingPlanDesc)
	require.NotEmpty(t, updated.pendingPlanName)
	require.NotNil(t, cmd)
}

func TestDerivingStateTransitionsToTopicOnAITitle(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "heuristic-fallback",
		pendingPlanDesc: "some description",
		overlays:        overlay.NewManager(),
	}

	model, _ := h.Update(planTitleMsg{title: "ai derived title"})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "ai derived title", updated.pendingPlanName)
	require.True(t, updated.overlays.IsActive())
}

func TestDerivingStateFallsBackOnAIError(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "heuristic fallback title",
		pendingPlanDesc: "some description",
		overlays:        overlay.NewManager(),
	}

	model, _ := h.Update(planTitleMsg{err: fmt.Errorf("timeout")})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "heuristic fallback title", updated.pendingPlanName)
	require.True(t, updated.overlays.IsActive())
}

func TestDerivingStateBlocksKeyInput(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "test",
		pendingPlanDesc: "test desc",
	}

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'x', Text: "x"})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)
	require.Nil(t, cmd)
}

func TestDerivingStateEscapeCancels(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "test",
		pendingPlanDesc: "test desc",
	}

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
	require.Empty(t, updated.pendingPlanName)
	require.Empty(t, updated.pendingPlanDesc)
}

func TestIsUserInOverlay(t *testing.T) {
	tests := []struct {
		state    state
		expected bool
	}{
		{stateDefault, false},
		{stateNewPlan, true},
		{stateNewPlanTopic, true},
		{stateConfirm, true},
		{statePrompt, true},
		{stateSpawnAgent, true},
		{statePermission, true},
	}
	for _, tt := range tests {
		h := &home{state: tt.state}
		require.Equal(t, tt.expected, h.isUserInOverlay(),
			"isUserInOverlay() for state %d", tt.state)
	}
}

func TestNewPlanSubmitSkipsAIWhenFirstLineIsViableSlug(t *testing.T) {
	tio4 := overlay.NewTextInputOverlay("new plan", "fix auth refresh\ndetails about the bug")
	tio4.SetMultiline(true)
	mgr4 := overlay.NewManager()
	mgr4.Show(tio4)
	h := &home{
		state:    stateNewPlan,
		overlays: mgr4,
	}

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	// Should skip stateNewPlanDeriving and go straight to topic picker
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "fix auth refresh", updated.pendingPlanName)
	// No AI command should be returned
	require.Nil(t, cmd)
}

func TestNewPlanOverlaySizePreservedOnSpuriousWindowSize(t *testing.T) {
	s := spinner.New()
	h := &home{
		state:        stateNewPlan,
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		nav:          ui.NewNavigationPanel(&s),
		menu:         ui.NewMenu(),
		toastManager: overlay.NewToastManager(&s),
		overlays:     overlay.NewManager(),
	}
	// Simulate initial terminal size.
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

	// Now create the overlay, show it via the manager, then set a fixed size.
	// (Size must be set AFTER Show so it overrides the manager's auto-sizing.)
	tio5 := overlay.NewTextInputOverlay("new plan", "")
	tio5.SetMultiline(true)
	h.overlays.Show(tio5)
	tio5.SetSize(70, 8)

	// Simulate a spurious WindowSize (same dimensions, triggered by instanceStartedMsg).
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

	// Overlay should still be 70 wide, not 120 (200*0.6).
	cur, ok := h.overlays.Current().(*overlay.TextInputOverlay)
	require.True(t, ok, "current overlay must be a TextInputOverlay")
	require.Equal(t, 70, cur.Width())
	require.Equal(t, 8, cur.Height())
}
