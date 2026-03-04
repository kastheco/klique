package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_IsActive(t *testing.T) {
	mgr := NewManager()
	assert.False(t, mgr.IsActive())

	mgr.Show(&stubOverlay{rendered: "test"})
	assert.True(t, mgr.IsActive())
}

func TestManager_ShowAndDismiss(t *testing.T) {
	mgr := NewManager()
	mgr.Show(&stubOverlay{rendered: "overlay content"})
	require.True(t, mgr.IsActive())

	result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, mgr.IsActive())
}

func TestManager_Submit(t *testing.T) {
	mgr := NewManager()
	mgr.Show(&stubOverlay{rendered: "form"})

	result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Submitted)
	assert.Equal(t, "test-value", result.Value)
	assert.False(t, mgr.IsActive(), "overlay should be dismissed after submit")
}

func TestManager_HandleKeyWhenInactive(t *testing.T) {
	mgr := NewManager()
	result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestManager_SetSize(t *testing.T) {
	s := &stubOverlay{}
	mgr := NewManager()
	mgr.Show(s)
	mgr.SetSize(100, 50)
	assert.Equal(t, 100, s.w)
	assert.Equal(t, 50, s.h)
}

func TestManager_Render_Inactive(t *testing.T) {
	mgr := NewManager()
	bg := "background content"
	assert.Equal(t, bg, mgr.Render(bg))
}

func TestManager_Render_Active(t *testing.T) {
	mgr := NewManager()
	mgr.SetSize(80, 24)
	mgr.Show(&stubOverlay{rendered: "OVERLAY"})
	bg := "background content here"
	result := mgr.Render(bg)
	assert.NotEqual(t, bg, result, "render should composite overlay onto background")
	assert.Contains(t, result, "OVERLAY")
}

func TestManager_Current(t *testing.T) {
	mgr := NewManager()
	assert.Nil(t, mgr.Current())

	s := &stubOverlay{rendered: "x"}
	mgr.Show(s)
	assert.Equal(t, s, mgr.Current())
}

func TestManager_Dismiss(t *testing.T) {
	mgr := NewManager()
	mgr.Show(&stubOverlay{rendered: "x"})
	require.True(t, mgr.IsActive())

	mgr.Dismiss()
	assert.False(t, mgr.IsActive())
}

// TestManager_Show_PreservesOverlaySize verifies that Show does not clobber the
// overlay's constructor-set dimensions with the viewport size.
func TestManager_Show_PreservesOverlaySize(t *testing.T) {
	mgr := NewManager()
	mgr.SetSize(120, 40) // viewport dimensions stored in manager

	s := &stubOverlay{w: 50, h: 10} // constructor-set size
	mgr.Show(s)

	// Show must NOT overwrite the overlay's own size with the viewport size.
	assert.Equal(t, 50, s.w, "Show must not clobber overlay width with viewport width")
	assert.Equal(t, 10, s.h, "Show must not clobber overlay height with viewport height")
}

// TestManager_ShowAt_PreservesOverlaySize mirrors the above for ShowAt.
func TestManager_ShowAt_PreservesOverlaySize(t *testing.T) {
	mgr := NewManager()
	mgr.SetSize(120, 40)

	s := &stubOverlay{w: 50, h: 10}
	mgr.ShowAt(s, false, false)

	assert.Equal(t, 50, s.w, "ShowAt must not clobber overlay width with viewport width")
	assert.Equal(t, 10, s.h, "ShowAt must not clobber overlay height with viewport height")
}

// TestManager_ShowPositioned_PreservesOverlaySize mirrors the above for ShowPositioned.
func TestManager_ShowPositioned_PreservesOverlaySize(t *testing.T) {
	mgr := NewManager()
	mgr.SetSize(120, 40)

	s := &stubOverlay{w: 56, h: 20}
	mgr.ShowPositioned(s, 10, 5, false)

	assert.Equal(t, 56, s.w, "ShowPositioned must not clobber overlay width with viewport width")
	assert.Equal(t, 20, s.h, "ShowPositioned must not clobber overlay height with viewport height")
}

// TestManager_SetSize_PropagatesAfterShow verifies that an explicit terminal
// resize still propagates to the active overlay even after the Show auto-sizing
// is removed.
func TestManager_SetSize_PropagatesAfterShow(t *testing.T) {
	mgr := NewManager()

	s := &stubOverlay{w: 50, h: 10} // constructor-set size
	mgr.Show(s)

	// Simulate a real terminal resize event — this SHOULD update the overlay.
	mgr.SetSize(120, 40)
	assert.Equal(t, 120, s.w, "SetSize must propagate to active overlay on terminal resize")
	assert.Equal(t, 40, s.h, "SetSize must propagate to active overlay on terminal resize")
}
