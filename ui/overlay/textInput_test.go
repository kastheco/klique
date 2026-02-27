package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextInputOverlay_DefaultEnterSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEnterInsertsNewline(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Enter when textarea is focused should NOT submit in multiline mode
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, closed)
	assert.False(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEnterOnButtonSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Tab to button
	ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 1, ti.FocusIndex)
	// Enter on button submits
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEscCancels(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.True(t, ti.Canceled)
}

func TestTextInputOverlay_SetPlaceholder(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetSize(80, 5) // wide enough so placeholder fits on one line
	ti.SetPlaceholder("describe what you want to work on...")
	assert.Contains(t, ti.Render(), "describe what you want to work on")
}

func TestTextInputOverlaySizeLockedAfterFirstSet(t *testing.T) {
	o := NewTextInputOverlay("test", "initial value")
	o.SetSize(70, 8)

	// Simulate a window resize event re-calling SetSize with different dimensions
	o.SetSize(120, 40)

	// The overlay should retain its original size
	rendered := o.Render()
	// The rendered width should reflect the original 70, not 120
	lines := strings.Split(rendered, "\n")
	maxWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxWidth {
			maxWidth = w
		}
	}
	// With padding+border the rendered width should be around 70, not 120+
	require.Less(t, maxWidth, 90, "overlay should not have grown to window size")
}
