package overlay

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TextInputOverlay represents a text input overlay with state management.
type TextInputOverlay struct {
	textarea      textarea.Model
	Title         string
	FocusIndex    int // 0 for text input, 1 for enter button
	Submitted     bool
	Canceled      bool
	OnSubmit      func()
	width, height int
	multiline     bool
	styles        Styles
}

// NewTextInputOverlay creates a new text input overlay with the given title and initial value.
func NewTextInputOverlay(title string, initialValue string) *TextInputOverlay {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	s := ti.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ti.SetStyles(s)

	// Ensure no character limit
	ti.CharLimit = 0
	// Ensure no maximum height limit
	ti.MaxHeight = 0

	return &TextInputOverlay{
		textarea:   ti,
		Title:      title,
		FocusIndex: 0,
		Submitted:  false,
		Canceled:   false,
		styles:     DefaultStyles(),
	}
}

// SetMultiline enables multiline mode where Enter inserts newlines
// and the user must Tab to the submit button then press Enter to submit.
func (t *TextInputOverlay) SetMultiline(enabled bool) {
	t.multiline = enabled
}

// SetPlaceholder sets the textarea placeholder text.
func (t *TextInputOverlay) SetPlaceholder(text string) {
	t.textarea.Placeholder = text
}

// SetSize updates the available dimensions for the overlay.
// Implements the Overlay interface.
func (t *TextInputOverlay) SetSize(width, height int) {
	t.textarea.SetHeight(height)
	t.width = width
	t.height = height
}

// Width returns the current width of the overlay.
func (t *TextInputOverlay) Width() int { return t.width }

// Height returns the current height of the overlay.
func (t *TextInputOverlay) Height() int { return t.height }

// HandleKey processes a key event and returns the result.
// Implements the Overlay interface.
func (t *TextInputOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	switch msg.String() {
	case "tab", "shift+tab":
		t.FocusIndex = (t.FocusIndex + 1) % 2
		if t.FocusIndex == 0 {
			t.textarea.Focus()
		} else {
			t.textarea.Blur()
		}
		return Result{}
	case "esc":
		t.Canceled = true
		return Result{Dismissed: true, Submitted: false}
	case "enter":
		if t.multiline && t.FocusIndex == 0 {
			// In multiline mode, Enter inserts a newline when textarea is focused
			t.textarea, _ = t.textarea.Update(msg)
			return Result{}
		}
		// Submit (non-multiline, or button is focused in multiline)
		t.Submitted = true
		if t.OnSubmit != nil {
			t.OnSubmit()
		}
		return Result{Dismissed: true, Submitted: true, Value: t.textarea.Value()}
	default:
		if t.FocusIndex == 0 {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return Result{}
	}
}

// View renders the text input overlay content.
// Implements the Overlay interface.
func (t *TextInputOverlay) View() string {
	style := t.styles.ModalBorder

	// Set textarea width to fit within the overlay
	w := t.width
	if w < 40 {
		w = 40
	}
	t.textarea.SetWidth(w - 6) // Account for padding and borders

	// Build the view
	content := t.styles.Title.Render(t.Title) + "\n"
	content += t.textarea.View() + "\n\n"

	// Render enter button with appropriate style
	enterButton := " Enter "
	if t.FocusIndex == 1 {
		enterButton = t.styles.FocusedButton.Render(enterButton)
	} else {
		enterButton = t.styles.Button.Render(enterButton)
	}
	content += enterButton
	if t.multiline {
		content += "  " + t.styles.Muted.Render("tab → enter submit · esc cancel")
	}

	return style.Render(content)
}
