package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestConfirmationOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewConfirmationOverlay("are you sure?")
}

func TestConfirmationOverlay_HandleKey_Confirm(t *testing.T) {
	c := NewConfirmationOverlay("delete?")
	result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "y", result.Action)
}

func TestConfirmationOverlay_HandleKey_Cancel(t *testing.T) {
	c := NewConfirmationOverlay("delete?")
	result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
	assert.Equal(t, "n", result.Action)
}

func TestConfirmationOverlay_HandleKey_Esc(t *testing.T) {
	c := NewConfirmationOverlay("delete?")
	result := c.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestConfirmationOverlay_HandleKey_CustomKeys(t *testing.T) {
	c := NewConfirmationOverlay("retry?")
	c.ConfirmKey = "r"
	c.CancelKey = "n"
	result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "r", result.Action)
}

func TestConfirmationOverlay_View(t *testing.T) {
	c := NewConfirmationOverlay("are you sure?")
	c.SetSize(60, 20)
	view := c.View()
	assert.Contains(t, view, "are you sure?")
}
