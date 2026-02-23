package app

import (
	"testing"

	"github.com/kastheco/klique/config/planparser"
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

	prompt := buildTaskPrompt(plan, task, 1, 3)

	assert.Contains(t, prompt, "Build a feature")
	assert.Contains(t, prompt, "Modular approach")
	assert.Contains(t, prompt, "Go, bubbletea")
	assert.Contains(t, prompt, "Task 2")
	assert.Contains(t, prompt, "Update Tests")
	assert.Contains(t, prompt, "Write the test")
	assert.Contains(t, prompt, "cli-tools")
	assert.Contains(t, prompt, "Only modify the files listed in your task")
	assert.Contains(t, prompt, "Wave 1 of 3")
}

func TestBuildTaskPrompt_SingleWave(t *testing.T) {
	plan := &planparser.Plan{Goal: "Simple"}
	task := planparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := buildTaskPrompt(plan, task, 1, 1)

	// Single wave shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
}
