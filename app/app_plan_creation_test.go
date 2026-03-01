package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanFilename(t *testing.T) {
	got := buildPlanFilename("Auth Refactor", time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	want := "2026-02-21-auth-refactor.md"
	if got != want {
		t.Fatalf("buildPlanFilename() = %q, want %q", got, want)
	}
}

func TestRenderPlanStub(t *testing.T) {
	stub := renderPlanStub("Auth Refactor", "Refactor JWT auth", "2026-02-21-auth-refactor.md")
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

	h := &home{planStateDir: plansDir, planState: ps}

	planFile := "2026-02-21-auth-refactor.md"
	branch := "plan/auth-refactor"
	err = h.createPlanRecord(planFile, "Refactor JWT auth", branch, time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	entry, ok := h.planState.Entry(planFile)
	require.True(t, ok)
	if entry.Branch != branch {
		t.Fatalf("entry.Branch = %q, want %q", entry.Branch, branch)
	}
}

func TestHandleDefaultStateStartsDescriptionOverlay(t *testing.T) {
	h := &home{
		state:        stateDefault,
		keySent:      true,
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
	}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlan, updated.state)
	require.NotNil(t, updated.textInputOverlay)
}

func TestHandleKeyPressNewPlanWithoutOverlayReturnsDefault(t *testing.T) {
	h := &home{state: stateNewPlan}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
}

func TestNewPlanSubmitShowsTopicPicker(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "refactor auth module"),
	}
	h.textInputOverlay.SetMultiline(true)

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

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

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
	require.Empty(t, updated.pendingPlanName)
	require.Empty(t, updated.pendingPlanDesc)
}

func TestNewPlanTopicPickerShowsPendingPlanName(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "auth refactor"),
	}
	h.textInputOverlay.SetMultiline(true)

	// Tab to button, then Enter to submit — enters deriving state
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)

	// Simulate AI title arriving — transitions to topic picker
	model2, _ := updated.Update(planTitleMsg{title: "auth refactor"})
	updated2, ok := model2.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated2.state)
	require.NotNil(t, updated2.pickerOverlay)
	require.Contains(t, strings.ToLower(updated2.pickerOverlay.Render()), "auth refactor")
}

func TestNewPlanSubmitEntersDerivingState(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "refactor auth module"),
	}
	h.textInputOverlay.SetMultiline(true)

	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

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
	}

	model, _ := h.Update(planTitleMsg{title: "ai derived title"})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "ai derived title", updated.pendingPlanName)
	require.NotNil(t, updated.pickerOverlay)
}

func TestDerivingStateFallsBackOnAIError(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "heuristic fallback title",
		pendingPlanDesc: "some description",
	}

	model, _ := h.Update(planTitleMsg{err: fmt.Errorf("timeout")})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "heuristic fallback title", updated.pendingPlanName)
	require.NotNil(t, updated.pickerOverlay)
}

func TestDerivingStateBlocksKeyInput(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "test",
		pendingPlanDesc: "test desc",
	}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

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

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEscape})

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
		{stateFocusAgent, true},
		{statePermission, true},
	}
	for _, tt := range tests {
		h := &home{state: tt.state}
		require.Equal(t, tt.expected, h.isUserInOverlay(),
			"isUserInOverlay() for state %d", tt.state)
	}
}

func TestNewPlanSubmitSkipsAIWhenFirstLineIsViableSlug(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "fix auth refresh\ndetails about the bug"),
	}
	h.textInputOverlay.SetMultiline(true)

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

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
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		nav:          ui.NewNavigationPanel(&s),
		menu:         ui.NewMenu(),
		toastManager: overlay.NewToastManager(&s),
	}
	// Simulate initial terminal size.
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

	// Now create the overlay with a fixed size.
	h.textInputOverlay = overlay.NewTextInputOverlay("new plan", "")
	h.textInputOverlay.SetMultiline(true)
	h.textInputOverlay.SetSize(70, 8)

	// Simulate a spurious WindowSize (same dimensions, triggered by instanceStartedMsg).
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 200, Height: 50})

	// Overlay should still be 70 wide, not 120 (200*0.6).
	require.Equal(t, 70, h.textInputOverlay.Width())
	require.Equal(t, 8, h.textInputOverlay.Height())
}
