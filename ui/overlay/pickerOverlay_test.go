package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestPickerOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewPickerOverlay("pick one", []string{"a", "b", "c"})
}

func TestPickerOverlay_HandleKey_Submit(t *testing.T) {
	p := NewPickerOverlay("pick", []string{"alpha", "beta"})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "alpha", result.Value)
}

func TestPickerOverlay_HandleKey_Navigate(t *testing.T) {
	p := NewPickerOverlay("pick", []string{"alpha", "beta"})
	p.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "beta", result.Value)
}

func TestPickerOverlay_HandleKey_Filter(t *testing.T) {
	p := NewPickerOverlay("pick", []string{"alpha", "beta", "gamma"})
	p.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "beta", result.Value)
}

func TestPickerOverlay_HandleKey_Cancel(t *testing.T) {
	p := NewPickerOverlay("pick", []string{"alpha"})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestPickerOverlay_AllowCustom(t *testing.T) {
	p := NewPickerOverlay("pick", []string{"alpha"})
	p.SetAllowCustom(true)
	p.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	result := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Submitted)
	assert.Equal(t, "z", result.Value)
}

func TestPickerOverlay_View(t *testing.T) {
	p := NewPickerOverlay("select item", []string{"one", "two"})
	p.SetSize(50, 20)
	view := p.View()
	assert.Contains(t, view, "select item")
	assert.Contains(t, view, "one")
	assert.Contains(t, view, "two")
}
