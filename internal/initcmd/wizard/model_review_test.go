package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReviewStep_RendersSummary(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Temperature: "0.1", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "gpt-5.3-codex", Effort: "xhigh", Temperature: "0.2", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "claude-opus-4-6", Effort: "max", Temperature: "0.5", Enabled: false},
	}

	r := newReviewStep(agents, []string{"claude", "opencode"})
	view := r.View(100, 36)
	assert.Contains(t, view, "claude-sonnet-4-6")
	assert.Contains(t, view, "gpt-5.3-codex")
	assert.Contains(t, view, "disabled") // planner
}

func TestReviewStep_FormatSummaryLine(t *testing.T) {
	a := AgentState{
		Role: "coder", Harness: "claude",
		Model: "claude-sonnet-4-6", Effort: "medium",
		Temperature: "0.1", Enabled: true,
	}
	line := formatReviewLine(a)
	assert.Contains(t, line, "claude")
	assert.Contains(t, line, "claude-sonnet-4-6")
	assert.Contains(t, line, "medium")
}
