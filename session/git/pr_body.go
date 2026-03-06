package git

import (
	"strconv"
	"strings"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstore"
)

// PRMetadata stores parsed plan content and git summary details used to build a PR body.
type PRMetadata struct {
	Description     string
	Goal            string
	Architecture    string
	TechStack       string
	ReviewerSummary string
	ReviewCycle     int
	Subtasks        []PRSubtask
	GitChanges      string
	GitCommits      string
	GitStats        string
}

// PRSubtask stores an individual plan subtask entry for PR body rendering.
type PRSubtask struct {
	Number int
	Title  string
	Status string
}

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

// BuildPRBody creates a lower-case, markdown-formatted PR body from PR metadata.
func BuildPRBody(meta PRMetadata) string {
	var sections []string
	appendSection := func(title string, body string) {
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			return
		}
		sections = append(sections, "## "+title+"\n\n"+trimmed)
	}

	appendSection("goal", meta.Goal)
	appendSection("architecture", meta.Architecture)
	appendSection("tech stack", meta.TechStack)

	if len(meta.Subtasks) > 0 {
		var taskLines strings.Builder
		for _, subtask := range meta.Subtasks {
			checked := "- [ ] "
			s := string(subtask.Status)
			if s == "complete" || s == "done" || s == "closed" {
				checked = "- [x] "
			}
			line := strings.TrimSpace(subtask.Title)
			if subtask.Number > 0 {
				line = "task " + strconv.Itoa(subtask.Number) + ": " + line
			}
			if line != "" {
				taskLines.WriteString(checked + line + "\n")
			}
		}
		appendSection("tasks", taskLines.String())
	}

	if meta.ReviewCycle > 0 {
		appendSection("review cycle", strconv.Itoa(meta.ReviewCycle))
	}
	appendSection("reviewer summary", meta.ReviewerSummary)
	appendSection("changes", meta.GitChanges)
	appendSection("commits", meta.GitCommits)
	appendSection("stats", meta.GitStats)

	return strings.Join(sections, "\n\n")
}

// BuildPRBodyFromInputs builds a PR body from the legacy input format.
func BuildPRBodyFromInputs(inputs PRBodyInputs, entry taskstore.TaskEntry) string {
	meta := PRMetadata{
		Description:     strings.TrimSpace(entry.Description),
		Goal:            strings.TrimSpace(entry.Goal),
		ReviewerSummary: strings.TrimSpace(inputs.ReviewerSummary),
		ReviewCycle:     inputs.ReviewCycle,
		GitChanges:      strings.TrimSpace(inputs.Changes),
		GitCommits:      strings.TrimSpace(inputs.Commits),
		GitStats:        strings.TrimSpace(inputs.Stats),
		Subtasks:        make([]PRSubtask, 0, len(inputs.Subtasks)),
	}

	if meta.ReviewCycle == 0 {
		meta.ReviewCycle = entry.ReviewCycle
	}

	if entry.Goal == "" && strings.TrimSpace(inputs.PlanContent) != "" {
		if plan, err := taskparser.Parse(inputs.PlanContent); err == nil {
			if meta.Goal == "" && strings.TrimSpace(plan.Goal) != "" {
				meta.Goal = strings.TrimSpace(plan.Goal)
			}
			meta.Architecture = strings.TrimSpace(plan.Architecture)
			meta.TechStack = strings.TrimSpace(plan.TechStack)
		}
	}

	for _, subtask := range inputs.Subtasks {
		meta.Subtasks = append(meta.Subtasks, PRSubtask{
			Number: int(subtask.TaskNumber),
			Title:  strings.TrimSpace(subtask.Title),
			Status: string(subtask.Status),
		})
	}

	return BuildPRBody(meta)
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
