package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestTextInputOverlay_DefaultEnterSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEnterInsertsNewline(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Enter when textarea is focused should NOT submit in multiline mode
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEnterOnButtonSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Tab to button
	ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, 1, ti.FocusIndex)
	// Enter on button submits
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
}

func TestTextInputOverlay_MultilineEscCancels(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.True(t, ti.Canceled)
}

func TestTextInputOverlay_SetPlaceholder(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetSize(80, 5) // wide enough so placeholder fits on one line
	ti.SetPlaceholder("describe what you want to work on...")
	// In textarea v2, the cursor is rendered inline with the placeholder, splitting
	// the first character from the rest. Check for the suffix that's always contiguous.
	assert.Contains(t, ti.View(), "escribe what you want to work on")
}

func TestTextInputOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewTextInputOverlay("title", "")
}

func TestTextInputOverlay_HandleKey_Submit(t *testing.T) {
	ti := NewTextInputOverlay("title", "hello")
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "hello", result.Value)
}

func TestTextInputOverlay_HandleKey_Cancel(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	result := ti.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}
