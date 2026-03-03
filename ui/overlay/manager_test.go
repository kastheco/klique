package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
