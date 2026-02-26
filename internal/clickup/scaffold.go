package clickup

import (
	"fmt"
	"regexp"
	"strings"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// ScaffoldPlan generates a plan markdown from a ClickUp task.
func ScaffoldPlan(task Task) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", task.Name)

	if task.Description != "" {
		fmt.Fprintf(&b, "**Goal:** %s\n\n", task.Description)
	}

	if task.ID != "" {
		fmt.Fprintf(&b, "**Source:** ClickUp %s", task.ID)
		if task.URL != "" {
			fmt.Fprintf(&b, " (%s)", task.URL)
		}
		b.WriteString("\n\n")
	}

	if task.Status != "" {
		fmt.Fprintf(&b, "**ClickUp Status:** %s\n\n", task.Status)
	}

	if task.Priority != "" {
		fmt.Fprintf(&b, "**Priority:** %s\n\n", task.Priority)
	}

	if task.ListName != "" {
		fmt.Fprintf(&b, "**List:** %s\n\n", task.ListName)
	}

	if len(task.Subtasks) > 0 {
		b.WriteString("## Reference: ClickUp Subtasks\n\n")
		for _, st := range task.Subtasks {
			checkbox := "- [ ] "
			if isDone(st.Status) {
				checkbox = "- [x] "
			}
			fmt.Fprintf(&b, "%s%s\n", checkbox, st.Name)
		}
		b.WriteString("\n")
	}

	if len(task.CustomFields) > 0 {
		b.WriteString("## Reference: Custom Fields\n\n")
		for _, cf := range task.CustomFields {
			fmt.Fprintf(&b, "- **%s:** %s\n", cf.Name, cf.Value)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ScaffoldFilename generates a plan filename from a task name and date.
func ScaffoldFilename(name, date string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return fmt.Sprintf("%s-%s.md", date, slug)
}

func isDone(status string) bool {
	s := strings.ToLower(status)
	return s == "done" || s == "complete" || s == "completed" || s == "closed"
}
