package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/stretchr/testify/assert"
)

// TestPostClickUpProgressSkipsWithoutTaskID verifies that resolveClickUpTaskID
// returns "" when the plan has no ClickUp task ID field and no Source line in
// content — postClickUpProgress then returns nil (no-op).
func TestPostClickUpProgressSkipsWithoutTaskID(t *testing.T) {
	entry := planstate.PlanEntry{} // no ClickUpTaskID field
	taskID := resolveClickUpTaskID(entry, "# Plan without a source line\n\nNo clickup here.")
	assert.Equal(t, "", taskID)

	// postClickUpProgress with empty taskID must be a no-op
	cmd := postClickUpProgress(nil, taskID, "wave 1 complete")
	assert.Nil(t, cmd)
}

// TestPostClickUpProgressUsesFieldFirst verifies that the ClickUpTaskID field
// takes priority over the **Source:** ClickUp <ID> line in content.
func TestPostClickUpProgressUsesFieldFirst(t *testing.T) {
	entry := planstate.PlanEntry{ClickUpTaskID: "field123"}
	content := "**Source:** ClickUp content456 (https://app.clickup.com/t/content456)"

	taskID := resolveClickUpTaskID(entry, content)
	assert.Equal(t, "field123", taskID, "field value must take priority over parsed content")
	assert.NotEqual(t, "content456", taskID, "content-parsed ID must not be used when field is set")
}

// TestPostClickUpProgressFallsBackToContentParse verifies that when the
// ClickUpTaskID field is empty, the task ID is parsed from plan content.
func TestPostClickUpProgressFallsBackToContentParse(t *testing.T) {
	entry := planstate.PlanEntry{} // field empty
	content := "**Source:** ClickUp content789 (https://app.clickup.com/t/content789)"

	taskID := resolveClickUpTaskID(entry, content)
	assert.Equal(t, "content789", taskID, "task ID must be parsed from content when field is empty")
}

// TestSingleWavePlanSkipsWaveComment verifies that wave_complete comments are
// NOT posted for single-wave plans. Only multi-wave plans emit intermediate
// wave-complete comments; single-wave plans use the all-waves-complete event.
func TestSingleWavePlanSkipsWaveComment(t *testing.T) {
	singleWavePlan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Only task"}}},
		},
	}
	singleOrch := NewWaveOrchestrator("single-wave-plan.md", singleWavePlan)

	assert.False(t, shouldPostWaveCompleteComment(singleOrch),
		"single-wave plans must not emit intermediate wave_complete comments")

	multiWavePlan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2"}}},
		},
	}
	multiOrch := NewWaveOrchestrator("multi-wave-plan.md", multiWavePlan)

	assert.True(t, shouldPostWaveCompleteComment(multiOrch),
		"multi-wave plans must emit intermediate wave_complete comments")
}

// TestShouldPostWaveCompleteCommentNilOrch verifies nil-safety of the guard.
func TestShouldPostWaveCompleteCommentNilOrch(t *testing.T) {
	assert.False(t, shouldPostWaveCompleteComment(nil))
}
