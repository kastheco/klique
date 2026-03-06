package git

import (
	"strconv"
	"strings"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstore"
)

// PRBodyInputs collects metadata used to build a PR body.
type PRBodyInputs struct {
	PlanContent     string
	ReviewerSummary string
	ReviewCycle     int
	Subtasks        []taskstore.SubtaskEntry
	Changes         string
	Commits         string
	Stats           string
}

// BuildPRTitle derives a lowercase-friendly PR title from task metadata.
// Priority: task description, goal, then filename stem.
func BuildPRTitle(description, goal, planFile string) string {
	if trimmed := strings.TrimSpace(description); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(goal); trimmed != "" {
		return trimmed
	}
	return strings.TrimSuffix(planFile, ".md")
}

// BuildPRBody creates a lower-case, markdown-formatted PR body from plan metadata,
// subtask status, reviewer summary, and git snippets.
func BuildPRBody(inputs PRBodyInputs, entry taskstore.TaskEntry) string {
	var (
		goal         string
		architecture string
		techStack    string
	)

	if entry.Goal != "" {
		goal = entry.Goal
	}
	if parsed, err := taskparser.Parse(inputs.PlanContent); err == nil {
		if goal == "" && strings.TrimSpace(parsed.Goal) != "" {
			goal = parsed.Goal
		}
		if strings.TrimSpace(parsed.Architecture) != "" {
			architecture = parsed.Architecture
		}
		if strings.TrimSpace(parsed.TechStack) != "" {
			techStack = parsed.TechStack
		}
	}

	if inputs.ReviewCycle == 0 {
		inputs.ReviewCycle = entry.ReviewCycle
	}

	var sections []string
	appendSection := func(title string, body string) {
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			return
		}
		sections = append(sections, "## "+title+"\n\n"+trimmed)
	}

	appendSection("goal", goal)
	appendSection("architecture", architecture)
	appendSection("tech stack", techStack)

	if len(inputs.Subtasks) > 0 {
		var taskLines strings.Builder
		for _, subtask := range inputs.Subtasks {
			checked := "- [ ] "
			s := string(subtask.Status)
			if s == "complete" || s == "done" || s == "closed" {
				checked = "- [x] "
			}
			line := strings.TrimSpace(subtask.Title)
			if subtask.TaskNumber > 0 {
				line = "task " + strconv.Itoa(subtask.TaskNumber) + ": " + line
			}
			if line != "" {
				taskLines.WriteString(checked + line + "\n")
			}
		}
		appendSection("tasks", taskLines.String())
	}

	if inputs.ReviewCycle > 0 {
		appendSection("review cycle", strconv.Itoa(inputs.ReviewCycle))
	}
	appendSection("reviewer summary", inputs.ReviewerSummary)
	appendSection("changes", inputs.Changes)
	appendSection("commits", inputs.Commits)
	appendSection("stats", inputs.Stats)

	return strings.Join(sections, "\n\n")
}

// PRBodySections stores parsed git snippet sections from GeneratePRBody output.
type PRBodySections struct {
	Changes string
	Commits string
	Stats   string
}

// ParsePRBodySections splits a body created by GeneratePRBody into git section bodies.
func ParsePRBodySections(body string) PRBodySections {
	var sections PRBodySections

	var target *string
	appendLine := func(line string) {
		if target == nil {
			return
		}
		*target += line + "\n"
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		switch {
		case strings.HasPrefix(trimmed, "## changes"):
			target = &sections.Changes
		case strings.HasPrefix(trimmed, "## commits"):
			target = &sections.Commits
		case strings.HasPrefix(trimmed, "## stats"):
			target = &sections.Stats
		case trimmed == "":
			appendLine("\n")
		default:
			appendLine(line)
		}
	}

	sections.Changes = strings.TrimSpace(sections.Changes)
	sections.Commits = strings.TrimSpace(sections.Commits)
	sections.Stats = strings.TrimSpace(sections.Stats)
	return sections
}
