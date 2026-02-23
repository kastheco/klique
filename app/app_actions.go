package app

import (
	"fmt"

	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/internal/initcmd/scaffold"
	"github.com/kastheco/klique/session"
	gitpkg "github.com/kastheco/klique/session/git"
	"github.com/kastheco/klique/ui/overlay"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// executeContextAction performs the action selected from a context menu.
func (m *home) executeContextAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "kill_instance":
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			title := selected.Title
			m.removeFromAllInstances(title)
			m.list.Kill()
			m.saveAllInstances()
			m.updateSidebarItems()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "open_instance":
		selected := m.list.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		return m, func() tea.Msg {
			ch, err := m.list.Attach()
			if err != nil {
				return err
			}
			<-ch
			return instanceChangedMsg{}
		}

	case "pause_instance":
		selected := m.list.GetSelectedInstance()
		if selected != nil && selected.Status != session.Paused {
			if err := selected.Pause(); err != nil {
				return m, m.handleError(err)
			}
			m.saveAllInstances()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "resume_instance":
		selected := m.list.GetSelectedInstance()
		if selected != nil && selected.Status == session.Paused {
			if err := selected.Resume(); err != nil {
				return m, m.handleError(err)
			}
			m.saveAllInstances()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "push_instance":
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		return m.pushSelectedInstance()

	case "create_pr_instance":
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("PR title", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil

	case "send_prompt_instance":
		selected := m.list.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		return m, m.enterFocusMode()

	case "copy_worktree_path":
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m, m.handleError(err)
		}
		_ = clipboard.WriteAll(worktree.GetWorktreePath())
		return m, nil

	case "copy_branch_name":
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		_ = clipboard.WriteAll(selected.Branch)
		return m, nil

	case "rename_instance":
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = stateRenameInstance
		m.textInputOverlay = overlay.NewTextInputOverlay("Rename instance", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil

	case "rename_topic_new":
		topicName := m.sidebar.GetSelectedTopicName()
		if topicName == "" {
			return m, nil
		}
		m.state = stateRenameInstance // reuse rename overlay state
		m.textInputOverlay = overlay.NewTextInputOverlay("Rename topic", topicName)
		m.textInputOverlay.SetSize(50, 3)
		return m, nil

	case "delete_topic_new":
		topicName := m.sidebar.GetSelectedTopicName()
		if topicName == "" || m.planState == nil {
			return m, nil
		}
		// Ungroup all plans in this topic
		for filename, entry := range m.planState.Plans {
			if entry.Topic == topicName {
				entry.Topic = ""
				m.planState.Plans[filename] = entry
			}
		}
		delete(m.planState.TopicEntries, topicName)
		if err := m.planState.Save(); err != nil {
			return m, m.handleError(err)
		}
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, tea.WindowSize()

	case "start_plan":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "plan")

	case "view_plan":
		return m.viewSelectedPlan()

	case "push_plan_branch":
		planInst := m.findPlanInstance()
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		pushAction := func() tea.Msg {
			worktree, err := planInst.GetGitWorktree()
			if err != nil {
				return err
			}
			if err := worktree.PushChanges("update from klique", true); err != nil {
				return err
			}
			return nil
		}
		message := fmt.Sprintf("Push changes from plan '%s'?", planInst.Title)
		return m, m.confirmAction(message, pushAction)

	case "create_plan_pr":
		planInst := m.findPlanInstance()
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		// Select the plan's instance so the PR flow can find it via GetSelectedInstance().
		m.list.SelectInstance(planInst)
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("PR title", planInst.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil

	case "mark_plan_done":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		if err := m.planState.SetStatus(planFile, planstate.StatusCompleted); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, tea.WindowSize()

	case "cancel_plan":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		planName := planstate.DisplayName(planFile)
		cancelAction := func() tea.Msg {
			if err := m.planState.SetStatus(planFile, planstate.StatusCancelled); err != nil {
				return err
			}
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateSidebarItems()
			return nil
		}
		return m, m.confirmAction(fmt.Sprintf("Cancel plan '%s'?", planName), cancelAction)

	case "modify_plan":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		if err := m.setPlanStatus(planFile, planstate.StatusPlanning); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m.spawnPlanAgent(planFile, "plan", buildModifyPlanPrompt(planFile))

	case "start_over_plan":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		entry, ok := m.planState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
		}
		planName := planstate.DisplayName(planFile)
		startOverAction := func() tea.Msg {
			// Kill all instances bound to this plan
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].PlanFile == planFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.ResetPlanBranch(m.activeRepoPath, entry.Branch); err != nil {
				return err
			}
			if err := m.setPlanStatus(planFile, planstate.StatusPlanning); err != nil {
				return err
			}
			_ = m.saveAllInstances()
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateSidebarItems()
			return planRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("Start over plan '%s'? This resets the branch.", planName), startOverAction)
	}

	return m, nil
}

// setPlanStatus updates the status of a plan in plan-state.json.
func (m *home) setPlanStatus(planFile string, status planstate.Status) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	return m.planState.SetStatus(planFile, status)
}

// findPlanInstance returns the instance bound to the currently selected plan in the sidebar.
// Returns nil if no plan is selected or no instance is bound to it.
func (m *home) findPlanInstance() *session.Instance {
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" {
		return nil
	}
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == planFile {
			return inst
		}
	}
	return nil
}

// openContextMenu builds a context menu for the currently focused/selected item
// (sidebar topic/plan or instance) and positions it next to the selected item.
func (m *home) openContextMenu() (tea.Model, tea.Cmd) {
	if m.focusSlot == slotSidebar {
		// Sidebar focused — use plan or topic context menu
		if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
			return m.openPlanContextMenu()
		}
		if m.sidebar.IsSelectedTopicHeader() {
			return m.openTopicContextMenu()
		}
		return m, nil
	}

	// Instance list focused — build instance context menu
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	items := []overlay.ContextMenuItem{
		{Label: "Open", Action: "open_instance"},
		{Label: "Kill", Action: "kill_instance"},
	}
	if selected.Status == session.Paused {
		items = append(items, overlay.ContextMenuItem{Label: "Resume", Action: "resume_instance"})
	} else {
		items = append(items, overlay.ContextMenuItem{Label: "Pause", Action: "pause_instance"})
	}
	if selected.Started() && selected.Status != session.Paused {
		items = append(items, overlay.ContextMenuItem{Label: "Focus agent", Action: "send_prompt_instance"})
	}
	items = append(items, overlay.ContextMenuItem{Label: "Rename", Action: "rename_instance"})
	items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_instance"})
	items = append(items, overlay.ContextMenuItem{Label: "Create PR", Action: "create_pr_instance"})
	items = append(items, overlay.ContextMenuItem{Label: "Copy worktree path", Action: "copy_worktree_path"})
	items = append(items, overlay.ContextMenuItem{Label: "Copy branch name", Action: "copy_branch_name"})
	// Position at the left edge of the instance list (right column)
	x := m.sidebarWidth + m.tabsWidth
	y := 1 + 4 + m.list.GetSelectedIdx()*4 // PaddingTop(1) + header rows + item offset
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}

func (m *home) openPlanContextMenu() (tea.Model, tea.Cmd) {
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}
	items := []overlay.ContextMenuItem{
		{Label: "Start plan", Action: "start_plan"},
		{Label: "View plan", Action: "view_plan"},
		{Label: "Push branch", Action: "push_plan_branch"},
		{Label: "Create PR", Action: "create_plan_pr"},
		{Label: "Mark done", Action: "mark_plan_done"},
		{Label: "Cancel plan", Action: "cancel_plan"},
	}
	x := m.sidebarWidth
	y := 1 + 4 + m.sidebar.GetSelectedIdx()
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}

// pushSelectedInstance pushes the selected instance's branch changes.
func (m *home) pushSelectedInstance() (tea.Model, tea.Cmd) {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	pushAction := func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}
		commitMsg := "update from klique"
		if err := worktree.PushChanges(commitMsg, true); err != nil {
			return err
		}
		return nil
	}
	message := "Push changes from '" + selected.Title + "'?"
	return m, m.confirmAction(message, pushAction)
}

func (m *home) openTopicContextMenu() (tea.Model, tea.Cmd) {
	topicName := m.sidebar.GetSelectedTopicName()
	if topicName == "" {
		return m, nil
	}
	items := []overlay.ContextMenuItem{
		{Label: "Rename topic", Action: "rename_topic_new"},
		{Label: "Delete topic (ungroup plans)", Action: "delete_topic_new"},
	}
	x := m.sidebarWidth
	y := 1 + 4 + m.sidebar.GetSelectedIdx()
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}

// triggerPlanStage handles a user action on a plan lifecycle stage row.
// It checks if the stage is locked, applies the concurrency gate for the
// implement stage, and then executes the stage transition.
func (m *home) triggerPlanStage(planFile, stage string) (tea.Model, tea.Cmd) {
	if m.planState == nil {
		return m, m.handleError(fmt.Errorf("no plan state loaded"))
	}
	entry, ok := m.planState.Plans[planFile]
	if !ok {
		return m, m.handleError(fmt.Errorf("missing plan state for %s", planFile))
	}

	// Check if stage is locked
	if isLocked(entry.Status, stage) {
		prev := map[string]string{
			"implement": "plan",
			"review":    "implement",
			"finished":  "review",
		}[stage]
		m.toastManager.Error(fmt.Sprintf("complete %s first", prev))
		return m, m.toastTickCmd()
	}

	// Concurrency gate for implement stage
	if stage == "implement" && entry.Topic != "" {
		if hasConflict, conflictPlan := m.planState.HasRunningCoderInTopic(entry.Topic, planFile); hasConflict {
			conflictName := planstate.DisplayName(conflictPlan)
			message := fmt.Sprintf("⚠ %s is already running in topic \"%s\"\n\nRunning both plans may cause issues.\nContinue anyway?", conflictName, entry.Topic)
			proceedAction := func() tea.Msg {
				if err := planStageStatus(planFile, stage, m.planState); err != nil {
					return err
				}
				return planRefreshMsg{}
			}
			return m, m.confirmAction(message, proceedAction)
		}
	}

	// For agent-spawning stages, dispatch to spawnPlanAgent which handles
	// status update + session creation.
	switch stage {
	case "plan":
		if err := m.planState.SetStatus(planFile, planstate.StatusPlanning); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m.spawnPlanAgent(planFile, "plan", buildPlanPrompt(planstate.DisplayName(planFile), entry.Description))
	case "implement":
		if err := m.planState.SetStatus(planFile, planstate.StatusImplementing); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m.spawnPlanAgent(planFile, "implement", buildImplementPrompt(planFile))
	case "review":
		if err := m.planState.SetStatus(planFile, planstate.StatusReviewing); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		planName := planstate.DisplayName(planFile)
		reviewPrompt := scaffold.LoadReviewPrompt("docs/plans/"+planFile, planName)
		return m.spawnPlanAgent(planFile, "review", reviewPrompt)
	}

	// Non-agent stages (finished): just update status.
	if err := planStageStatus(planFile, stage, m.planState); err != nil {
		return m, m.handleError(err)
	}
	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateSidebarItems()
	return m, nil
}

// planStageStatus writes the appropriate status for a plan lifecycle stage to disk.
// Safe to call from a goroutine — only does disk I/O, no model mutations.
func planStageStatus(planFile, stage string, ps *planstate.PlanState) error {
	switch stage {
	case "plan":
		return ps.SetStatus(planFile, planstate.StatusPlanning)
	case "implement":
		return ps.SetStatus(planFile, planstate.StatusImplementing)
	case "review":
		return ps.SetStatus(planFile, planstate.StatusReviewing)
	case "finished":
		return ps.SetStatus(planFile, planstate.StatusCompleted)
	}
	return nil
}

// isLocked returns true if the given stage cannot be triggered given the current plan status.
func isLocked(status planstate.Status, stage string) bool {
	switch stage {
	case "plan":
		return false
	case "implement":
		return status == planstate.StatusReady
	case "review":
		return status == planstate.StatusReady || status == planstate.StatusInProgress
	case "finished":
		return status != planstate.StatusReviewing && status != planstate.StatusDone && status != planstate.StatusCompleted
	default:
		return true
	}
}
