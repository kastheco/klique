package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestContextMenu_ImplementsOverlay(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	var _ Overlay = NewContextMenu(items)
}

func TestContextMenu_HandleKey_Select(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "kill", result.Action)
}

func TestContextMenu_HandleKey_Navigate(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_NumberShortcut(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_Dismiss(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestContextMenu_HandleKey_DisabledSkipped(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "disabled", Action: "disabled", Disabled: true},
		{Label: "enabled", Action: "enabled"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "enabled", result.Action)
}
