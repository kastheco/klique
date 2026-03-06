package orchestration

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/stretchr/testify/assert"
)

func TestBuildTaskPrompt(t *testing.T) {
	plan := &taskparser.Plan{
		Goal:         "Build a feature",
		Architecture: "Modular approach",
		TechStack:    "Go, bubbletea",
	}
	task := taskparser.Task{
		Number: 2,
		Title:  "Update Tests",
		Body:   "**Step 1:** Write the test\n\n**Step 2:** Run it",
	}

	prompt := BuildTaskPrompt(plan, task, 1, 3, 4)

	// Plan context
	assert.Contains(t, prompt, "Build a feature")
	assert.Contains(t, prompt, "Modular approach")
	assert.Contains(t, prompt, "Go, bubbletea")
	// Rules section must be inlined (no skill-load instruction)
	assert.NotContains(t, prompt, "Load the `kasmos-coder` skill")
	assert.Contains(t, prompt, "## Rules")

	// Task identity
	assert.Contains(t, prompt, "Task 2")
	assert.Contains(t, prompt, "Update Tests")
	assert.Contains(t, prompt, "Write the test")
	assert.Contains(t, prompt, "Wave 1 of 3")

	// Parallel awareness (multi-task)
	assert.Contains(t, prompt, "Task 2 of 4")
	assert.Contains(t, prompt, "3 other agents")
	assert.Contains(t, prompt, "NEVER run `git add .`")
	assert.Contains(t, prompt, "NEVER run `git stash`")
	assert.Contains(t, prompt, "NEVER run `git checkout --")
	assert.Contains(t, prompt, "formatters/linters")
	assert.Contains(t, prompt, "test failures in files outside your task")
	assert.Contains(t, prompt, "build failure caused by missing types")
	assert.Contains(t, prompt, "surgical changes")
}

func TestBuildTaskPrompt_InlineCoderRules(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Test feature"}
	task := taskparser.Task{Number: 1, Title: "Do thing", Body: "Make the change"}

	prompt := BuildTaskPrompt(plan, task, 1, 1, 1)

	assert.NotContains(t, prompt, "kasmos-coder")
	assert.NotContains(t, prompt, "cli-tools")
	assert.NotContains(t, prompt, "Load the")

	assert.Contains(t, prompt, "## Rules")
	assert.Contains(t, prompt, "git add <specific-files>")
	assert.Contains(t, prompt, "feat(task-N):")
	assert.Contains(t, prompt, "-run Test")
	assert.Contains(t, prompt, "go build ./...")
}

func TestBuildTaskPrompt_SingleTask(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Simple"}
	task := taskparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := BuildTaskPrompt(plan, task, 1, 1, 1)

	// Single task shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
	assert.NotContains(t, prompt, "NEVER run")
	assert.NotContains(t, prompt, "other agents")
	assert.NotContains(t, prompt, "build failure caused by missing types")
}

func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := BuildWaveAnnotationPrompt("my-feature.md")
	assert.Contains(t, prompt, "kas task show my-feature.md")
	assert.Contains(t, prompt, "## Wave")
	assert.Contains(t, prompt, "planner-finished-my-feature.md")
	assert.NotContains(t, prompt, "The plan at docs/plans/")
}

func TestBuildElaborationPrompt(t *testing.T) {
	prompt := BuildElaborationPrompt("my-feature.md")

	// Must reference the plan file for retrieval
	assert.Contains(t, prompt, "kas task show my-feature.md")
	// Must reference updating the plan
	assert.Contains(t, prompt, "kas task update-content my-feature.md")
	// Must reference the signal
	assert.Contains(t, prompt, "elaborator-finished-my-feature.md")
	// Must instruct to expand task bodies
	assert.Contains(t, prompt, "implementation detail")
	// Must instruct to preserve structure
	assert.Contains(t, prompt, "Preserve")
	// Must reference reading the codebase
	assert.Contains(t, prompt, "codebase")
}
