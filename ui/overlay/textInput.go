package overlay

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	sizeSet       bool // true after the first SetSize call
}

// NewTextInputOverlay creates a new text input overlay with the given title and initial value.
func NewTextInputOverlay(title string, initialValue string) *TextInputOverlay {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

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

func (t *TextInputOverlay) SetSize(width, height int) {
	if t.sizeSet {
		return // ignore resize events after initial sizing
	}
	t.sizeSet = true
	t.textarea.SetHeight(height)
	t.width = width
	t.height = height
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (t *TextInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		t.FocusIndex = (t.FocusIndex + 1) % 2
		if t.FocusIndex == 0 {
			t.textarea.Focus()
		} else {
			t.textarea.Blur()
		}
		return false
	case tea.KeyEsc:
		t.Canceled = true
		return true
	case tea.KeyEnter:
		if t.multiline && t.FocusIndex == 0 {
			// In multiline mode, Enter inserts a newline when textarea is focused
			t.textarea, _ = t.textarea.Update(msg)
			return false
		}
		// Submit (non-multiline, or button is focused in multiline)
		t.Submitted = true
		if t.OnSubmit != nil {
			t.OnSubmit()
		}
		return true
	default:
		if t.FocusIndex == 0 {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return false
	}
}

// GetValue returns the current value of the text input.
func (t *TextInputOverlay) GetValue() string {
	return t.textarea.Value()
}

// IsSubmitted returns whether the form was submitted.
func (t *TextInputOverlay) IsSubmitted() bool {
	return t.Submitted
}

// Render renders the text input overlay.
func (t *TextInputOverlay) Render() string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorIris).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(colorIris).
		Bold(true).
		MarginBottom(1)

	buttonStyle := lipgloss.NewStyle().
		Foreground(colorSubtle)

	focusedButtonStyle := buttonStyle
	focusedButtonStyle = focusedButtonStyle.
		Background(colorIris).
		Foreground(colorBase)

	// Set textarea width to fit within the overlay
	w := t.width
	if w < 40 {
		w = 40
	}
	t.textarea.SetWidth(w - 6) // Account for padding and borders

	// Build the view
	content := titleStyle.Render(t.Title) + "\n"
	content += t.textarea.View() + "\n\n"

	// Render enter button with appropriate style
	enterButton := " Enter "
	if t.FocusIndex == 1 {
		enterButton = focusedButtonStyle.Render(enterButton)
	} else {
		enterButton = buttonStyle.Render(enterButton)
	}
	content += enterButton
	if t.multiline {
		content += "  " + lipgloss.NewStyle().Foreground(colorMuted).Render("tab → enter submit · esc cancel")
	}

	return style.Render(content)
}
