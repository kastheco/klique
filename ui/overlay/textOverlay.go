package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
)

// TextOverlay represents a text screen overlay.
type TextOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed
	OnDismiss func()
	// Content to display in the overlay
	content string

	width  int
	height int
	styles Styles
}

// NewTextOverlay creates a new text screen overlay with the given content.
func NewTextOverlay(content string) *TextOverlay {
	return &TextOverlay{
		Dismissed: false,
		content:   content,
		styles:    DefaultStyles(),
	}
}

// HandleKey processes a key event and returns the result.
// Any key dismisses the text overlay.
// Implements the Overlay interface.
func (t *TextOverlay) HandleKey(msg tea.KeyMsg) Result {
	t.Dismissed = true
	if t.OnDismiss != nil {
		t.OnDismiss()
	}
	return Result{Dismissed: true}
}

// HandleKeyPress processes a key press and updates the state.
// Deprecated: use HandleKey instead. Returns true if the overlay should be closed.
func (t *TextOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	return t.HandleKey(msg).Dismissed
}

// View renders the text overlay content.
// Implements the Overlay interface.
func (t *TextOverlay) View() string {
	style := t.styles.ModalBorder
	if t.width > 0 {
		style = style.Width(t.width)
	}
	return style.Render(t.content)
}

// Render renders the text overlay.
// Deprecated: use View instead.
func (t *TextOverlay) Render(opts ...WhitespaceOption) string {
	return t.View()
}

// SetSize updates the available dimensions for the overlay.
// Implements the Overlay interface.
func (t *TextOverlay) SetSize(w, h int) {
	t.width = w
	t.height = h
}

// SetWidth sets the width of the text overlay.
// Deprecated: use SetSize instead.
func (t *TextOverlay) SetWidth(width int) {
	t.width = width
}
