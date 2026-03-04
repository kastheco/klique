package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestPermissionOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewPermissionOverlay("instance", "desc", "pattern")
}

func TestPermissionOverlay_HandleKey_Confirm(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "allow_once", result.Action)
}

func TestPermissionOverlay_HandleKey_Navigate(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "allow_always", result.Action)
}

func TestPermissionOverlay_HandleKey_Reject(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "reject", result.Action)
}

func TestPermissionOverlay_HandleKey_Dismiss(t *testing.T) {
	p := NewPermissionOverlay("inst", "run command", "*.sh")
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
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
