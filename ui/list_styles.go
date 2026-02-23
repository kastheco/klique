package ui

import (
	"github.com/charmbracelet/lipgloss"
)

const readyIcon = "● "
const pausedIcon = "\uf04c "
const completedIcon = "✓ "

var readyStyle = lipgloss.NewStyle().
	Foreground(ColorFoam)

var notifyStyle = lipgloss.NewStyle().
	Foreground(ColorRose)

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(ColorFoam)

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(ColorLove)

var pausedStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)

var completedStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Faint(true)

// completedTitleStyle renders the title line for implementation-complete instances.
var completedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(ColorMuted).
	Faint(true)

// dimmedTitleStyle is for non-highlighted instances when a filter is active.
var dimmedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(ColorMuted)

// dimmedDescStyle matches dimmedTitleStyle for the description line.
var dimmedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(ColorMuted)

// completedDescStyle renders the description/branch line for implementation-complete instances.
var completedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(ColorMuted).
	Faint(true)

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(ColorText)

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(ColorSubtle)

var evenRowTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(ColorSurface).
	Foreground(ColorText)

var evenRowDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(ColorSurface).
	Foreground(ColorSubtle)

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(ColorIris).
	Foreground(ColorBase)

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(ColorIris).
	Foreground(ColorBase)

// Active (unfocused) styles — muted version of selected
var activeTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(ColorOverlay).
	Foreground(ColorText)

var activeDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(ColorOverlay).
	Foreground(ColorText)

var mainTitle = lipgloss.NewStyle().
	Background(ColorIris).
	Foreground(ColorBase)

var autoYesStyle = lipgloss.NewStyle().
	Background(ColorGold).
	Foreground(ColorBase)

var resourceStyle = lipgloss.NewStyle().
	Foreground(ColorSubtle)

var activityStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)

// Status filter tab styles
var activeFilterTab = lipgloss.NewStyle().
	Background(ColorIris).
	Foreground(ColorBase).
	Padding(0, 1)

var inactiveFilterTab = lipgloss.NewStyle().
	Background(ColorOverlay).
	Foreground(ColorSubtle).
	Padding(0, 1)

// StatusFilter determines which instances are shown based on their status.
type StatusFilter int

const (
	StatusFilterAll    StatusFilter = iota // Show all instances
	StatusFilterActive                     // Show only non-paused instances
)

// SortMode determines how instances are ordered.
type SortMode int

const (
	SortNewest SortMode = iota // Most recently updated first (default)
	SortOldest                 // Oldest first
	SortName                   // Alphabetical by title
	SortStatus                 // Grouped by status: running, ready, paused
)

var sortModeLabels = map[SortMode]string{
	SortNewest: "Newest",
	SortOldest: "Oldest",
	SortName:   "Name",
	SortStatus: "Status",
}

var sortDropdownStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Padding(0, 1)

// listBorderStyle wraps the instance list in a subtle rounded border matching the sidebar.
var listBorderStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorOverlay).
	Padding(0, 1)

const branchIcon = "\uf126"

// waveBadgeStyle is used for wave task badges (e.g. "W1", "W2") in the instance list.
var waveBadgeStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Bold(false)
