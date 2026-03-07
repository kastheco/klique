package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostClickUpProgressSkipsWithoutTaskID verifies that resolveClickUpTaskID
// returns "" when the plan has no ClickUp task ID field and no Source line in
// content — postClickUpProgress then returns nil (no-op).
func TestPostClickUpProgressSkipsWithoutTaskID(t *testing.T) {
	entry := taskstate.TaskEntry{} // no ClickUpTaskID field
	taskID := resolveClickUpTaskID(entry, "# Plan without a source line\n\nNo clickup here.")
	assert.Equal(t, "", taskID)

	// postClickUpProgress with empty taskID must be a no-op
	cmd := postClickUpProgress(nil, taskID, "wave 1 complete")
	assert.Nil(t, cmd)
}

// TestPostClickUpProgressUsesFieldFirst verifies that the ClickUpTaskID field
// takes priority over the **Source:** ClickUp <ID> line in content.
func TestPostClickUpProgressUsesFieldFirst(t *testing.T) {
	entry := taskstate.TaskEntry{ClickUpTaskID: "field123"}
	content := "**Source:** ClickUp content456 (https://app.clickup.com/t/content456)"

	taskID := resolveClickUpTaskID(entry, content)
	assert.Equal(t, "field123", taskID, "field value must take priority over parsed content")
	assert.NotEqual(t, "content456", taskID, "content-parsed ID must not be used when field is set")
}

// TestPostClickUpProgressFallsBackToContentParse verifies that when the
// ClickUpTaskID field is empty, the task ID is parsed from plan content.
func TestPostClickUpProgressFallsBackToContentParse(t *testing.T) {
	entry := taskstate.TaskEntry{} // field empty
	content := "**Source:** ClickUp content789 (https://app.clickup.com/t/content789)"

	taskID := resolveClickUpTaskID(entry, content)
	assert.Equal(t, "content789", taskID, "task ID must be parsed from content when field is empty")
}

// TestBuildClickUpComment verifies the ClickUp progress comment formatter
// produces expected markdown for each event type.
func TestBuildClickUpComment(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		detail  string
		wantSub string
	}{
		{"plan_ready", "plan_ready", "3 tasks, 2 waves", "plan finalized"},
		{"wave_complete", "wave_complete", "wave 1/2: 3/3 tasks", "wave 1/2"},
		{"review_approved", "review_approved", "", "review approved"},
		{"review_changes", "review_changes_requested", "fix the tests", "changes requested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := buildClickUpProgressComment(tt.event, "my-feature", tt.detail)
			assert.Contains(t, comment, tt.wantSub)
		})
	}
}

// TestSingleWavePlanSkipsWaveComment verifies that wave_complete comments are
// NOT posted for single-wave plans. Only multi-wave plans emit intermediate
// wave-complete comments; single-wave plans use the all-waves-complete event.
func TestSingleWavePlanSkipsWaveComment(t *testing.T) {
	singleWavePlan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Only task"}}},
		},
	}
	singleOrch := orchestration.NewWaveOrchestrator("single-wave-plan", singleWavePlan)

	assert.False(t, singleOrch.ShouldPostWaveCompleteComment(),
		"single-wave plans must not emit intermediate wave_complete comments")

	multiWavePlan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2"}}},
		},
	}
	multiOrch := orchestration.NewWaveOrchestrator("multi-wave-plan", multiWavePlan)

	assert.True(t, multiOrch.ShouldPostWaveCompleteComment(),
		"multi-wave plans must emit intermediate wave_complete comments")
}

// TestShouldPostWaveCompleteCommentNilOrch verifies nil-safety of the guard.
func TestShouldPostWaveCompleteCommentNilOrch(t *testing.T) {
	var nilOrch *orchestration.WaveOrchestrator
	assert.False(t, nilOrch.ShouldPostWaveCompleteComment())
}

// TestFixerCompleteHook_ClearsPendingFeedback verifies that when an
// ImplementFinished signal fires while pendingReviewFeedback is set (meaning
// a fixer was spawned after ReviewChangesRequested), the feedback is
// cleared from pendingReviewFeedback and a tea.Cmd is returned for the
// fixer_complete ClickUp comment. Since home.postClickUpProgress returns nil
// when there is no commenter (click-up not configured), we verify the
// observable side-effect: pendingReviewFeedback is always cleared.
func TestFixerCompleteHook_ClearsPendingFeedback(t *testing.T) {
	const planFile = "fixer-hook"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	require.NoError(t, ps.Register(planFile, "fixer hook test", "plan/fixer-hook", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	coderInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "fixer-hook-implement",
		Path:      dir,
		Program:   "claude",
		TaskFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	_ = list.AddInstance(coderInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		nav:                   list,
		allInstances:          []*session.Instance{coderInst},
		menu:                  ui.NewMenu(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:          overlay.NewToastManager(&sp),
		taskState:             ps,
		taskStateDir:          plansDir,
		taskStore:             store,
		taskStoreProject:      "test",
		fsm:                   fsm,
		plannerPrompted:       make(map[string]bool),
		pendingReviewFeedback: map[string]string{planFile: "fix the auth logic"},
		waveOrchestrators:     make(map[string]*orchestration.WaveOrchestrator),
		instanceFinalizers:    make(map[*session.Instance]func()),
		activeRepoPath:        dir,
		program:               "claude",
	}

	// Fire an ImplementFinished signal with pendingReviewFeedback set.
	msg := metadataResultMsg{
		PlanState: ps,
		Signals: []taskfsm.Signal{
			{Event: taskfsm.ImplementFinished, TaskFile: planFile},
		},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// pendingReviewFeedback must be cleared — fixer_complete hook consumed it.
	_, hasFeedback := updated.pendingReviewFeedback[planFile]
	assert.False(t, hasFeedback,
		"pendingReviewFeedback must be cleared after ImplementFinished fires for a fixer")
}

// TestFixerCompleteHook_SkipsWhenNoFeedback verifies that the fixer_complete
// hook does NOT fire when ImplementFinished is for an original coder (no
// pending feedback). pendingReviewFeedback must remain empty/unchanged.
func TestFixerCompleteHook_SkipsWhenNoFeedback(t *testing.T) {
	const planFile = "no-feedback-hook"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	require.NoError(t, ps.Register(planFile, "no feedback test", "plan/no-feedback", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	coderInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "no-feedback-implement",
		Path:      dir,
		Program:   "claude",
		TaskFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	_ = list.AddInstance(coderInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		nav:                   list,
		allInstances:          []*session.Instance{coderInst},
		menu:                  ui.NewMenu(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:          overlay.NewToastManager(&sp),
		taskState:             ps,
		taskStateDir:          plansDir,
		taskStore:             store,
		taskStoreProject:      "test",
		fsm:                   fsm,
		plannerPrompted:       make(map[string]bool),
		pendingReviewFeedback: make(map[string]string), // no feedback for this plan
		waveOrchestrators:     make(map[string]*orchestration.WaveOrchestrator),
		instanceFinalizers:    make(map[*session.Instance]func()),
		activeRepoPath:        dir,
		program:               "claude",
	}

	// Fire ImplementFinished without any pending feedback.
	msg := metadataResultMsg{
		PlanState: ps,
		Signals: []taskfsm.Signal{
			{Event: taskfsm.ImplementFinished, TaskFile: planFile},
		},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// pendingReviewFeedback must still be empty — no fixer_complete hook fired.
	assert.Empty(t, updated.pendingReviewFeedback,
		"pendingReviewFeedback must remain empty when no feedback was pending")
}
