package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
)

func TestPermissionDetection_ShowsOverlayForOpenCode(t *testing.T) {
	m := newTestHome(t)
	inst := &session.Instance{
		Title:   "test-agent",
		Program: "opencode",
	}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)
	m.nav.SetSelectedInstance(0)

	// Simulate metadata tick with permission prompt detected
	inst.CachedContent = "△ Permission required\n  ← Access external directory /opt\n\nPatterns\n\n- /opt/*\n"
	inst.CachedContentSet = true

	pp := session.ParsePermissionPrompt(inst.CachedContent, inst.Program)
	assert.NotNil(t, pp)

	// Simulate the detection path
	m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
	m.pendingPermissionInstance = inst
	m.state = statePermission

	assert.Equal(t, statePermission, m.state)
	assert.NotNil(t, m.permissionOverlay)
}

func TestPermissionOverlay_ArrowKeysNavigate(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")

	// Default is "allow always" (index 0)
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice())

	// Right → "allow once"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice())

	// Right → "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Right at end → stays on "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Left → back to "allow once"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice())
}

func TestPermissionOverlay_EnterConfirms(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, po.IsConfirmed())
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice()) // default
}

func TestPermissionOverlay_EscDismisses(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, po.IsConfirmed())
}

func TestPermissionCache_AutoApprovesCachedPattern(t *testing.T) {
	m := newTestHome(t)
	m.permissionCache = config.NewPermissionCache(t.TempDir())
	m.permissionCache.Remember("/opt/*")
	assert.True(t, m.permissionCache.IsAllowedAlways("/opt/*"))
}
