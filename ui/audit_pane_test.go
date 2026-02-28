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
}

func TestAuditPane_RenderEvents(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:34", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam},
		{Time: "12:35", Kind: "agent_finished", Icon: "✓", Message: "coder finished", Color: ColorGold},
	})
	output := pane.String()
	assert.Contains(t, output, "spawned coder")
	assert.Contains(t, output, "coder finished")
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

func TestAuditPane_SetFilter(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 10)
	pane.SetFilter("my-plan.md")
	output := pane.String()
	// Header should contain the filter label
	assert.Contains(t, output, "my-plan.md")
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
		}
	}
	pane.SetEvents(events)
	pane.ScrollDown(10)
	pane.ScrollUp(10)
	// After scrolling back up, should be near the top again
	output := pane.String()
	assert.NotEmpty(t, output)
}
