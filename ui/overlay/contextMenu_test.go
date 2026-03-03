package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestContextMenu_ImplementsOverlay(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	var _ Overlay = NewContextMenu(0, 0, items)
}

func TestContextMenu_HandleKey_Select(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(0, 0, items)
	result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "kill", result.Action)
}

func TestContextMenu_HandleKey_Navigate(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(0, 0, items)
	cm.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_NumberShortcut(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(0, 0, items)
	result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_Dismiss(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	cm := NewContextMenu(0, 0, items)
	result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestContextMenu_HandleKey_DisabledSkipped(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "disabled", Action: "disabled", Disabled: true},
		{Label: "enabled", Action: "enabled"},
	}
	cm := NewContextMenu(0, 0, items)
	result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, "enabled", result.Action)
}
