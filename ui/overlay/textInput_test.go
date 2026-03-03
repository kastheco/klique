package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestTextInputOverlay_DefaultEnterSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEnterInsertsNewline(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Enter when textarea is focused should NOT submit in multiline mode
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEnterOnButtonSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Tab to button
	ti.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 1, ti.FocusIndex)
	// Enter on button submits
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEscCancels(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.True(t, ti.Canceled)
}

func TestTextInputOverlay_SetPlaceholder(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetSize(80, 5) // wide enough so placeholder fits on one line
	ti.SetPlaceholder("describe what you want to work on...")
	assert.Contains(t, ti.View(), "describe what you want to work on")
}

func TestTextInputOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewTextInputOverlay("title", "")
}

func TestTextInputOverlay_HandleKey_Submit(t *testing.T) {
	ti := NewTextInputOverlay("title", "hello")
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "hello", result.Value)
}

func TestTextInputOverlay_HandleKey_Cancel(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}
