package ui

import "fmt"

// Zone ID constants for bubblezone hit detection.
// These are used both in render paths (zone.Mark) and input paths (zone.Get().InBounds).
const (
	ZoneNavPanel  = "zone-nav-panel"
	ZoneNavSearch = "zone-nav-search"
)

// NavRowZoneID returns the zone ID for a navigation panel row by its rows-slice index.
func NavRowZoneID(idx int) string {
	return fmt.Sprintf("zone-nav-row-%d", idx)
}
