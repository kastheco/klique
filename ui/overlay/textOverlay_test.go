package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestTextOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewTextOverlay("content")
}

func TestTextOverlay_HandleKey_AnyKeyDismisses(t *testing.T) {
	o := NewTextOverlay("help text")
	result := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.True(t, result.Dismissed)
}

func TestTextOverlay_View(t *testing.T) {
	o := NewTextOverlay("help content here")
	o.SetSize(60, 20)
	view := o.View()
	assert.Contains(t, view, "help content here")
}
