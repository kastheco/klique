package ui

import (
	"strings"
	"testing"

	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- renderNavDetail ----------

func TestRenderNavDetail_EmptyData_ReturnsEmpty(t *testing.T) {
	data := NavDetailData{}
	result := renderNavDetail(data, 60)
	assert.Empty(t, result, "empty NavDetailData should produce no output")
}

func TestRenderNavDetail_PlanHeaderSelected(t *testing.T) {
	data := NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "my-plan",
			PlanStatus:           "implementing",
			PlanGoal:             "build something great",
		},
	}
	result := renderNavDetail(data, 60)
	assert.Contains(t, result, "plan")
	assert.Contains(t, result, "my-plan")
	assert.Contains(t, result, "implementing")
	assert.Contains(t, result, "build something great")
}

func TestRenderNavDetail_InstanceSelected(t *testing.T) {
	data := NavDetailData{
		InfoData: InfoData{
			HasInstance: true,
			Title:       "worker-session",
			Status:      "running",
			AgentType:   "coder",
		},
	}
	result := renderNavDetail(data, 60)
	assert.Contains(t, result, "instance")
	assert.Contains(t, result, "worker-session")
	assert.Contains(t, result, "running")
	assert.Contains(t, result, "coder")
}

func TestRenderNavDetail_InstanceWithPlan(t *testing.T) {
	data := NavDetailData{
		InfoData: InfoData{
			HasInstance: true,
			HasPlan:     true,
			Title:       "coder-1",
			PlanName:    "my-plan",
			PlanStatus:  "implementing",
			Status:      "running",
		},
	}
	result := renderNavDetail(data, 60)
	assert.Contains(t, result, "plan")
	assert.Contains(t, result, "my-plan")
	assert.Contains(t, result, "instance")
}

func TestRenderNavDetail_PlanHeaderWithWaveTasks(t *testing.T) {
	data := NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "wave-plan",
			WaveTasks: []WaveTaskInfo{
				{Number: 1, State: "complete"},
				{Number: 2, State: "running"},
				{Number: 3, State: "pending"},
			},
		},
	}
	result := renderNavDetail(data, 60)
	assert.Contains(t, result, "wave progress")
	assert.Contains(t, result, "task 1")
	assert.Contains(t, result, "task 2")
}

func TestRenderPlanHeaderDetail_InstanceCounts(t *testing.T) {
	data := InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             "counted-plan",
		PlanInstanceCount:    3,
		PlanRunningCount:     1,
		PlanPausedCount:      1,
		PlanReadyCount:       1,
	}
	result := renderPlanHeaderDetail(data, 60)
	assert.Contains(t, result, "instances")
	assert.Contains(t, result, "1 running")
	assert.Contains(t, result, "1 paused")
	assert.Contains(t, result, "1 ready")
}

func TestRenderPlanHeaderDetail_ReviewSection(t *testing.T) {
	data := InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             "done-plan",
		ReviewOutcome:        "approved",
		ReviewCycle:          2,
		MaxReviewFixCycles:   3,
	}
	result := renderPlanHeaderDetail(data, 60)
	assert.Contains(t, result, "review")
	assert.Contains(t, result, "approved")
	assert.Contains(t, result, "2 / 3")
}

func TestRenderInstanceDetail_WaveAndTaskNumbers(t *testing.T) {
	data := InfoData{
		HasInstance: true,
		Title:       "coder-w2-t3",
		WaveNumber:  2,
		TotalWaves:  4,
		TaskNumber:  3,
		TotalTasks:  8,
		TaskTitle:   "Add logging",
	}
	result := renderInstanceDetail(data, 60)
	assert.Contains(t, result, "wave")
	assert.Contains(t, result, "2/4")
	assert.Contains(t, result, "task")
	assert.Contains(t, result, "3 of 8: Add logging")
}

// ---------- NavigationPanel detail integration ----------

func TestNavigationPanel_SetDetailData_PlanHeader(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)
	data := NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "my-plan",
			PlanStatus:           "implementing",
		},
	}
	// SetDetailData should not panic and should store the data.
	n.SetDetailData(data)
	assert.Equal(t, data, n.detailData)
}

func TestNavigationPanel_SetDetailData_EmptyData(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)
	n.SetDetailData(NavDetailData{})
	assert.Equal(t, NavDetailData{}, n.detailData)
}

func TestNavigationPanel_ToggleSelectedExpand_ShowsDetail_PlanHeader(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p"}}
	instances := []*session.Instance{makeInst("inst", "p", session.Running)}
	statuses := map[string]TopicStatus{"p": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Select plan header and toggle — should set detailVisible.
	n.selectedIdx = 0
	assert.Equal(t, navRowPlanHeader, n.rows[0].Kind)
	assert.False(t, n.detailVisible)

	ok := n.ToggleSelectedExpand()
	assert.True(t, ok)
	assert.True(t, n.detailVisible, "detail should be visible after toggle on plan header")

	// Toggle again to hide.
	ok = n.ToggleSelectedExpand()
	assert.True(t, ok)
	assert.False(t, n.detailVisible, "detail should be hidden after second toggle")
}

func TestNavigationPanel_ToggleSelectedExpand_ShowsDetail_Instance(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p"}}
	instances := []*session.Instance{makeInst("inst", "p", session.Running)}
	statuses := map[string]TopicStatus{"p": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Select the instance row (row 1) and toggle.
	n.selectedIdx = 1
	assert.Equal(t, navRowInstance, n.rows[1].Kind)
	assert.False(t, n.detailVisible)

	// ToggleSelectedExpand on instance returns false (no tree node) but still
	// toggles the detail section.
	ok := n.ToggleSelectedExpand()
	assert.False(t, ok, "instance rows return false (no tree node to toggle)")
	assert.True(t, n.detailVisible, "detail should be visible after toggle on instance")
}

func TestNavigationPanel_String_DetailSectionVisible(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 50)

	plans := []PlanDisplay{{Filename: "my-plan"}}
	n.SetData(plans, nil, nil, nil, nil)

	// Set detail data for the plan header.
	n.SetDetailData(NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "my-plan",
			PlanStatus:           "implementing",
		},
	})
	n.detailVisible = true

	output := n.String()
	// The detail section should be present.
	assert.Contains(t, output, "plan", "detail section should show plan section header")
}

func TestNavigationPanel_String_DetailSectionHidden(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)

	plans := []PlanDisplay{{Filename: "my-plan"}}
	n.SetData(plans, nil, nil, nil, nil)

	n.SetDetailData(NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "my-plan",
		},
	})
	// Leave detailVisible = false (default).

	output := n.String()
	// Detail divider separator should not be present.
	// (The legend contains "plans" but not the detail divider.)
	// Just confirm it renders without panic and contains the plan.
	assert.Contains(t, output, "my-plan")
}

func TestNavigationPanel_ScrollDetailUp_ScrollDetailDown(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 50)

	// Build enough content to require scrolling.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line content here that is long enough to fill rows\n")
	}
	n.SetDetailData(NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "scroll-test",
			PlanGoal:             sb.String(),
		},
	})
	n.detailVisible = true

	// Scroll down then up — should not panic.
	n.ScrollDetailDown()
	n.ScrollDetailDown()
	n.ScrollDetailUp()
}

func TestNavigationPanel_SetSize_RefreshesDetailViewport(t *testing.T) {
	n := newTestPanel()
	n.SetDetailData(NavDetailData{
		InfoData: InfoData{
			IsPlanHeaderSelected: true,
			PlanName:             "resize-test",
		},
	})
	n.detailVisible = true

	// Changing size should not panic.
	require.NotPanics(t, func() {
		n.SetSize(80, 40)
		n.SetSize(40, 20)
	})
}

func TestNavigationPanel_DetailSectionLines_ZeroWhenHidden(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)
	assert.Equal(t, 0, n.detailSectionLines())
}

func TestNavigationPanel_DetailSectionLines_NonZeroWhenVisible(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)
	n.detailVisible = true
	lines := n.detailSectionLines()
	assert.Greater(t, lines, 0)
}

func TestNavigationPanel_AvailRows_AccountsForDetail(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)

	withoutDetail := n.availRows()
	n.detailVisible = true
	withDetail := n.availRows()

	assert.Less(t, withDetail, withoutDetail, "detail visible should reduce available nav rows")
}
