package clickup_test

import (
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
)

func TestScaffoldPlan_BasicTask(t *testing.T) {
	task := clickup.Task{
		ID:          "abc123",
		Name:        "Design auth flow",
		Description: "Implement OAuth2 authentication for the API",
		Status:      "In Progress",
		Priority:    "High",
		URL:         "https://app.clickup.com/t/abc123",
		ListName:    "Backend",
	}

	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "**Goal:** Implement OAuth2 authentication for the API")
	assert.Contains(t, md, "**Source:** ClickUp abc123")
	assert.Contains(t, md, "https://app.clickup.com/t/abc123")
	assert.Contains(t, md, "**ClickUp Status:** In Progress")
}

func TestScaffoldPlan_WithSubtasks(t *testing.T) {
	task := clickup.Task{
		ID:          "def456",
		Name:        "Setup CI/CD",
		Description: "Configure CI pipeline",
		Subtasks: []clickup.Subtask{
			{Name: "Add Dockerfile", Status: "done"},
			{Name: "Configure GitHub Actions", Status: "open"},
		},
	}

	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "## Reference: ClickUp Subtasks")
	assert.Contains(t, md, "- [x] Add Dockerfile")
	assert.Contains(t, md, "- [ ] Configure GitHub Actions")
}

func TestScaffoldPlan_WithCustomFields(t *testing.T) {
	task := clickup.Task{
		ID:   "ghi789",
		Name: "Feature X",
		CustomFields: []clickup.CustomField{
			{Name: "Sprint", Value: "2026-W09"},
			{Name: "Story Points", Value: "5"},
		},
	}

	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "## Reference: Custom Fields")
	assert.Contains(t, md, "- **Sprint:** 2026-W09")
}

func TestScaffoldFilename(t *testing.T) {
	tests := map[string]string{
		"Design Auth Flow":       "2026-02-24-design-auth-flow.md",
		"API v2 — New Endpoints": "2026-02-24-api-v2-new-endpoints.md",
		"  spaces & symbols!!! ": "2026-02-24-spaces-symbols.md",
	}

	for input, want := range tests {
		got := clickup.ScaffoldFilename(input, "2026-02-24")
		assert.Equal(t, want, got, "input: %q", input)
	}
}
