package ui

import (
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// ── Shared styles ─────────────────────────────────────────────────────────────
//
// These are used by both info_pane.go rendering (kept for backward compat with
// info_pane_test.go until it is removed) and nav_detail.go rendering.

var (
	infoSectionStyle = lipgloss.NewStyle().Foreground(ColorFoam).Bold(true)
	infoDividerStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	infoLabelStyle   = lipgloss.NewStyle().Foreground(ColorMuted).Width(20)
	infoValueStyle   = lipgloss.NewStyle().Foreground(ColorText)
)

// InfoData carries display data for the info / detail pane.
// Populated by the app layer from instance + plan + wave state.
type InfoData struct {
	// Instance fields
	Title   string
	Program string
	Branch  string
	Path    string
	Created string
	Status  string

	// Plan fields (empty when no plan is associated)
	PlanName        string
	PlanDescription string
	PlanStatus      string
	PlanGoal        string
	PlanTopic       string
	PlanBranch      string
	PlanCreated     string
	PlanningAt      time.Time
	ImplementingAt  time.Time
	ReviewingAt     time.Time
	DoneAt          time.Time

	// Plan summary fields (rendered when plan header row is selected)
	PlanInstanceCount int
	PlanRunningCount  int
	PlanReadyCount    int
	PlanPausedCount   int
	PlanAddedLines    int
	PlanRemovedLines  int
	CompletedTasks    int
	TotalSubtasks     int
	AllWaveSubtasks   []WaveSubtaskGroup

	// Resource utilisation
	CPUPercent float64
	MemMB      float64

	// Wave / task context (zero values mean no wave info)
	AgentType  string
	WaveNumber int
	TotalWaves int
	TaskNumber int
	TotalTasks int
	WaveTasks  []WaveTaskInfo
	TaskTitle  string

	// Review outcome (populated when plan is done)
	ReviewCycle        int
	ReviewOutcome      string
	MaxReviewFixCycles int

	// Selection state flags
	HasPlan              bool
	HasInstance          bool
	IsPlanHeaderSelected bool
}

// WaveTaskInfo describes a single task slot in the current wave.
type WaveTaskInfo struct {
	Number int
	State  string // "complete", "running", "failed", or "pending"
}

// SubtaskDisplay describes a parsed subtask in all-wave progress rendering.
type SubtaskDisplay struct {
	Number int
	Title  string
	Status string
}

// WaveSubtaskGroup groups subtasks by wave number.
type WaveSubtaskGroup struct {
	WaveNumber int
	Subtasks   []SubtaskDisplay
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// statusColor maps known status strings to palette colors.
func statusColor(status string) color.Color {
	switch status {
	case "implementing":
		return ColorIris
	case "planning", "running":
		return ColorFoam
	case "reviewing", "done":
		return ColorGold
	case "ready", "cancelled", "paused":
		return ColorMuted
	case "failed", "error":
		return ColorLove
	default:
		return ColorText
	}
}

// statusToGlyph maps status strings to a glyph and palette color.
func statusToGlyph(status string) (string, color.Color) {
	switch status {
	case "complete":
		return "✓", ColorFoam
	case "running":
		return "●", ColorIris
	case "failed":
		return "✗", ColorLove
	case "closed", "done":
		return "✓", ColorFoam
	default:
		return "○", ColorMuted
	}
}

// formatPhaseTime formats a phase timestamp for display.
func formatPhaseTime(ts time.Time) string {
	if ts.IsZero() {
		return "—"
	}
	return ts.Format("2006-01-02 15:04")
}

// asciiProgressBar renders a text-based progress bar.
func asciiProgressBar(total, done int) string {
	barWidth := 10
	if total <= 0 {
		return "[          ]"
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := int(float64(done) / float64(total) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled) + "]"
}
