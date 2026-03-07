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

	prompt := BuildTaskPrompt("feature.md", plan, task, 1, 3, 4, nil)

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
	assert.Contains(t, prompt, "implement-task-finished-w1-t2-feature.md")
}

func TestBuildTaskPrompt_InlineCoderRules(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Test feature"}
	task := taskparser.Task{Number: 1, Title: "Do thing", Body: "Make the change"}

	prompt := BuildTaskPrompt("feature.md", plan, task, 1, 1, 1, nil)

	assert.NotContains(t, prompt, "kasmos-coder")
	assert.NotContains(t, prompt, "cli-tools")
	assert.NotContains(t, prompt, "Load the")

	assert.Contains(t, prompt, "## Rules")
	assert.Contains(t, prompt, "git add <specific-files>")
	assert.Contains(t, prompt, "feat(task-N):")
	assert.Contains(t, prompt, "-run Test")
	assert.Contains(t, prompt, "go build ./...")
	// Primary gateway command
	assert.Contains(t, prompt, "kas signal emit implement_task_finished feature.md")
	// Fallback filesystem sentinel still present
	assert.Contains(t, prompt, "touch .kasmos/signals/implement-task-finished-w1-t1-feature.md")
}

func TestBuildTaskPrompt_ContainsSignalEmit(t *testing.T) {
	plan := &taskparser.Plan{Waves: []taskparser.Wave{{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "test", Body: "do stuff"}}}}}
	prompt := BuildTaskPrompt("my-plan", plan, plan.Waves[0].Tasks[0], 1, 1, 1, nil)
	assert.Contains(t, prompt, "kas signal emit implement_task_finished my-plan")
	assert.Contains(t, prompt, "implement-task-finished-w1-t1-my-plan")
}

func TestBuildTaskPrompt_SingleTask(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Simple"}
	task := taskparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := BuildTaskPrompt("feature.md", plan, task, 1, 1, 1, nil)

	// Single task shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
	assert.NotContains(t, prompt, "NEVER run")
	assert.NotContains(t, prompt, "other agents")
	assert.NotContains(t, prompt, "build failure caused by missing types")
}

func TestBuildTaskPrompt_WithMeta(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Feature X", Architecture: "Modular", TechStack: "Go"}
	task := taskparser.Task{Number: 1, Title: "Add widget", Body: "Implement the widget"}
	meta := &TaskMeta{
		TaskNumber:     1,
		VerifyChecks:   []string{"go test ./widget/... -v", "go vet ./widget/..."},
		ContextRefs:    []string{"ref://widget-interface"},
		PreferredModel: "openai/gpt-5.3-codex-spark",
	}

	prompt := BuildTaskPrompt("feat.md", plan, task, 1, 2, 1, meta)

	assert.Contains(t, prompt, "go test ./widget/... -v")
	assert.Contains(t, prompt, "go vet ./widget/...")
	assert.Contains(t, prompt, "## Verification Commands")
	assert.NotContains(t, prompt, "ref://widget-interface")
	assert.NotContains(t, prompt, "openai/gpt-5.3-codex-spark")
}

func TestBuildTaskPrompt_NilMeta(t *testing.T) {
	plan := &taskparser.Plan{Goal: "Simple"}
	task := taskparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := BuildTaskPrompt("feat.md", plan, task, 1, 1, 1, nil)

	assert.NotContains(t, prompt, "## Verification Commands")
	assert.Contains(t, prompt, "## Rules")
	assert.Contains(t, prompt, "Task 1")
}

func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := BuildWaveAnnotationPrompt("my-feature")
	assert.Contains(t, prompt, "kas task show my-feature")
	assert.Contains(t, prompt, "## Wave")
	// Primary gateway command
	assert.Contains(t, prompt, "kas signal emit planner_finished my-feature")
	// Fallback filesystem sentinel still present
	assert.Contains(t, prompt, "planner-finished-my-feature")
	assert.NotContains(t, prompt, "The plan at docs/plans/")
}

func TestBuildMasterReviewPrompt(t *testing.T) {
	prompt := BuildMasterReviewPrompt("my-feature", "diff content here", "PASS: 42 tests")

	assert.Contains(t, prompt, "my-feature")
	assert.Contains(t, prompt, "diff content here")
	assert.Contains(t, prompt, "PASS: 42 tests")
	assert.Contains(t, prompt, "kasmos-master")
	assert.Contains(t, prompt, "master-approved-my-feature")
	assert.Contains(t, prompt, "## Test Results")
	assert.Contains(t, prompt, "## Diff")
}

func TestBuildElaborationPrompt(t *testing.T) {
	prompt := BuildElaborationPrompt("my-feature")

	// Must reference the plan file for retrieval
	assert.Contains(t, prompt, "kas task show my-feature")
	// Must reference updating the plan
	assert.Contains(t, prompt, "kas task update-content my-feature")
	// Primary gateway command
	assert.Contains(t, prompt, "kas signal emit elaborator_finished my-feature")
	// Fallback filesystem sentinel still present
	assert.Contains(t, prompt, "elaborator-finished-my-feature")
	// Must instruct to expand task bodies
	assert.Contains(t, prompt, "implementation detail")
	// Must instruct to preserve structure
	assert.Contains(t, prompt, "Preserve")
	// Must reference reading the codebase
	assert.Contains(t, prompt, "codebase")
}

func TestBuildArchitectPrompt(t *testing.T) {
	prompt := BuildArchitectPrompt("my-feature")

	assert.Contains(t, prompt, "kasmos-architect")
	assert.Contains(t, prompt, "kas task show my-feature")
	assert.Contains(t, prompt, "kas task update-content my-feature")
	assert.Contains(t, prompt, "architect-finished-my-feature")
	assert.Contains(t, prompt, "architect-v1.json")
	assert.Contains(t, prompt, "parallel")
	// BuildArchitectPrompt intentionally remains a filesystem-only (touch) prompt
	// until a gateway consumer for architect-finished is implemented.
	assert.NotContains(t, prompt, "kas signal emit architect_finished")
}
