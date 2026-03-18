package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectPlanBrowserMsgs(cmd tea.Cmd) []planBrowserOpenedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	var results []planBrowserOpenedMsg
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			results = append(results, collectPlanBrowserMsgs(sub)...)
		}
	} else if opened, ok := msg.(planBrowserOpenedMsg); ok {
		results = append(results, opened)
	}
	return results
}

func TestHandleKeyPress_BrowserOpensSelectedPlan(t *testing.T) {
	h := newTestHome()
	h.taskStoreProject = "proj"
	h.nav.SetPlans([]ui.PlanDisplay{{Filename: "plan-browser", Status: "ready"}})
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+"plan-browser"))

	called := false
	h.planBrowserOpener = func(repoRoot, project, planFile string) (string, bool, error) {
		called = true
		assert.Equal(t, h.activeRepoPath, repoRoot)
		assert.Equal(t, "proj", project)
		assert.Equal(t, "plan-browser", planFile)
		return "http://127.0.0.1:7433/admin/tasks/plan-browser?project=proj", false, nil
	}

	h.keySent = true
	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'b', Text: "b"})
	updated := model.(*home)
	require.NotNil(t, cmd)
	msgs := collectPlanBrowserMsgs(cmd)
	require.Len(t, msgs, 1)
	opened := msgs[0]
	assert.True(t, called)
	assert.False(t, opened.startedServer)
	assert.Contains(t, opened.url, "/admin/tasks/plan-browser")
	assert.True(t, updated.toastManager.HasActiveToasts())
}

func TestHandleKeyPress_BrowserUsesInstanceTaskWhenNoPlanSelected(t *testing.T) {
	h := newTestHome()
	h.taskStoreProject = "proj"
	inst, err := session.NewInstance(session.InstanceOptions{Title: "coder", Program: "opencode"})
	require.NoError(t, err)
	h.nav.AddInstance(inst)()
	inst.TaskFile = "plan-browser"
	h.nav.SelectInstance(inst)

	called := false
	h.planBrowserOpener = func(repoRoot, project, planFile string) (string, bool, error) {
		called = true
		assert.Equal(t, "plan-browser", planFile)
		return "http://127.0.0.1:7433/admin/tasks/plan-browser?project=proj", true, nil
	}

	h.keySent = true
	_, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: 'b', Text: "b"})
	require.NotNil(t, cmd)
	msgs := collectPlanBrowserMsgs(cmd)
	require.Len(t, msgs, 1)
	opened := msgs[0]
	assert.True(t, called)
	assert.True(t, opened.startedServer)
}

func TestOpenTaskContextMenu_IncludesBrowserAction(t *testing.T) {
	h := newTestHome()
	h.nav.SetPlans([]ui.PlanDisplay{{Filename: "plan-browser", Status: "ready"}})
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+"plan-browser"))

	model, _ := h.openTaskContextMenu()
	updated := model.(*home)
	menu, ok := updated.overlays.Current().(*overlay.ContextMenu)
	require.True(t, ok)
	assert.Contains(t, menu.View(), "open in browser")
}
