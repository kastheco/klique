package wizard

import (
	"testing"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/stretchr/testify/assert"
)

func TestHarnessStep_Toggle(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
		{Name: "opencode", Path: "/usr/bin/opencode", Found: true},
		{Name: "codex", Path: "", Found: false},
	})

	assert.True(t, h.selected["claude"])
	assert.True(t, h.selected["opencode"])
	assert.False(t, h.selected["codex"])

	// Toggle claude off
	h.cursor = 0
	h.toggle()
	assert.False(t, h.selected["claude"])

	// Toggle codex on
	h.cursor = 2
	h.toggle()
	assert.True(t, h.selected["codex"])
}

func TestHarnessStep_CannotProceedEmpty(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
	})
	h.selected["claude"] = false
	assert.False(t, h.canProceed())
}

func TestHarnessStep_SelectedNames(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "claude", Path: "/usr/bin/claude", Found: true},
		{Name: "opencode", Path: "", Found: false},
		{Name: "codex", Path: "/usr/bin/codex", Found: true},
	})
	// opencode not found, not selected by default
	names := h.selectedNames()
	assert.Equal(t, []string{"claude", "codex"}, names)
}

func TestHarnessStep_CursorNavigation(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{
		{Name: "a", Found: true},
		{Name: "b", Found: true},
		{Name: "c", Found: false},
	})
	assert.Equal(t, 0, h.cursor)

	h.cursorDown()
	assert.Equal(t, 1, h.cursor)

	h.cursorDown()
	assert.Equal(t, 2, h.cursor)

	h.cursorDown() // should clamp
	assert.Equal(t, 2, h.cursor)

	h.cursorUp()
	assert.Equal(t, 1, h.cursor)
}

func TestHarnessStep_ViewDoesNotRenderRootHeader(t *testing.T) {
	h := newHarnessStep([]harness.DetectResult{{Name: "claude", Found: true}})
	view := h.View(80, 24)
	assert.NotContains(t, view, "klique init wizard")
	assert.NotContains(t, view, "guided setup for harnesses and agents")
}
