package app

import (
	"testing"

	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSidebarStatusTestInstance(t *testing.T, planFile string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    "status-test",
		Path:     ".",
		Program:  "claude",
		TaskFile: planFile,
	})
	require.NoError(t, err)
	return inst
}

func TestMergeTopicStatus(t *testing.T) {
	inst := newSidebarStatusTestInstance(t, "")

	running := mergeTopicStatus(ui.TopicStatus{}, inst, true)
	assert.True(t, running.HasRunning)
	assert.False(t, running.HasNotification)

	inst.PromptDetected = true
	promptDetected := mergeTopicStatus(ui.TopicStatus{}, inst, true)
	assert.False(t, promptDetected.HasRunning)

	inst.Notified = true
	notified := mergeTopicStatus(ui.TopicStatus{}, inst, false)
	assert.True(t, notified.HasNotification)
	assert.False(t, notified.HasRunning)

	inst.Status = session.Paused
	paused := mergeTopicStatus(ui.TopicStatus{}, inst, true)
	assert.False(t, paused.HasRunning)
}

func TestMergePlanStatus(t *testing.T) {
	reviewer := newSidebarStatusTestInstance(t, "plan")
	reviewer.IsReviewer = true
	reviewer.Notified = true

	st := mergePlanStatus(ui.TopicStatus{}, reviewer, false)
	assert.True(t, st.HasNotification)
	assert.False(t, st.HasRunning)

	coder := newSidebarStatusTestInstance(t, "plan")
	st = mergePlanStatus(st, coder, true)
	assert.True(t, st.HasRunning)
	assert.True(t, st.HasNotification)

	pausedCoder := newSidebarStatusTestInstance(t, "plan")
	pausedCoder.Status = session.Paused
	paused := mergePlanStatus(ui.TopicStatus{}, pausedCoder, true)
	assert.False(t, paused.HasRunning)
	assert.False(t, paused.HasNotification)

	noPlan := newSidebarStatusTestInstance(t, "")
	existing := ui.TopicStatus{HasRunning: true}
	unchanged := mergePlanStatus(existing, noPlan, true)
	assert.Equal(t, existing, unchanged)
}

func TestComputeStatusBarData_Baseline(t *testing.T) {
	h := &home{
		activeRepoPath: "/home/user/repos/kasmos",
	}
	h.nav = ui.NewNavigationPanel(&h.spinner)

	data := h.computeStatusBarData()
	assert.Equal(t, "main", data.Branch)
}

func TestComputeStatusBarData_IncludesVersion(t *testing.T) {
	h := &home{activeRepoPath: "/home/user/repos/kasmos", version: "v2.0.0-beta-abc1234"}
	h.nav = ui.NewNavigationPanel(&h.spinner)
	data := h.computeStatusBarData()
	assert.Equal(t, "v2.0.0-beta-abc1234", data.Version)
}
