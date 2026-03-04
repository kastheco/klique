package overlay

import tea "charm.land/bubbletea/v2"

// Manager manages the active modal overlay and composites it onto the background.
type Manager struct {
	active   Overlay
	centered bool // whether to center the overlay (true for modals)
	shadow   bool // whether to show shadow/fade effect
	w, h     int  // viewport dimensions
	x, y     int  // explicit position (used when centered=false)
}

// NewManager creates an overlay manager with no active overlay.
func NewManager() *Manager {
	return &Manager{centered: true, shadow: true}
}

// Show activates an overlay. Any previously active overlay is replaced.
// No-op if m is nil.
// The overlay's own size (set in its constructor or by an explicit SetSize call
// at the call site) is preserved. The manager propagates viewport dimensions to
// overlays only on actual terminal-resize events via SetSize.
func (m *Manager) Show(o Overlay) {
	if m == nil {
		return
	}
	m.active = o
}

// ShowAt activates an overlay at a specific position (for context menus).
// No-op if m is nil.
// The overlay's own size is preserved; see Show for the sizing rationale.
func (m *Manager) ShowAt(o Overlay, centered, shadow bool) {
	if m == nil {
		return
	}
	m.active = o
	m.centered = centered
	m.shadow = shadow
	m.x = 0
	m.y = 0
}

// ShowPositioned activates an overlay at an explicit screen position.
// Use this for context menus and other non-centered overlays.
// No-op if m is nil.
// The overlay's own size is preserved; see Show for the sizing rationale.
func (m *Manager) ShowPositioned(o Overlay, x, y int, shadow bool) {
	if m == nil {
		return
	}
	m.active = o
	m.centered = false
	m.shadow = shadow
	m.x = x
	m.y = y
}

// Dismiss closes the active overlay without returning a result.
// No-op if m is nil.
func (m *Manager) Dismiss() {
	if m == nil {
		return
	}
	m.active = nil
	m.centered = true
	m.shadow = true
	m.x = 0
	m.y = 0
}

// IsActive returns true if a modal overlay is currently displayed.
// Returns false if m is nil.
func (m *Manager) IsActive() bool {
	if m == nil {
		return false
	}
	return m.active != nil
}

// Current returns the active overlay, or nil.
// Returns nil if m is nil.
func (m *Manager) Current() Overlay {
	if m == nil {
		return nil
	}
	return m.active
}

// HandleKey delegates to the active overlay. Returns a zero Result if inactive.
// Returns a zero Result if m is nil.
func (m *Manager) HandleKey(msg tea.KeyMsg) Result {
	if m == nil || m.active == nil {
		return Result{}
	}
	result := m.active.HandleKey(msg)
	if result.Dismissed {
		m.active = nil
		m.centered = true
		m.shadow = true
		m.x = 0
		m.y = 0
	}
	return result
}

// SetSize updates the viewport dimensions and propagates to the active overlay.
// No-op if m is nil.
func (m *Manager) SetSize(w, h int) {
	if m == nil {
		return
	}
	m.w = w
	m.h = h
	if m.active != nil {
		m.active.SetSize(w, h)
	}
}

// Render composites the active overlay onto the background string.
// Returns the background unchanged if no overlay is active or m is nil.
func (m *Manager) Render(bg string) string {
	if m == nil || m.active == nil {
		return bg
	}
	return PlaceOverlay(m.x, m.y, m.active.View(), bg, m.shadow, m.centered)
}
