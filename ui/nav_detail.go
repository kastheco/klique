package ui

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// NavDetailData bundles the InfoData and an optional glamour-rendered plan
// markdown for the nav panel's inline detail drill-down section.
// InfoData from info_pane.go is the canonical input shape.
type NavDetailData struct {
	InfoData     InfoData
	RenderedPlan string
}

// ---------- standalone rendering helpers ----------
//
// These are pure functions adapted from InfoPane's methods. They do not depend
// on InfoPane state — they take explicit width and InfoData arguments so they
// can be composed in NavigationPanel without importing InfoPane logic.

// detailRenderRow renders a single label+value row at the given width.
func detailRenderRow(label, value string, width int) string {
	valW := width - lipgloss.Width(infoLabelStyle.Render(label))
	if valW < 10 {
		valW = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		infoValueStyle.Width(valW).Render(value),
	)
}

// detailRenderStatusRow renders a label+value row with the value coloured by status.
func detailRenderStatusRow(label, value string, width int) string {
	valW := width - lipgloss.Width(infoLabelStyle.Render(label))
	if valW < 10 {
		valW = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		lipgloss.NewStyle().Foreground(statusColor(value)).Width(valW).Render(value),
	)
}

// detailRenderDivider renders a horizontal rule sized to width.
func detailRenderDivider(width int) string {
	w := width - 4
	if w < 10 {
		w = 10
	}
	return infoDividerStyle.Render(strings.Repeat("-", w))
}

// detailWrapText wraps text to fit within the label-adjusted column width at
// the given total width. Returns rendered rows (with an empty label prefix).
func detailWrapText(text string, width int) []string {
	if text == "" {
		return nil
	}
	maxWidth := width - lipgloss.Width(infoLabelStyle.Render("goal")) - 1
	if maxWidth < 10 {
		maxWidth = 10
	}
	var lines []string
	words := strings.Fields(text)
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= maxWidth {
			current += " " + word
		} else {
			lines = append(lines, detailRenderRow("", current, width))
			current = word
		}
	}
	if current != "" {
		lines = append(lines, detailRenderRow("", current, width))
	}
	return lines
}

// detailRenderGoal renders the goal section within the given width.
func detailRenderGoal(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("goal"),
		detailRenderDivider(width),
	}
	if data.PlanGoal == "" {
		return strings.Join(rows, "\n")
	}
	rows = append(rows, detailWrapText(data.PlanGoal, width)...)
	return strings.Join(rows, "\n")
}

// detailRenderLifecycle renders the lifecycle timestamps section.
func detailRenderLifecycle(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("lifecycle"),
		detailRenderDivider(width),
	}
	phases := []struct {
		label   string
		time    time.Time
		reached bool
	}{
		{"planning", data.PlanningAt, !data.PlanningAt.IsZero()},
		{"implementing", data.ImplementingAt, !data.ImplementingAt.IsZero()},
		{"reviewing", data.ReviewingAt, !data.ReviewingAt.IsZero()},
		{"done", data.DoneAt, !data.DoneAt.IsZero()},
	}
	for _, phase := range phases {
		glyph := "○"
		if phase.reached {
			glyph = "●"
		}
		rows = append(rows, detailRenderRow(phase.label, fmt.Sprintf("%s %s", glyph, formatPhaseTime(phase.time)), width))
	}
	return strings.Join(rows, "\n")
}

// detailRenderProgress renders the subtask progress section.
func detailRenderProgress(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("progress"),
		detailRenderDivider(width),
		detailRenderRow("subtasks", fmt.Sprintf("%d/%d %s",
			data.CompletedTasks, data.TotalSubtasks,
			asciiProgressBar(data.TotalSubtasks, data.CompletedTasks)), width),
	}

	groups := append([]WaveSubtaskGroup{}, data.AllWaveSubtasks...)
	sort.Slice(groups, func(i, j int) bool { return groups[i].WaveNumber < groups[j].WaveNumber })
	for _, group := range groups {
		rows = append(rows, detailRenderRow(fmt.Sprintf("wave %d", group.WaveNumber), "", width))
		for _, task := range group.Subtasks {
			glyph, col := statusToGlyph(task.Status)
			icon := lipgloss.NewStyle().Foreground(col).Render(glyph)
			taskLine := fmt.Sprintf("%s task %d: %s", icon, task.Number, task.Title)
			rows = append(rows, infoValueStyle.Render(taskLine))
		}
	}
	return strings.Join(rows, "\n")
}

// detailRenderPlanMeta renders the plan metadata block.
func detailRenderPlanMeta(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("plan"),
		detailRenderDivider(width),
	}
	if data.PlanName != "" {
		rows = append(rows, detailRenderRow("name", data.PlanName, width))
	}
	if data.PlanDescription != "" {
		rows = append(rows, detailRenderRow("description", data.PlanDescription, width))
	}
	if data.PlanStatus != "" {
		rows = append(rows, detailRenderStatusRow("status", data.PlanStatus, width))
	}
	if data.PlanTopic != "" {
		rows = append(rows, detailRenderRow("topic", data.PlanTopic, width))
	}
	if data.PlanBranch != "" {
		rows = append(rows, detailRenderRow("branch", data.PlanBranch, width))
	}
	if data.PlanCreated != "" {
		rows = append(rows, detailRenderRow("created", data.PlanCreated, width))
	}
	return strings.Join(rows, "\n")
}

// detailRenderReview renders the review outcome section.
func detailRenderReview(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("review"),
		detailRenderDivider(width),
	}
	if data.ReviewCycle > 0 {
		cycleLabel := fmt.Sprintf("%d", data.ReviewCycle)
		if data.MaxReviewFixCycles > 0 {
			cycleLabel = fmt.Sprintf("%d / %d", data.ReviewCycle, data.MaxReviewFixCycles)
		}
		rows = append(rows, detailRenderRow("cycle", cycleLabel, width))
	}
	rows = append(rows, detailRenderStatusRow("outcome", data.ReviewOutcome, width))
	return strings.Join(rows, "\n")
}

// detailRenderWaveSection renders the task-state grid for the current wave.
func detailRenderWaveSection(data InfoData, width int) string {
	rows := []string{
		infoSectionStyle.Render("wave progress"),
		detailRenderDivider(width),
	}
	for _, t := range data.WaveTasks {
		var glyph string
		var col color.Color
		switch t.State {
		case "complete":
			glyph, col = "✓", ColorFoam
		case "running":
			glyph, col = "●", ColorIris
		case "failed":
			glyph, col = "✗", ColorLove
		default:
			glyph, col = "○", ColorMuted
		}
		icon := lipgloss.NewStyle().Foreground(col).Render(glyph)
		rows = append(rows, infoLabelStyle.Render(fmt.Sprintf("task %d", t.Number))+icon+" "+t.State)
	}
	return strings.Join(rows, "\n")
}

// renderPlanHeaderDetail renders the full plan summary for a plan-header row.
// It mirrors InfoPane.renderPlanSummary but as a standalone function.
func renderPlanHeaderDetail(data InfoData, width int) string {
	var sections []string

	sections = append(sections, detailRenderPlanMeta(data, width))
	sections = append(sections, detailRenderGoal(data, width))
	sections = append(sections, detailRenderLifecycle(data, width))
	sections = append(sections, detailRenderProgress(data, width))

	if data.ReviewOutcome != "" {
		sections = append(sections, detailRenderReview(data, width))
	}

	if data.PlanInstanceCount > 0 {
		instanceSummary := fmt.Sprintf("%d", data.PlanInstanceCount)
		var parts []string
		if data.PlanRunningCount > 0 {
			parts = append(parts, fmt.Sprintf("%d running", data.PlanRunningCount))
		}
		if data.PlanReadyCount > 0 {
			parts = append(parts, fmt.Sprintf("%d ready", data.PlanReadyCount))
		}
		if data.PlanPausedCount > 0 {
			parts = append(parts, fmt.Sprintf("%d paused", data.PlanPausedCount))
		}
		if len(parts) > 0 {
			instanceSummary += " (" + strings.Join(parts, ", ") + ")"
		}
		instanceBlock := strings.Join([]string{
			infoSectionStyle.Render("instances"),
			detailRenderDivider(width),
			detailRenderRow("instances", instanceSummary, width),
		}, "\n")
		sections = append(sections, instanceBlock)
	}

	if len(data.WaveTasks) > 0 {
		sections = append(sections, detailRenderWaveSection(data, width))
	}

	return strings.Join(sections, "\n\n")
}

// renderInstanceDetail renders the instance metadata detail for an instance row.
// It mirrors InfoPane.renderInstanceSection (+ plan section) but as a standalone function.
func renderInstanceDetail(data InfoData, width int) string {
	var sections []string

	if data.HasPlan {
		sections = append(sections, detailRenderPlanMeta(data, width))
	}

	// Instance section
	rows := []string{
		infoSectionStyle.Render("instance"),
		detailRenderDivider(width),
	}
	if data.Title != "" {
		rows = append(rows, detailRenderRow("title", data.Title, width))
	}
	if data.AgentType != "" {
		rows = append(rows, detailRenderRow("role", data.AgentType, width))
	}
	if data.Program != "" {
		rows = append(rows, detailRenderRow("program", data.Program, width))
	}
	if data.Status != "" {
		rows = append(rows, detailRenderStatusRow("status", data.Status, width))
	}
	if data.Branch != "" {
		rows = append(rows, detailRenderRow("branch", data.Branch, width))
	}
	if data.Path != "" {
		rows = append(rows, detailRenderRow("path", data.Path, width))
	}
	if data.Created != "" {
		rows = append(rows, detailRenderRow("created", data.Created, width))
	}
	if data.PlanGoal != "" {
		rows = append(rows, detailRenderRow("goal", data.PlanGoal, width))
	}
	if data.WaveNumber > 0 {
		rows = append(rows, detailRenderRow("wave", fmt.Sprintf("%d/%d", data.WaveNumber, data.TotalWaves), width))
	}
	if data.TaskNumber > 0 {
		taskText := fmt.Sprintf("%d of %d", data.TaskNumber, data.TotalTasks)
		if data.TaskTitle != "" {
			taskText = fmt.Sprintf("%d of %d: %s", data.TaskNumber, data.TotalTasks, data.TaskTitle)
		}
		rows = append(rows, detailRenderRow("task", taskText, width))
	}
	if data.CPUPercent > 0 || data.MemMB > 0 {
		rows = append(rows, detailRenderRow("cpu", fmt.Sprintf("%.0f%%", math.Round(data.CPUPercent)), width))
		rows = append(rows, detailRenderRow("memory", fmt.Sprintf("%.0fM", data.MemMB), width))
	}
	sections = append(sections, strings.Join(rows, "\n"))

	if len(data.WaveTasks) > 0 {
		sections = append(sections, detailRenderWaveSection(data, width))
	}

	return strings.Join(sections, "\n\n")
}

// renderNavDetail produces the full detail section content from NavDetailData.
// Returns an empty string when the data does not correspond to a supporting row.
func renderNavDetail(data NavDetailData, width int) string {
	d := data.InfoData
	switch {
	case d.IsPlanHeaderSelected:
		return renderPlanHeaderDetail(d, width)
	case d.HasInstance:
		return renderInstanceDetail(d, width)
	default:
		return ""
	}
}
