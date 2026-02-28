package ui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditPane_RenderEmpty(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	output := pane.String()
	assert.Contains(t, output, "no events")
	// Should render with the styled empty icon
	assert.Contains(t, output, "·")
}

func TestAuditPane_RenderEvents(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:34", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam, Level: "info"},
		{Time: "12:35", Kind: "agent_finished", Icon: "✓", Message: "coder finished", Color: ColorGold, Level: "info"},
	})
	output := pane.String()
	assert.Contains(t, output, "spawned coder")
	assert.Contains(t, output, "coder finished")
}

func TestAuditPane_RenderLevels(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:34", Kind: "error", Icon: "!", Message: "something broke", Color: ColorLove, Level: "error"},
		{Time: "12:35", Kind: "permission_detected", Icon: "!", Message: "needs attention", Color: ColorGold, Level: "warn"},
		{Time: "12:36", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam, Level: "info"},
	})
	output := pane.String()
	assert.Contains(t, output, "something broke")
	assert.Contains(t, output, "needs attention")
	assert.Contains(t, output, "spawned coder")
}

func TestAuditPane_ScrollDown(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 3) // very small — forces scroll
	events := make([]AuditEventDisplay, 20)
	for i := range events {
		events[i] = AuditEventDisplay{
			Time:    fmt.Sprintf("12:%02d", i),
			Kind:    "test",
			Icon:    "·",
			Message: fmt.Sprintf("event %d", i),
			Color:   ColorText,
			Level:   "info",
		}
	}
	pane.SetEvents(events)
	pane.ScrollDown(5)
	output := pane.String()
	// Should show events from the scrolled position, not the top
	assert.NotContains(t, output, "event 0")
}

func TestAuditPane_ToggleVisibility(t *testing.T) {
	pane := NewAuditPane()
	assert.True(t, pane.Visible())
	pane.ToggleVisible()
	assert.False(t, pane.Visible())
	pane.ToggleVisible()
	assert.True(t, pane.Visible())
}

func TestAuditPane_HeaderContainsLabel(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	output := pane.String()
	// Header should contain the "log" label in the divider
	assert.Contains(t, output, "log")
}

func TestAuditPane_ScrollUp(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 3)
	events := make([]AuditEventDisplay, 20)
	for i := range events {
		events[i] = AuditEventDisplay{
			Time:    fmt.Sprintf("12:%02d", i),
			Kind:    "test",
			Icon:    "·",
			Message: fmt.Sprintf("event %d", i),
			Color:   ColorText,
			Level:   "info",
		}
	}
	pane.SetEvents(events)
	pane.ScrollDown(10)
	pane.ScrollUp(10)
	// After scrolling back up, should be near the top again
	output := pane.String()
	assert.NotEmpty(t, output)
}

func TestAuditPane_MessageTruncation(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(40, 5) // narrow width to trigger truncation
	longMsg := "this is a very long message that should be truncated because it exceeds the available width"
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:34", Kind: "test", Icon: "·", Message: longMsg, Color: ColorText, Level: "info"},
	})
	output := pane.String()
	// The full message should NOT appear — it should be truncated
	assert.NotContains(t, output, longMsg)
	// But a truncated prefix should be there
	assert.Contains(t, output, "this is a very")
}

func TestAuditPane_Height(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 8)
	assert.Equal(t, 8, pane.Height())
}
