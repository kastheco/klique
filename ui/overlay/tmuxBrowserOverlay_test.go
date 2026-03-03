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

	// View should not panic and should contain session titles
	rendered := b.View()
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

	b.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, b.selectedIdx)

	b.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, b.selectedIdx)

	b.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
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
	b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	assert.Len(t, b.filtered, 1)
	assert.Equal(t, 1, b.filtered[0]) // index of "db"
}

func TestTmuxBrowserOverlay_Actions(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}

	tests := []struct {
		name          string
		key           tea.KeyMsg
		wantDismissed bool
		wantAction    string
	}{
		{"esc dismisses", tea.KeyMsg{Type: tea.KeyEsc}, true, ""},
		{"enter attaches", tea.KeyMsg{Type: tea.KeyEnter}, true, "attach"},
		{"k kills when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, false, "kill"},
		{"a adopts when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, true, "adopt"},
		{"o attaches when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}, true, "attach"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewTmuxBrowserOverlay(items)
			result := b.HandleKey(tt.key)
			assert.Equal(t, tt.wantDismissed, result.Dismissed)
			assert.Equal(t, tt.wantAction, result.Action)
		})
	}
}

func TestTmuxBrowserOverlay_ActionKeysTypeWhenSearchActive(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	// Type "x" to enter search mode
	b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.Equal(t, "x", b.searchQuery)

	// Now "k" should type into search, not kill
	result := b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	assert.False(t, result.Dismissed)
	assert.Equal(t, "xk", b.searchQuery)
}

func TestTmuxBrowserOverlay_SelectedItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	assert.Equal(t, "kas_a", b.SelectedItem().Name)

	b.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "kas_b", b.SelectedItem().Name)
}

func TestTmuxBrowserOverlay_RemoveItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	b.HandleKey(tea.KeyMsg{Type: tea.KeyDown}) // select "b"

	b.RemoveSelected()
	assert.Len(t, b.sessions, 2)
	assert.Equal(t, "kas_a", b.sessions[0].Name)
	assert.Equal(t, "kas_c", b.sessions[1].Name)
}

func TestTmuxBrowserOverlay_Empty(t *testing.T) {
	b := NewTmuxBrowserOverlay(nil)
	assert.True(t, b.IsEmpty())
	rendered := b.View()
	assert.Contains(t, rendered, "no sessions")
}

func TestTmuxBrowserOverlay_ManagedItemBlocksAdopt(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_managed", Title: "managed", Created: time.Now(), Managed: true, AgentType: "coder"},
	}
	b := NewTmuxBrowserOverlay(items)

	// "a" should be a no-op for managed items — not dismissed, no action
	result := b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	assert.False(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestTmuxBrowserOverlay_OrphanItemAllowsAdopt(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_orphan", Title: "orphan", Created: time.Now(), Managed: false},
	}
	b := NewTmuxBrowserOverlay(items)

	result := b.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "adopt", result.Action)
}

func TestTmuxBrowserOverlay_ManagedItemRendersAgentType(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_auth", Title: "auth", Created: time.Now(), Managed: true, AgentType: "coder", TaskFile: "auth-plan"},
	}
	b := NewTmuxBrowserOverlay(items)
	rendered := b.View()
	assert.Contains(t, rendered, "coder")
}

func TestTmuxBrowserOverlay_MixedItems(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_managed", Title: "managed", Created: time.Now(), Managed: true, AgentType: "planner"},
		{Name: "kas_orphan", Title: "orphan", Created: time.Now(), Managed: false},
	}
	b := NewTmuxBrowserOverlay(items)
	rendered := b.View()
	assert.Contains(t, rendered, "managed")
	assert.Contains(t, rendered, "orphan")
	assert.Contains(t, rendered, "planner")
}

func TestTmuxBrowserOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewTmuxBrowserOverlay(nil)
}

func TestTmuxBrowserOverlay_HandleKey_Dismiss(t *testing.T) {
	b := NewTmuxBrowserOverlay(nil)
	result := b.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestTmuxBrowserOverlay_HandleKey_Attach(t *testing.T) {
	items := []TmuxBrowserItem{{Name: "sess", Title: "my-session"}}
	b := NewTmuxBrowserOverlay(items)
	result := b.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "attach", result.Action)
}
