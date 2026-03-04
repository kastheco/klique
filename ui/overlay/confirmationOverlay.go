package overlay

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmationOverlay represents a confirmation dialog overlay.
type ConfirmationOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Message to display in the overlay
	message string
	// Width of the overlay
	width int
	// height is stored but not used for layout (fixed content height)
	height int
	// Callback function to be called when the user confirms (presses 'y')
	OnConfirm func()
	// Callback function to be called when the user cancels (presses 'n' or 'esc')
	OnCancel func()
	// Custom confirm key (defaults to 'y')
	ConfirmKey string
	// Custom cancel key (defaults to 'n')
	CancelKey string
	// styles holds the shared overlay styles
	styles Styles
}

// NewConfirmationOverlay creates a new confirmation dialog overlay with the given message.
func NewConfirmationOverlay(message string) *ConfirmationOverlay {
	return &ConfirmationOverlay{
		Dismissed:  false,
		message:    message,
		width:      50, // Default width
		ConfirmKey: "y",
		CancelKey:  "n",
		styles:     DefaultStyles(),
	}
}

// HandleKey processes a key event and returns the result.
// Implements the Overlay interface.
func (c *ConfirmationOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	key := msg.String()
	switch key {
	case c.ConfirmKey:
		c.Dismissed = true
		if c.OnConfirm != nil {
			c.OnConfirm()
		}
		return Result{Dismissed: true, Submitted: true, Action: key}
	case c.CancelKey:
		c.Dismissed = true
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return Result{Dismissed: true, Submitted: false, Action: key}
	case "esc":
		c.Dismissed = true
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return Result{Dismissed: true, Submitted: false}
	default:
		return Result{}
	}
}

// View renders the confirmation overlay content.
// Implements the Overlay interface.
func (c *ConfirmationOverlay) View() string {
	style := c.styles.DangerBorder.Width(c.width)

	// Add the confirmation instructions
	content := c.message + "\n\n" +
		"Press " + lipgloss.NewStyle().Bold(true).Render(c.ConfirmKey) + " to confirm, " +
		lipgloss.NewStyle().Bold(true).Render(c.CancelKey) + " or " +
		lipgloss.NewStyle().Bold(true).Render("esc") + " to cancel"

	return style.Render(content)
}

// SetSize updates the available dimensions for the overlay.
// Implements the Overlay interface.
func (c *ConfirmationOverlay) SetSize(w, h int) {
	c.width = w
	c.height = h
}
