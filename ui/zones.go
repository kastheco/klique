package ui

import "fmt"

// Zone ID constants for bubblezone hit detection.
// These are used both in render paths (zone.Mark) and input paths (zone.Get().InBounds).
const (
	ZoneNavPanel  = "zone-nav-panel"
	ZoneNavSearch = "zone-nav-search"
	ZoneTabAgent  = "zone-tab-agent"
	ZoneTabDiff   = "zone-tab-diff"
	ZoneTabInfo   = "zone-tab-info"
	ZoneAgentPane = "zone-agent-pane"
	ZoneViewPlan  = "zone-view-plan"
)

// TabZoneIDs maps tab index to zone ID.
// Tab order (post-reorder): InfoTab=0, PreviewTab=1, DiffTab=2.
var TabZoneIDs = [3]string{ZoneTabInfo, ZoneTabAgent, ZoneTabDiff}

// NavRowZoneID returns the zone ID for a navigation panel row by its rows-slice index.
func NavRowZoneID(idx int) string {
	return fmt.Sprintf("zone-nav-row-%d", idx)
}
