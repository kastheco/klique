package git

import (
	"strconv"
	"strings"
)

// PRMetadata stores parsed plan content and git summary details used to build a PR body.
type PRMetadata struct {
	Description     string
	Goal            string
	Architecture    string
	TechStack       string
	ReviewCycle     int
	Subtasks        []PRSubtask
	ReviewerSummary string
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

// BuildPRTitle derives a lowercase-friendly PR title from task metadata.
// Priority: task description, then filename stem.
func BuildPRTitle(description, planFile string) string {
	if trimmed := strings.TrimSpace(description); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(planFile); trimmed != "" {
		return trimmed
	}
	return "update"
}

// BuildPRBody creates a lower-case, markdown-formatted PR body from PR metadata.
func BuildPRBody(meta PRMetadata) string {
	var sections []string

	description := strings.TrimSpace(meta.Description)
	if description == "" {
		description = "update"
	}

	summaryLines := []string{
		"- description: " + description,
	}
	if trimmed := strings.TrimSpace(meta.Goal); trimmed != "" {
		summaryLines = append(summaryLines, "- goal: "+trimmed)
	}
	if trimmed := strings.TrimSpace(meta.Architecture); trimmed != "" {
		summaryLines = append(summaryLines, "- architecture: "+trimmed)
	}
	if trimmed := strings.TrimSpace(meta.TechStack); trimmed != "" {
		summaryLines = append(summaryLines, "- tech stack: "+trimmed)
	}
	if meta.ReviewCycle > 0 {
		summaryLines = append(summaryLines, "- review cycle: "+strconv.Itoa(meta.ReviewCycle))
	}

	sections = append(sections, "## summary\n\n"+strings.Join(summaryLines, "\n"))

	appendSection := func(title string, body string) {
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			return
		}
		sections = append(sections, "## "+title+"\n\n"+trimmed)
	}

	if len(meta.Subtasks) > 0 {
		var taskLines strings.Builder
		for _, subtask := range meta.Subtasks {
			checked := "- [ ] "
			s := strings.TrimSpace(strings.ToLower(string(subtask.Status)))
			if s == "complete" || s == "done" || s == "closed" {
				checked = "- [x] "
			}
			taskLines.WriteString(
				checked + strconv.Itoa(subtask.Number) + ". " + strings.TrimSpace(subtask.Title) + "\n",
			)
		}
		appendSection("tasks", taskLines.String())
	}

	appendSection("reviewer notes", meta.ReviewerSummary)
	appendSection("changes", meta.GitChanges)
	appendSection("commits", meta.GitCommits)
	appendSection("stats", meta.GitStats)

	return strings.Join(sections, "\n\n")
}
