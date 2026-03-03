package overlay

import tea "github.com/charmbracelet/bubbletea"

// Result is returned by Overlay.HandleKey to signal what happened.
type Result struct {
	// Dismissed is true when the overlay should be closed.
	Dismissed bool
	// Submitted is true when the user confirmed/submitted (not just dismissed).
	Submitted bool
	// Value carries the primary return value (text input content, selected item, etc.).
	Value string
	// Action carries a secondary action identifier (context menu action, browser action, etc.).
	Action string
}

// Overlay is the common interface for all modal overlay components.
// Every overlay type in the package implements this interface.
type Overlay interface {
	// HandleKey processes a key event and returns the result.
	HandleKey(msg tea.KeyMsg) Result
	// View renders the overlay content (without the PlaceOverlay compositing).
	View() string
	// SetSize updates the available dimensions for the overlay.
	SetSize(w, h int)
}
