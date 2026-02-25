package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeScrollTestSidebar(t *testing.T, nPlans int) *Sidebar {
	t.Helper()
	s := NewSidebar()
	s.SetSize(30, 20)

	plans := make([]PlanDisplay, nPlans)
	for i := range plans {
		plans[i] = PlanDisplay{
			Filename: fmt.Sprintf("2026-02-25-plan-%d.md", i),
			Status:   "done",
		}
	}
	// Pass as history (done plans go to history section)
	s.SetTopicsAndPlans(nil, nil, plans)
	// Expand history so rows are visible
	s.historyExpanded = true
	s.rebuildRows()
	return s
}

func TestSidebarScrollOffset_DownScrolls(t *testing.T) {
	s := makeScrollTestSidebar(t, 30)
	s.SetSize(30, 10) // short — forces scroll
	initial := s.scrollOffset
	for i := 0; i < 25; i++ {
		s.Down()
	}
	assert.Greater(t, s.scrollOffset, initial, "scrollOffset should increase when selection moves past bottom")
}

func TestSidebarScrollOffset_UpScrollsBack(t *testing.T) {
	s := makeScrollTestSidebar(t, 30)
	s.SetSize(30, 10)
	for i := 0; i < 25; i++ {
		s.Down()
	}
	for i := 0; i < 25; i++ {
		s.Up()
	}
	assert.Equal(t, 0, s.scrollOffset, "scrollOffset should return to 0 at top")
}

func TestSidebarScrollOffset_ResizeClamps(t *testing.T) {
	s := makeScrollTestSidebar(t, 30)
	s.SetSize(30, 10)
	for i := 0; i < 25; i++ {
		s.Down()
	}
	s.SetSize(30, 60) // taller — offset may be invalid
	assert.GreaterOrEqual(t, s.scrollOffset, 0, "scrollOffset must not go negative")
}

func TestSidebarString_DoesNotOverflowHeight(t *testing.T) {
	s := makeScrollTestSidebar(t, 30)
	s.SetSize(30, 12)
	rendered := s.String()
	lines := strings.Split(rendered, "\n")
	assert.LessOrEqual(t, len(lines), 12, "rendered sidebar must not exceed panel height")
}

func TestSidebarScrollOffset_RebuildResetsOffset(t *testing.T) {
	s := makeScrollTestSidebar(t, 30)
	s.SetSize(30, 10)
	for i := 0; i < 25; i++ {
		s.Down()
	}
	assert.Greater(t, s.scrollOffset, 0, "should be scrolled before rebuild")
	// Trigger rebuildRows via SetTopicsAndPlans
	s.SetTopicsAndPlans(nil, nil, nil)
	assert.Equal(t, 0, s.scrollOffset, "scrollOffset should reset to 0 after rebuild")
}
