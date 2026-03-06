package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ MouseHandler = NewPermissionOverlay("instance", "desc", "pattern")

func permissionMouseTarget(t *testing.T, view, needle string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		x := strings.Index(clean, needle)
		if x >= 0 {
			return x, y
		}
	}
	require.FailNowf(t, "missing target", "could not find %q in view", needle)
	return 0, 0
}

func TestPermissionOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewPermissionOverlay("instance", "desc", "pattern")
}

func TestPermissionOverlay_HandleKey_Confirm(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "allow_always", result.Action, "default selection should be allow_always")
}

func TestPermissionOverlay_HandleKey_NavigateLeft(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "allow_once", result.Action)
}

func TestPermissionOverlay_HandleKey_Reject(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "reject", result.Action)
}

func TestPermissionOverlay_HandleKey_Dismiss(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestPermissionOverlay_View(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	p.SetSize(60, 20)
	view := p.View()
	assert.Contains(t, view, "permission required")
	assert.Contains(t, view, "run command")
}

func TestPermissionOverlay_HandleMouse_SelectAllowOnce(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	x, y := permissionMouseTarget(t, p.View(), "allow once")

	result := p.HandleMouse(x, y, tea.MouseLeft)

	assert.Equal(t, Result{Dismissed: true, Submitted: true, Action: "allow_once"}, result)
}

func TestPermissionOverlay_HandleMouse_NonChoiceLineNoop(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")

	result := p.HandleMouse(0, 0, tea.MouseLeft)

	assert.Equal(t, Result{}, result)
}

func TestPermissionOverlay_HandleMouse_DescriptionContainingAllowOnceStillUsesChoiceRow(t *testing.T) {
	p := NewPermissionOverlay("inst", "echo allow once", "*.sh")
	x, y := permissionMouseTarget(t, p.View(), "allow always")

	result := p.HandleMouse(x, y, tea.MouseLeft)

	assert.Equal(t, Result{Dismissed: true, Submitted: true, Action: "allow_always"}, result)
}
