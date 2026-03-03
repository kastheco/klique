package overlay

import tea "github.com/charmbracelet/bubbletea"

// Manager manages the active modal overlay and composites it onto the background.
type Manager struct {
	active   Overlay
	centered bool // whether to center the overlay (true for modals)
	shadow   bool // whether to show shadow/fade effect
	w, h     int  // viewport dimensions
}

// NewManager creates an overlay manager with no active overlay.
func NewManager() *Manager {
	return &Manager{centered: true, shadow: true}
}

// Show activates an overlay. Any previously active overlay is replaced.
func (m *Manager) Show(o Overlay) {
	m.active = o
	if m.w > 0 || m.h > 0 {
		o.SetSize(m.w, m.h)
	}
}

// ShowAt activates an overlay at a specific position (for context menus).
func (m *Manager) ShowAt(o Overlay, centered, shadow bool) {
	m.active = o
	m.centered = centered
	m.shadow = shadow
	if m.w > 0 || m.h > 0 {
		o.SetSize(m.w, m.h)
	}
}

// Dismiss closes the active overlay without returning a result.
func (m *Manager) Dismiss() {
	m.active = nil
	m.centered = true
	m.shadow = true
}

// IsActive returns true if a modal overlay is currently displayed.
func (m *Manager) IsActive() bool {
	return m.active != nil
}

// Current returns the active overlay, or nil.
func (m *Manager) Current() Overlay {
	return m.active
}

// HandleKey delegates to the active overlay. Returns a zero Result if inactive.
func (m *Manager) HandleKey(msg tea.KeyMsg) Result {
	if m.active == nil {
		return Result{}
	}
	result := m.active.HandleKey(msg)
	if result.Dismissed {
		m.active = nil
		m.centered = true
		m.shadow = true
	}
	return result
}

// SetSize updates the viewport dimensions and propagates to the active overlay.
func (m *Manager) SetSize(w, h int) {
	m.w = w
	m.h = h
	if m.active != nil {
		m.active.SetSize(w, h)
	}
}

// Render composites the active overlay onto the background string.
// Returns the background unchanged if no overlay is active.
func (m *Manager) Render(bg string) string {
	if m.active == nil {
		return bg
	}
	return PlaceOverlay(0, 0, m.active.View(), bg, m.shadow, m.centered)
}
