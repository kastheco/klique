package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const infoPaneTestPlanContent = `# Plan

**Goal:** improve the info tab display

## Wave 1

### Task 1: add goal field
populate goal from store

### Task 2: add lifecycle timestamps
populate planning_at, implementing_at

## Wave 2

### Task 3: add subtask progress
show all-wave subtask progress
`

// buildInfoPaneHome creates a home struct wired with taskState and orchestrator for plan "plan.md".
// The plan has 3 subtasks across 2 waves. Task 1 is complete, tasks 2 and 3 are pending.
func buildInfoPaneHome(t *testing.T) (*home, *taskstate.TaskState, taskstore.Store, *orchestration.WaveOrchestrator) {
	t.Helper()

	dir := t.TempDir()
	store := newTestStore(t)

	ps, err := newTestPlanStateWithStore(t, store, dir)
	require.NoError(t, err)

	const planFile = "plan.md"
	require.NoError(t, ps.Create(planFile, "info tab improvements", "plan/info-tab", "", time.Now()))
	require.NoError(t, ps.IngestContent(planFile, infoPaneTestPlanContent))

	// Set lifecycle timestamps via the store directly.
	planningTs := time.Date(2025, 1, 10, 9, 0, 0, 0, time.UTC)
	implementingTs := time.Date(2025, 1, 12, 10, 30, 0, 0, time.UTC)
	require.NoError(t, store.SetPhaseTimestamp("test", planFile, "planning", planningTs))
	require.NoError(t, store.SetPhaseTimestamp("test", planFile, "implementing", implementingTs))

	// Reload so in-memory entry has the timestamps.
	ps2, err := taskstate.Load(store, "test", dir)
	require.NoError(t, err)

	// Mark task 1 complete via store.
	require.NoError(t, store.UpdateSubtaskStatus("test", planFile, 1, taskstore.SubtaskStatusComplete))

	plan, err := taskparser.Parse(infoPaneTestPlanContent)
	require.NoError(t, err)

	orch := orchestration.NewWaveOrchestrator(planFile, plan)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		nav:               ui.NewNavigationPanel(&sp),
		menu:              ui.NewMenu(),
		auditPane:         ui.NewAuditPane(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		overlays:          overlay.NewManager(),
		activeRepoPath:    os.TempDir(),
		taskState:         ps2,
		taskStore:         store,
		taskStoreProject:  "test",
		waveOrchestrators: map[string]*orchestration.WaveOrchestrator{planFile: orch},
	}

	// Register plan in nav so selection works.
	h.nav.SetData([]ui.PlanDisplay{{Filename: planFile}}, nil, nil, nil, nil)

	return h, ps2, store, orch
}

// TestUpdateInfoPaneForPlanHeader_GoalPopulated verifies that PlanGoal is set.
func TestUpdateInfoPaneForPlanHeader_GoalPopulated(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	ok := h.nav.SelectByID(ui.SidebarPlanPrefix + "plan.md")
	require.True(t, ok, "must be able to select plan header")

	h.updateInfoPaneForPlanHeader()

	data := h.tabbedWindow.GetInfoData()
	assert.Equal(t, "improve the info tab display", data.PlanGoal)
}

// TestUpdateInfoPaneForPlanHeader_LifecycleTimestamps verifies that PlanningAt and ImplementingAt are set.
func TestUpdateInfoPaneForPlanHeader_LifecycleTimestamps(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	ok := h.nav.SelectByID(ui.SidebarPlanPrefix + "plan.md")
	require.True(t, ok)

	h.updateInfoPaneForPlanHeader()

	data := h.tabbedWindow.GetInfoData()
	assert.Equal(t, 2025, data.PlanningAt.Year())
	assert.Equal(t, time.Month(1), data.PlanningAt.Month())
	assert.Equal(t, 10, data.PlanningAt.Day())
	assert.Equal(t, 2025, data.ImplementingAt.Year())
	assert.Equal(t, 12, data.ImplementingAt.Day())
	assert.True(t, data.ReviewingAt.IsZero(), "ReviewingAt must be zero — not set")
}

// TestUpdateInfoPaneForPlanHeader_SubtaskProgress verifies CompletedTasks, TotalSubtasks, and AllWaveSubtasks.
func TestUpdateInfoPaneForPlanHeader_SubtaskProgress(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	ok := h.nav.SelectByID(ui.SidebarPlanPrefix + "plan.md")
	require.True(t, ok)

	h.updateInfoPaneForPlanHeader()

	data := h.tabbedWindow.GetInfoData()
	assert.Equal(t, 3, data.TotalSubtasks, "must count all 3 subtasks")
	assert.Equal(t, 1, data.CompletedTasks, "task 1 is complete")

	require.Len(t, data.AllWaveSubtasks, 2, "must have 2 wave groups")

	wave1 := data.AllWaveSubtasks[0]
	assert.Equal(t, 1, wave1.WaveNumber)
	require.Len(t, wave1.Subtasks, 2)
	assert.Equal(t, 1, wave1.Subtasks[0].Number)
	assert.Equal(t, "add goal field", wave1.Subtasks[0].Title)
	assert.Equal(t, "complete", wave1.Subtasks[0].Status)
	assert.Equal(t, 2, wave1.Subtasks[1].Number)
	assert.Equal(t, "add lifecycle timestamps", wave1.Subtasks[1].Title)

	wave2 := data.AllWaveSubtasks[1]
	assert.Equal(t, 2, wave2.WaveNumber)
	require.Len(t, wave2.Subtasks, 1)
	assert.Equal(t, 3, wave2.Subtasks[0].Number)
	assert.Equal(t, "add subtask progress", wave2.Subtasks[0].Title)
}

// TestUpdateInfoPane_InstanceView_GoalAndLifecycle verifies goal and timestamps populate in instance view.
func TestUpdateInfoPane_InstanceView_GoalAndLifecycle(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "coder-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   "plan.md",
		TaskNumber: 1,
		WaveNumber: 1,
		AgentType:  session.AgentTypeCoder,
	})
	require.NoError(t, err)

	h.nav.SetData([]ui.PlanDisplay{{Filename: "plan.md"}}, []*session.Instance{inst}, nil, nil, map[string]ui.TopicStatus{"plan.md": {}})
	ok := h.nav.SelectInstance(inst)
	require.True(t, ok)

	h.updateInfoPane()

	data := h.tabbedWindow.GetInfoData()
	assert.True(t, data.HasInstance)
	assert.Equal(t, "improve the info tab display", data.PlanGoal)
	assert.False(t, data.PlanningAt.IsZero(), "PlanningAt must be set")
	assert.False(t, data.ImplementingAt.IsZero(), "ImplementingAt must be set")
}

// TestUpdateInfoPane_InstanceView_TaskTitle verifies TaskTitle is populated from the plan.
func TestUpdateInfoPane_InstanceView_TaskTitle(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "coder-T2",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   "plan.md",
		TaskNumber: 2,
		WaveNumber: 1,
		AgentType:  session.AgentTypeCoder,
	})
	require.NoError(t, err)

	h.nav.SetData([]ui.PlanDisplay{{Filename: "plan.md"}}, []*session.Instance{inst}, nil, nil, map[string]ui.TopicStatus{"plan.md": {}})
	ok := h.nav.SelectInstance(inst)
	require.True(t, ok)

	h.updateInfoPane()

	data := h.tabbedWindow.GetInfoData()
	assert.Equal(t, "add lifecycle timestamps", data.TaskTitle)
}

// TestUpdateInfoPane_InstanceView_SubtaskProgress verifies subtask progress in instance view.
func TestUpdateInfoPane_InstanceView_SubtaskProgress(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    "coder-T1",
		Path:     t.TempDir(),
		Program:  "claude",
		TaskFile: "plan.md",
	})
	require.NoError(t, err)

	h.nav.SetData([]ui.PlanDisplay{{Filename: "plan.md"}}, []*session.Instance{inst}, nil, nil, map[string]ui.TopicStatus{"plan.md": {}})
	ok := h.nav.SelectInstance(inst)
	require.True(t, ok)

	h.updateInfoPane()

	data := h.tabbedWindow.GetInfoData()
	assert.Equal(t, 3, data.TotalSubtasks)
	assert.Equal(t, 1, data.CompletedTasks)
	assert.Len(t, data.AllWaveSubtasks, 2)
}

// TestUpdateInfoPane_SubtaskReadFailure_PreservesSubtaskFields verifies that when GetSubtasks
// fails, prior subtask fields are preserved and not zeroed out.
func TestUpdateInfoPane_SubtaskReadFailure_PreservesSubtaskFields(t *testing.T) {
	h, _, _, _ := buildInfoPaneHome(t)

	// Replace the taskState's store with one that will fail GetSubtasks.
	// We do this by replacing taskState with a version that has no store.
	// Simplest: build a home with a broken store wrapped in taskState that errors on GetSubtasks.
	failStore := &failingSubtaskStore{inner: newTestStore(t)}
	dir := t.TempDir()
	ps, err := taskstate.Load(failStore, "test", dir)
	require.NoError(t, err)

	// Manually insert plan entry into ps so Entry() returns it.
	ps.Plans = map[string]taskstate.TaskEntry{
		"plan.md": {
			Status: taskstate.StatusImplementing,
			Goal:   "test goal",
		},
	}

	h.taskState = ps

	// Pre-set info data with subtask fields via SetInfoData so the home has prior data.
	prior := ui.InfoData{
		HasInstance:     true,
		TotalSubtasks:   5,
		CompletedTasks:  2,
		AllWaveSubtasks: []ui.WaveSubtaskGroup{{WaveNumber: 1, Subtasks: []ui.SubtaskDisplay{{Number: 1, Title: "old", Status: "complete"}}}},
	}
	h.tabbedWindow.SetInfoData(prior)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    "coder",
		Path:     t.TempDir(),
		Program:  "claude",
		TaskFile: "plan.md",
	})
	require.NoError(t, err)

	h.nav.SetData([]ui.PlanDisplay{{Filename: "plan.md"}}, []*session.Instance{inst}, nil, nil, map[string]ui.TopicStatus{"plan.md": {}})
	ok := h.nav.SelectInstance(inst)
	require.True(t, ok)

	h.updateInfoPane()

	data := h.tabbedWindow.GetInfoData()
	// Subtask fields must be preserved from prior data, not zeroed.
	assert.Equal(t, 5, data.TotalSubtasks, "TotalSubtasks must be preserved on GetSubtasks error")
	assert.Equal(t, 2, data.CompletedTasks, "CompletedTasks must be preserved on GetSubtasks error")
	assert.Len(t, data.AllWaveSubtasks, 1, "AllWaveSubtasks must be preserved on GetSubtasks error")
}

// failingSubtaskStore wraps a Store and returns an error on GetSubtasks.
type failingSubtaskStore struct {
	inner taskstore.Store
}

func (f *failingSubtaskStore) Create(project string, entry taskstore.TaskEntry) error {
	return f.inner.Create(project, entry)
}
func (f *failingSubtaskStore) Get(project, filename string) (taskstore.TaskEntry, error) {
	return f.inner.Get(project, filename)
}
func (f *failingSubtaskStore) Update(project, filename string, entry taskstore.TaskEntry) error {
	return f.inner.Update(project, filename, entry)
}
func (f *failingSubtaskStore) Rename(project, oldFilename, newFilename string) error {
	return f.inner.Rename(project, oldFilename, newFilename)
}
func (f *failingSubtaskStore) GetContent(project, filename string) (string, error) {
	return f.inner.GetContent(project, filename)
}
func (f *failingSubtaskStore) SetContent(project, filename, content string) error {
	return f.inner.SetContent(project, filename, content)
}
func (f *failingSubtaskStore) SetSubtasks(project, filename string, subtasks []taskstore.SubtaskEntry) error {
	return f.inner.SetSubtasks(project, filename, subtasks)
}
func (f *failingSubtaskStore) GetSubtasks(project, filename string) ([]taskstore.SubtaskEntry, error) {
	return nil, fmt.Errorf("subtask store unavailable")
}
func (f *failingSubtaskStore) UpdateSubtaskStatus(project, filename string, taskNumber int, status taskstore.SubtaskStatus) error {
	return f.inner.UpdateSubtaskStatus(project, filename, taskNumber, status)
}
func (f *failingSubtaskStore) SetPhaseTimestamp(project, filename, phase string, ts time.Time) error {
	return f.inner.SetPhaseTimestamp(project, filename, phase, ts)
}
func (f *failingSubtaskStore) SetClickUpTaskID(project, filename, taskID string) error {
	return f.inner.SetClickUpTaskID(project, filename, taskID)
}
func (f *failingSubtaskStore) IncrementReviewCycle(project, filename string) error {
	return f.inner.IncrementReviewCycle(project, filename)
}
func (f *failingSubtaskStore) SetPlanGoal(project, filename, goal string) error {
	return f.inner.SetPlanGoal(project, filename, goal)
}
func (f *failingSubtaskStore) List(project string) ([]taskstore.TaskEntry, error) {
	return f.inner.List(project)
}
func (f *failingSubtaskStore) ListByStatus(project string, statuses ...taskstore.Status) ([]taskstore.TaskEntry, error) {
	return f.inner.ListByStatus(project, statuses...)
}
func (f *failingSubtaskStore) ListByTopic(project, topic string) ([]taskstore.TaskEntry, error) {
	return f.inner.ListByTopic(project, topic)
}
func (f *failingSubtaskStore) ListTopics(project string) ([]taskstore.TopicEntry, error) {
	return f.inner.ListTopics(project)
}
func (f *failingSubtaskStore) CreateTopic(project string, entry taskstore.TopicEntry) error {
	return f.inner.CreateTopic(project, entry)
}
func (f *failingSubtaskStore) SetPRURL(project, filename, url string) error {
	return f.inner.SetPRURL(project, filename, url)
}
func (f *failingSubtaskStore) SetPRState(project, filename, reviewDecision, checkStatus string) error {
	return f.inner.SetPRState(project, filename, reviewDecision, checkStatus)
}
func (f *failingSubtaskStore) RecordPRReview(project, filename string, reviewID int, state, body, reviewer string) error {
	return f.inner.RecordPRReview(project, filename, reviewID, state, body, reviewer)
}
func (f *failingSubtaskStore) IsReviewProcessed(project, filename string, reviewID int) bool {
	return f.inner.IsReviewProcessed(project, filename, reviewID)
}
func (f *failingSubtaskStore) MarkReviewReacted(project, filename string, reviewID int) error {
	return f.inner.MarkReviewReacted(project, filename, reviewID)
}
func (f *failingSubtaskStore) MarkReviewFixerDispatched(project, filename string, reviewID int) error {
	return f.inner.MarkReviewFixerDispatched(project, filename, reviewID)
}
func (f *failingSubtaskStore) ListPendingReviews(project, filename string) ([]taskstore.PRReviewEntry, error) {
	return f.inner.ListPendingReviews(project, filename)
}
func (f *failingSubtaskStore) Ping() error  { return f.inner.Ping() }
func (f *failingSubtaskStore) Close() error { return f.inner.Close() }
