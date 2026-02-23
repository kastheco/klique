// Package planparser extracts wave/task structure from plan markdown files.
package planparser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Task represents a single task extracted from a plan.
type Task struct {
	Number int    // Task number (1-indexed, from ### Task N: Title)
	Title  string // Task title (text after "Task N: ")
	Body   string // Full task body (everything between this ### Task and the next heading)
}

// Wave represents a group of tasks that can run in parallel.
type Wave struct {
	Number int    // Wave number (1-indexed)
	Tasks  []Task // Tasks in this wave
}

// Plan represents a parsed plan with header metadata and wave-grouped tasks.
type Plan struct {
	Goal         string
	Architecture string
	TechStack    string
	Waves        []Wave
}

// HeaderContext returns the plan header as a string suitable for task prompts.
func (p *Plan) HeaderContext() string {
	var sb strings.Builder
	if p.Goal != "" {
		sb.WriteString("**Goal:** " + p.Goal + "\n")
	}
	if p.Architecture != "" {
		sb.WriteString("**Architecture:** " + p.Architecture + "\n")
	}
	if p.TechStack != "" {
		sb.WriteString("**Tech Stack:** " + p.TechStack + "\n")
	}
	return sb.String()
}

var (
	waveHeaderRe = regexp.MustCompile(`(?m)^## Wave (\d+)\s*$`)
	taskHeaderRe = regexp.MustCompile(`(?m)^### Task (\d+):\s*(.+)$`)
	goalRe       = regexp.MustCompile(`(?m)^\*\*Goal:\*\*\s*(.+)$`)
	archRe       = regexp.MustCompile(`(?m)^\*\*Architecture:\*\*\s*(.+)$`)
	techRe       = regexp.MustCompile(`(?m)^\*\*Tech Stack:\*\*\s*(.+)$`)
)

// Parse extracts waves and tasks from plan markdown content.
// Returns an error if no ## Wave headers are found.
func Parse(content string) (*Plan, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("empty plan content")
	}

	plan := &Plan{}

	// Extract header fields
	if m := goalRe.FindStringSubmatch(content); len(m) > 1 {
		plan.Goal = strings.TrimSpace(m[1])
	}
	if m := archRe.FindStringSubmatch(content); len(m) > 1 {
		plan.Architecture = strings.TrimSpace(m[1])
	}
	if m := techRe.FindStringSubmatch(content); len(m) > 1 {
		plan.TechStack = strings.TrimSpace(m[1])
	}

	// Find all wave header positions
	waveMatches := waveHeaderRe.FindAllStringSubmatchIndex(content, -1)
	if len(waveMatches) == 0 {
		return nil, fmt.Errorf("no wave headers found in plan; add ## Wave N sections before implementing")
	}

	// Split content into wave sections
	for i, wm := range waveMatches {
		waveNumStr := content[wm[2]:wm[3]]
		waveNum, _ := strconv.Atoi(waveNumStr)

		// Determine the section boundaries for this wave
		sectionStart := wm[1] // end of "## Wave N" line
		var sectionEnd int
		if i+1 < len(waveMatches) {
			sectionEnd = waveMatches[i+1][0] // start of next wave header
		} else {
			sectionEnd = len(content)
		}
		section := content[sectionStart:sectionEnd]

		// Extract tasks from this wave section
		tasks, err := parseTasks(section)
		if err != nil {
			return nil, fmt.Errorf("wave %d: %w", waveNum, err)
		}

		plan.Waves = append(plan.Waves, Wave{
			Number: waveNum,
			Tasks:  tasks,
		})
	}

	return plan, nil
}

// parseTasks extracts ### Task entries from a wave section.
func parseTasks(section string) ([]Task, error) {
	taskMatches := taskHeaderRe.FindAllStringSubmatchIndex(section, -1)
	if len(taskMatches) == 0 {
		return nil, nil
	}

	var tasks []Task
	for i, tm := range taskMatches {
		numStr := section[tm[2]:tm[3]]
		num, _ := strconv.Atoi(numStr)
		title := strings.TrimSpace(section[tm[4]:tm[5]])

		// Task body: from end of header line to start of next task (or end of section)
		bodyStart := tm[1]
		var bodyEnd int
		if i+1 < len(taskMatches) {
			bodyEnd = taskMatches[i+1][0]
		} else {
			bodyEnd = len(section)
		}
		body := strings.TrimSpace(section[bodyStart:bodyEnd])

		tasks = append(tasks, Task{
			Number: num,
			Title:  title,
			Body:   body,
		})
	}

	return tasks, nil
}
