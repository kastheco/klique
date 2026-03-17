package overlay

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

	b.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, b.selectedIdx)

	b.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, b.selectedIdx)

	b.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
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
	b.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	b.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	assert.Len(t, b.filtered, 1)
	assert.Equal(t, 1, b.filtered[0]) // index of "db"
}

func TestTmuxBrowserOverlay_Actions(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}

	tests := []struct {
		name          string
		key           tea.KeyPressMsg
		wantDismissed bool
		wantAction    string
	}{
		{"esc dismisses", tea.KeyPressMsg{Code: tea.KeyEscape}, true, ""},
		{"enter attaches", tea.KeyPressMsg{Code: tea.KeyEnter}, true, "attach"},
		{"k kills when search empty", tea.KeyPressMsg{Code: 'k', Text: "k"}, false, "kill"},
		{"a adopts when search empty", tea.KeyPressMsg{Code: 'a', Text: "a"}, true, "adopt"},
		{"o attaches when search empty", tea.KeyPressMsg{Code: 'o', Text: "o"}, true, "attach"},
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
	b.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Equal(t, "x", b.searchQuery)

	// Now "k" should type into search, not kill
	result := b.HandleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
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

	b.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "kas_b", b.SelectedItem().Name)
}

func TestTmuxBrowserOverlay_RemoveItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	b.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // select "b"

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
	result := b.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.False(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestTmuxBrowserOverlay_OrphanItemAllowsAdopt(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_orphan", Title: "orphan", Created: time.Now(), Managed: false},
	}
	b := NewTmuxBrowserOverlay(items)

	result := b.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
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
	result := b.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestTmuxBrowserOverlay_HandleKey_Attach(t *testing.T) {
	items := []TmuxBrowserItem{{Name: "sess", Title: "my-session"}}
	b := NewTmuxBrowserOverlay(items)
	result := b.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "attach", result.Action)
}

func tmuxBrowserMouseTarget(t *testing.T, view, needle string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		x := strings.Index(clean, needle)
		if x >= 0 {
			return x, y
		}
	}
	require.FailNowf(t, "missing target", "could not find %q in view", needle)
	return 0, 0
}

func TestTmuxBrowserOverlay_HandleMouse_TitleFitsWithinMaxWidth(t *testing.T) {
	// Title is 48 chars — well under tmuxBrowserMaxTitleWidth (60), so it should
	// be displayed in full (no truncation) and remain clickable.
	title := "this-is-a-very-long-session-title-that-truncates"
	items := []TmuxBrowserItem{{Name: "sess", Title: title, Created: time.Now()}}
	b := NewTmuxBrowserOverlay(items)

	// The full title must appear because titleColWidth == len(title) here.
	assert.Equal(t, len([]rune(title)), b.titleColWidth)
	renderedTitle := truncateStr(title, b.titleColWidth) // no-op truncation
	x, y := tmuxBrowserMouseTarget(t, b.View(), renderedTitle)

	result := b.HandleMouse(x, y, tea.MouseLeft)

	assert.True(t, result.Dismissed)
	assert.Equal(t, "attach", result.Action)
}

func TestTmuxBrowserOverlay_HandleMouse_TitleTruncatedAtMaxWidth(t *testing.T) {
	// Title exceeds tmuxBrowserMaxTitleWidth — should be truncated at the cap.
	title := strings.Repeat("x", tmuxBrowserMaxTitleWidth+10)
	items := []TmuxBrowserItem{{Name: "sess", Title: title, Created: time.Now()}}
	b := NewTmuxBrowserOverlay(items)

	assert.Equal(t, tmuxBrowserMaxTitleWidth, b.titleColWidth)
	renderedTitle := truncateStr(title, tmuxBrowserMaxTitleWidth)
	x, y := tmuxBrowserMouseTarget(t, b.View(), renderedTitle)

	result := b.HandleMouse(x, y, tea.MouseLeft)

	assert.True(t, result.Dismissed)
	assert.Equal(t, "attach", result.Action)
}

func TestTmuxBrowserOverlay_HandleMouse_PrefixMatchUsesClickedRow(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "sess-a", Title: "a", Created: time.Now()},
		{Name: "sess-alpha", Title: "alpha", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	x, y := tmuxBrowserMouseTarget(t, b.View(), "alpha")

	result := b.HandleMouse(x, y, tea.MouseLeft)

	assert.True(t, result.Dismissed)
	assert.Equal(t, "attach", result.Action)
	assert.Equal(t, 1, b.selectedIdx)
}

func TestTmuxBrowserOverlay_AutoWidthExpandsForLongTitle(t *testing.T) {
	// "short" (5 chars) uses tmuxBrowserMinTitleWidth (20) and hits the
	// minInnerWidth floor, so its modal is narrow.
	// The long title (34 chars) pushes titleColWidth+fixedRowCost above the
	// floor, making the modal measurably wider.
	shortItems := []TmuxBrowserItem{
		{Name: "kas_short", Title: "short", Created: time.Now()},
	}
	// 34 chars — long enough that titleColWidth(34)+fixedRowCost(21)=55 > minInnerWidth(54).
	longTitle := "api-response-logging-review-long-1"
	longItems := []TmuxBrowserItem{
		{Name: "kas_long", Title: longTitle, Created: time.Now()},
	}

	bShort := NewTmuxBrowserOverlay(shortItems)
	bLong := NewTmuxBrowserOverlay(longItems)

	assert.Greater(t, bLong.width, bShort.width,
		"modal should be wider when session names are longer")
	assert.Contains(t, stripANSI(bLong.View()), longTitle,
		"full session title must be visible without truncation")
}

func TestTmuxBrowserOverlay_AutoWidthMinimumForShortTitles(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "short", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	// Even with a short title the modal must be at least wide enough for the hint.
	innerWidth := b.width - tmuxBrowserBorderCost
	assert.GreaterOrEqual(t, innerWidth, tmuxBrowserMinInnerWidth,
		"inner width must accommodate the hint line")
}

func TestTmuxBrowserOverlay_BadgeWidthIncludedInAutoWidth(t *testing.T) {
	// Use a title long enough that the row cost exceeds the minInnerWidth floor
	// both with and without a badge, so the badge actually widens the modal.
	// 34 chars: titleColWidth(34)+fixedRowCost(21)=55 (no badge), +badgeWidth(11)=66 (badge).
	longTitle := "api-response-logging-review-long-1"
	plain := []TmuxBrowserItem{
		{Name: "kas_plain", Title: longTitle, Created: time.Now(), Managed: false},
	}
	managed := []TmuxBrowserItem{
		{Name: "kas_managed", Title: longTitle, Created: time.Now(), Managed: true, AgentType: "reviewer"},
	}

	bPlain := NewTmuxBrowserOverlay(plain)
	bManaged := NewTmuxBrowserOverlay(managed)

	assert.Greater(t, bManaged.width, bPlain.width,
		"modal must be wider when badges are present")
}

func TestTmuxBrowserOverlay_SetSizeCapsWidth(t *testing.T) {
	// Create an overlay with a long title that would prefer a wide modal.
	items := []TmuxBrowserItem{
		{Name: "kas_long", Title: "api-response-logging-review-1", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	preferredWidth := b.width

	// Simulate a narrow terminal — width must be capped.
	narrowTerminal := preferredWidth - 5
	if narrowTerminal < 10 {
		narrowTerminal = 10
	}
	b.SetSize(narrowTerminal, 40)
	assert.LessOrEqual(t, b.width, narrowTerminal,
		"overlay must not exceed the terminal width")

	// A wide terminal must not shrink the computed preferred width.
	b.SetSize(preferredWidth+50, 40)
	assert.Equal(t, preferredWidth, b.width,
		"wide terminal must preserve content-derived width")
}
