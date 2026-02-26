package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planparser"
	"github.com/stretchr/testify/assert"
)

func TestBuildTaskPrompt(t *testing.T) {
	plan := &planparser.Plan{
		Goal:         "Build a feature",
		Architecture: "Modular approach",
		TechStack:    "Go, bubbletea",
	}
	task := planparser.Task{
		Number: 2,
		Title:  "Update Tests",
		Body:   "**Step 1:** Write the test\n\n**Step 2:** Run it",
	}

	prompt := buildTaskPrompt(plan, task, 1, 3, 4)

	// Plan context
	assert.Contains(t, prompt, "Build a feature")
	assert.Contains(t, prompt, "Modular approach")
	assert.Contains(t, prompt, "Go, bubbletea")
	assert.Contains(t, prompt, "cli-tools")

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
	assert.Contains(t, prompt, "surgical changes")
}

func TestBuildTaskPrompt_SingleTask(t *testing.T) {
	plan := &planparser.Plan{Goal: "Simple"}
	task := planparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := buildTaskPrompt(plan, task, 1, 1, 1)

	// Single task shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
	assert.NotContains(t, prompt, "NEVER run")
	assert.NotContains(t, prompt, "other agents")
}
