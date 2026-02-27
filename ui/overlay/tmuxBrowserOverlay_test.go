package overlay

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxBrowserOverlay_Basic(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now().Add(-10 * time.Minute), Width: 80, Height: 24},
		{Name: "kas_bar", Title: "bar", Created: time.Now().Add(-3 * time.Hour), Width: 120, Height: 40, Attached: true},
	}

	b := NewTmuxBrowserOverlay(items)
	require.NotNil(t, b)

	// Render should not panic and should contain session titles
	rendered := b.Render()
	assert.Contains(t, rendered, "foo")
	assert.Contains(t, rendered, "bar")
}

func TestTmuxBrowserOverlay_Navigation(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	assert.Equal(t, 0, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, b.selectedIdx)
}

func TestTmuxBrowserOverlay_SearchFilter(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_auth", Title: "auth", Created: time.Now()},
		{Name: "kas_db", Title: "db", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	assert.Len(t, b.filtered, 2)

	// Type "db" to filter — note: "a" and "k" are action keys when search is empty,
	// so we use "d" then "b" (non-action-key characters) to enter search mode safely.
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	assert.Len(t, b.filtered, 1)
	assert.Equal(t, 1, b.filtered[0]) // index of "db"
}

func TestTmuxBrowserOverlay_Actions(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}

	tests := []struct {
		name     string
		key      tea.KeyMsg
		expected BrowserAction
	}{
		{"esc dismisses", tea.KeyMsg{Type: tea.KeyEsc}, BrowserDismiss},
		{"enter attaches", tea.KeyMsg{Type: tea.KeyEnter}, BrowserAttach},
		{"k kills when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, BrowserKill},
		{"a adopts when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, BrowserAdopt},
		{"o attaches when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}, BrowserAttach},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewTmuxBrowserOverlay(items)
			action := b.HandleKeyPress(tt.key)
			assert.Equal(t, tt.expected, action)
		})
	}
}

func TestTmuxBrowserOverlay_ActionKeysTypeWhenSearchActive(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	// Type "x" to enter search mode
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.Equal(t, "x", b.searchQuery)

	// Now "k" should type into search, not kill
	action := b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	assert.Equal(t, BrowserNone, action)
	assert.Equal(t, "xk", b.searchQuery)
}

func TestTmuxBrowserOverlay_SelectedItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	assert.Equal(t, "kas_a", b.SelectedItem().Name)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "kas_b", b.SelectedItem().Name)
}

func TestTmuxBrowserOverlay_RemoveItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown}) // select "b"

	b.RemoveSelected()
	assert.Len(t, b.sessions, 2)
	assert.Equal(t, "kas_a", b.sessions[0].Name)
	assert.Equal(t, "kas_c", b.sessions[1].Name)
}

func TestTmuxBrowserOverlay_Empty(t *testing.T) {
	b := NewTmuxBrowserOverlay(nil)
	assert.True(t, b.IsEmpty())
	rendered := b.Render()
	assert.Contains(t, rendered, "no sessions")
}
