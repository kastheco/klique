package app

import (
	"fmt"
	"os/exec"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/ui/overlay"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// executeContextAction performs the action selected from a context menu.
func (m *home) executeContextAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "kill_instance":
		selected := m.nav.GetSelectedInstance()
		if selected != nil {
			title := selected.Title
			m.audit(auditlog.EventAgentKilled, "agent killed",
				auditlog.WithInstance(title),
				auditlog.WithAgent(selected.AgentType),
				auditlog.WithPlan(selected.TaskFile),
			)
			m.removeFromAllInstances(title)
			m.nav.Kill()
			m.saveAllInstances()
			m.updateNavPanelStatus()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "open_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		return m, func() tea.Msg {
			ch, err := m.nav.Attach()
			if err != nil {
				return err
			}
			<-ch
			return instanceChangedMsg{}
		}

	case "pause_instance":
		selected := m.nav.GetSelectedInstance()
		if selected != nil && selected.Status != session.Paused {
			if err := selected.Pause(); err != nil {
				return m, m.handleError(err)
			}
			m.audit(auditlog.EventAgentPaused, "agent paused",
				auditlog.WithInstance(selected.Title),
				auditlog.WithAgent(selected.AgentType),
				auditlog.WithPlan(selected.TaskFile),
			)
			m.saveAllInstances()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "resume_instance":
		selected := m.nav.GetSelectedInstance()
		if selected != nil && selected.Status == session.Paused {
			if err := selected.Resume(); err != nil {
				return m, m.handleError(err)
			}
			m.audit(auditlog.EventAgentResumed, "agent resumed",
				auditlog.WithInstance(selected.Title),
				auditlog.WithAgent(selected.AgentType),
				auditlog.WithPlan(selected.TaskFile),
			)
			m.saveAllInstances()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "push_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		return m.pushSelectedInstance()

	case "create_pr_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = statePRTitle
		tio := overlay.NewTextInputOverlay("pr title", selected.Title)
		tio.SetSize(60, 3)
		m.overlays.Show(tio)
		return m, nil

	case "send_prompt_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		return m, m.enterFocusMode()

	case "copy_worktree_path":
		selected := m.nav.GetSelectedInstance()
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
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		_ = clipboard.WriteAll(selected.Branch)
		return m, nil

	case "rename_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = stateRenameInstance
		tio := overlay.NewTextInputOverlay("rename instance", selected.Title)
		tio.SetSize(60, 3)
		m.overlays.Show(tio)
		return m, nil

	case "mark_task_complete":
		selected := m.nav.GetSelectedInstance()
		if selected == nil || selected.TaskNumber == 0 {
			return m, nil
		}
		orch, ok := m.waveOrchestrators[selected.TaskFile]
		if !ok {
			return m, nil
		}
		orch.MarkTaskComplete(selected.TaskNumber)
		selected.SetStatus(session.Ready)
		m.toastManager.Success(fmt.Sprintf("task %d marked complete", selected.TaskNumber))
		return m, tea.Batch(m.instanceChanged(), m.toastTickCmd())

	case "change_topic":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		m.pendingChangeTopicTask = planFile
		topicNames := m.getTopicNames()
		topicNames = append([]string{"(No topic)"}, topicNames...)
		po := overlay.NewPickerOverlay("Move to topic", topicNames)
		po.SetAllowCustom(true)
		m.overlays.Show(po)
		m.state = stateChangeTopic
		return m, nil

	case "set_status":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		m.pendingSetStatusTask = planFile
		statuses := []string{"ready", "planning", "implementing", "reviewing", "done", "cancelled"}
		m.overlays.Show(overlay.NewPickerOverlay("set status", statuses))
		m.state = stateSetStatus
		return m, nil

	case "start_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerTaskStage(planFile, "plan")

	case "start_implement":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerTaskStage(planFile, "implement")

	case "start_solo":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerTaskStage(planFile, "solo")

	case "start_review":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerTaskStage(planFile, "review")

	case "inspect_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile != "" {
			m.nav.InspectPlan(planFile)
		}
		return m, tea.WindowSize()

	case "view_plan":
		return m.viewSelectedPlan()

	case "rename_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		currentName := taskstate.DisplayName(planFile)
		m.state = stateRenameTask
		tio := overlay.NewTextInputOverlay("rename task", currentName)
		tio.SetSize(60, 3)
		m.overlays.Show(tio)
		return m, nil

	case "chat_about_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		m.pendingChatAboutTask = planFile
		m.state = stateChatAboutTask
		tio := overlay.NewTextInputOverlay("ask about this task", "")
		tio.SetSize(60, 5)
		tio.SetMultiline(true)
		tio.SetPlaceholder("what would you like to know?")
		m.overlays.Show(tio)
		return m, nil

	case "push_plan_branch":
		planInst := m.findTaskInstance()
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		capturedPlanTitle := planInst.Title
		capturedPlanBranch := planInst.Branch
		pushAction := func() tea.Msg {
			worktree, err := planInst.GetGitWorktree()
			if err != nil {
				return err
			}
			if err := worktree.PushChanges("update from kas", true); err != nil {
				return err
			}
			m.audit(auditlog.EventGitPush, fmt.Sprintf("pushed plan branch %s", capturedPlanBranch),
				auditlog.WithInstance(capturedPlanTitle),
			)
			return nil
		}
		message := fmt.Sprintf("push changes from plan '%s'?", planInst.Title)
		return m, m.confirmAction(message, pushAction)

	case "create_plan_pr":
		planInst := m.findTaskInstance()
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		// Select the plan's instance so the PR flow can find it via GetSelectedInstance().
		m.nav.SelectInstance(planInst)
		m.state = statePRTitle
		tio := overlay.NewTextInputOverlay("pr title", planInst.Title)
		tio.SetSize(60, 3)
		m.overlays.Show(tio)
		return m, nil

	case "merge_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.taskState == nil {
			return m, nil
		}
		entry, ok := m.taskState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("task not found: %s", planFile))
		}
		if entry.Branch == "" {
			return m, m.handleError(fmt.Errorf("plan has no branch to merge"))
		}
		planName := taskstate.DisplayName(planFile)
		mergeAction := func() tea.Msg {
			// Kill all instances bound to this plan.
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].TaskFile == planFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.MergeTaskBranch(m.activeRepoPath, entry.Branch); err != nil {
				return err
			}
			// Walk through FSM to done if not already there.
			if taskfsm.Status(entry.Status) != taskfsm.StatusDone {
				if taskfsm.Status(entry.Status) != taskfsm.StatusReviewing {
					if err := m.fsmSetReviewing(planFile); err != nil {
						return err
					}
				}
				if err := m.fsm.Transition(planFile, taskfsm.ReviewApproved); err != nil {
					return err
				}
			}
			m.audit(auditlog.EventPlanMerged, "task merged to main: "+planName,
				auditlog.WithPlan(planFile))
			_ = m.saveAllInstances()
			m.loadTaskState()
			m.updateSidebarTasks()
			return taskRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("merge '%s' branch into main?", planName), mergeAction)

	case "mark_plan_done":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.taskState == nil {
			return m, nil
		}
		entry, ok := m.taskState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("task not found: %s", planFile))
		}
		if entry.Status != taskstate.StatusDone {
			// Walk through any missing lifecycle stages before approval so mark-done
			// works from ready/implementing/reviewing states.
			if entry.Status != taskstate.StatusReviewing {
				if err := m.fsmSetReviewing(planFile); err != nil {
					return m, m.handleError(err)
				}
			}
			if err := m.fsm.Transition(planFile, taskfsm.ReviewApproved); err != nil {
				return m, m.handleError(err)
			}
			m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → done (manual)",
				auditlog.WithPlan(planFile))
		}
		m.loadTaskState()
		m.updateSidebarTasks()
		return m, tea.WindowSize()

	case "request_review":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.taskState == nil {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, taskfsm.RequestReview); err != nil {
			return m, m.handleError(err)
		}
		m.loadTaskState()
		m.updateSidebarTasks()
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			return m, cmd
		}
		return m, tea.WindowSize()

	case "resume_implement":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.taskState == nil {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, taskfsm.Reimplement); err != nil {
			return m, m.handleError(err)
		}
		m.loadTaskState()
		m.updateSidebarTasks()
		return m, tea.WindowSize()

	case "cancel_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.taskState == nil {
			return m, nil
		}
		planName := taskstate.DisplayName(planFile)
		cancelAction := func() tea.Msg {
			if err := m.fsm.Transition(planFile, taskfsm.Cancel); err != nil {
				return err
			}
			m.audit(auditlog.EventPlanCancelled, "task cancelled by user: "+planName,
				auditlog.WithPlan(planFile))
			return taskRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("cancel task '%s'?", planName), cancelAction)

	case "modify_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, taskfsm.PlanStart); err != nil {
			return m, m.handleError(err)
		}
		m.loadTaskState()
		m.updateSidebarTasks()
		return m.spawnTaskAgent(planFile, "plan", buildModifyTaskPrompt(planFile))

	case "start_over_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		entry, ok := m.taskState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("task not found: %s", planFile))
		}
		planName := taskstate.DisplayName(planFile)
		startOverAction := func() tea.Msg {
			// Kill all instances bound to this plan
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].TaskFile == planFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.ResetTaskBranch(m.activeRepoPath, entry.Branch); err != nil {
				return err
			}
			if err := m.fsmForceToPlanning(planFile); err != nil {
				return err
			}
			m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → planning (start over)",
				auditlog.WithPlan(planFile),
				auditlog.WithDetail("start over: branch reset"))
			_ = m.saveAllInstances()
			m.loadTaskState()
			m.updateSidebarTasks()
			return taskRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("start over task '%s'? this resets the branch.", planName), startOverAction)

	case "restart_instance":
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		capturedTitle := selected.Title
		capturedAgent := selected.AgentType
		capturedPlan := selected.TaskFile
		return m, func() tea.Msg {
			err := selected.Restart()
			if err != nil {
				return err
			}
			m.audit(auditlog.EventAgentRestarted, "agent restarted",
				auditlog.WithInstance(capturedTitle),
				auditlog.WithAgent(capturedAgent),
				auditlog.WithPlan(capturedPlan),
			)
			_ = m.saveAllInstances()
			return instanceChangedMsg{}
		}

	case "toggle_auto_advance":
		if m.appConfig == nil {
			return m, nil
		}
		m.appConfig.AutoAdvanceWaves = !m.appConfig.AutoAdvanceWaves
		label := "off"
		if m.appConfig.AutoAdvanceWaves {
			label = "on"
		}
		m.toastManager.Success(fmt.Sprintf("auto-advance waves: %s", label))
		// Persist to disk (best-effort)
		_ = config.SaveConfig(m.appConfig)
		return m, m.toastTickCmd()
	}

	return m, nil
}

// fsmSetImplementing transitions the plan to implementing, handling the
// planning→ready→implementing two-step when called after a planner finishes.
func (m *home) fsmSetImplementing(planFile string) error {
	if m.taskState == nil {
		return fmt.Errorf("task state is not loaded")
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return fmt.Errorf("task not found: %s", planFile)
	}
	current := taskfsm.Status(entry.Status)
	if current == taskfsm.StatusImplementing {
		return nil // already implementing (re-spawning coder), no status change
	}
	if current == taskfsm.StatusPlanning {
		// Planner finished without writing a sentinel — transition through ready.
		if err := m.fsm.Transition(planFile, taskfsm.PlannerFinished); err != nil {
			return err
		}
		m.audit(auditlog.EventPlanTransition, "planning → ready",
			auditlog.WithPlan(planFile))
	}
	if err := m.fsm.Transition(planFile, taskfsm.ImplementStart); err != nil {
		return err
	}
	m.audit(auditlog.EventPlanTransition, string(current)+" → implementing",
		auditlog.WithPlan(planFile))
	return nil
}

// fsmSetReviewing walks the FSM to reviewing from any earlier state.
// If already reviewing, it's a no-op (allows re-spawning a reviewer).
func (m *home) fsmSetReviewing(planFile string) error {
	if m.taskState == nil {
		return fmt.Errorf("task state is not loaded")
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return fmt.Errorf("task not found: %s", planFile)
	}
	current := taskfsm.Status(entry.Status)
	if current == taskfsm.StatusReviewing {
		return nil // already reviewing, no-op
	}
	// Walk through intermediate states to reach implementing first.
	if current != taskfsm.StatusImplementing {
		if err := m.fsmSetImplementing(planFile); err != nil {
			return err
		}
	}
	if err := m.fsm.Transition(planFile, taskfsm.ImplementFinished); err != nil {
		return err
	}
	m.audit(auditlog.EventPlanTransition, string(current)+" → reviewing",
		auditlog.WithPlan(planFile))
	return nil
}

// fsmRevertToPlanning moves the plan back to planning state from implementing.
// Used when implementation can't start (e.g., missing wave headers).
func (m *home) fsmRevertToPlanning(planFile string) error {
	if m.taskState == nil {
		return fmt.Errorf("task state is not loaded")
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return fmt.Errorf("task not found: %s", planFile)
	}
	if taskfsm.Status(entry.Status) == taskfsm.StatusPlanning {
		return nil // already there
	}
	if err := m.fsm.Transition(planFile, taskfsm.Cancel); err != nil {
		return err
	}
	return m.fsm.Transition(planFile, taskfsm.Reopen)
}

// fsmForceToPlanning moves the plan to planning from any current state.
// Used for start-over scenarios where branch history is reset.
func (m *home) fsmForceToPlanning(planFile string) error {
	if m.taskState == nil {
		return fmt.Errorf("task state is not loaded")
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return fmt.Errorf("task not found: %s", planFile)
	}
	switch taskfsm.Status(entry.Status) {
	case taskfsm.StatusPlanning:
		return nil
	case taskfsm.StatusCancelled:
		return m.fsm.Transition(planFile, taskfsm.Reopen)
	case taskfsm.StatusDone:
		return m.fsm.Transition(planFile, taskfsm.StartOver)
	default:
		// ready, planning, implementing, reviewing → Cancel then Reopen
		if err := m.fsm.Transition(planFile, taskfsm.Cancel); err != nil {
			return err
		}
		return m.fsm.Transition(planFile, taskfsm.Reopen)
	}
}

// findTaskInstance returns the instance bound to the currently selected plan in the sidebar.
// Returns nil if no plan is selected or no instance is bound to it.
func (m *home) findTaskInstance() *session.Instance {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		return nil
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile {
			return inst
		}
	}
	return nil
}

// openContextMenu builds a context menu for the currently focused/selected item
// (plan or instance) and positions it next to the selected item.
func (m *home) openContextMenu() (tea.Model, tea.Cmd) {
	if m.focusSlot == slotNav {
		// Nav panel focused — instance rows get the instance menu,
		// plan headers get the plan menu, everything else is a no-op.
		if inst := m.nav.GetSelectedInstance(); inst != nil {
			// fall through to instance context menu below
		} else if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openTaskContextMenu()
		} else {
			return m, nil
		}
	}

	// Build instance context menu (reached from nav or other slots)
	selected := m.nav.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	items := []overlay.ContextMenuItem{
		{Label: "open", Action: "open_instance"},
		{Label: "kill", Action: "kill_instance"},
		{Label: "restart", Action: "restart_instance"},
	}
	if selected.Status == session.Paused {
		items = append(items, overlay.ContextMenuItem{Label: "resume", Action: "resume_instance"})
	} else {
		items = append(items, overlay.ContextMenuItem{Label: "pause", Action: "pause_instance"})
	}
	if selected.Started() && selected.Status != session.Paused {
		items = append(items, overlay.ContextMenuItem{Label: "focus agent", Action: "send_prompt_instance"})
	}
	items = append(items, overlay.ContextMenuItem{Label: "rename", Action: "rename_instance"})
	items = append(items, overlay.ContextMenuItem{Label: "push branch", Action: "push_instance"})
	items = append(items, overlay.ContextMenuItem{Label: "create pr", Action: "create_pr_instance"})
	// Wave task: offer manual completion
	if selected.TaskNumber > 0 {
		if orch, ok := m.waveOrchestrators[selected.TaskFile]; ok && orch.IsTaskRunning(selected.TaskNumber) {
			items = append(items, overlay.ContextMenuItem{Label: "mark complete", Action: "mark_task_complete"})
		}
	}
	// Position at the left edge of the instance list (middle column)
	x := m.navWidth
	y := 1 + 4 + m.nav.GetSelectedIdx()*4 // PaddingTop(1) + header rows + item offset
	m.overlays.ShowPositioned(overlay.NewContextMenu(items), x, y, false)
	m.state = stateContextMenu
	return m, nil
}

func (m *home) openTaskContextMenu() (tea.Model, tea.Cmd) {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}

	var items []overlay.ContextMenuItem
	if m.taskState != nil {
		if entry, ok := m.taskState.Plans[planFile]; ok {
			// Offer every forward lifecycle stage from the current state so the
			// user can manually advance through plan → implement → review → done.
			switch entry.Status {
			case taskstate.StatusReady, taskstate.StatusPlanning:
				items = append(items,
					overlay.ContextMenuItem{Label: "start planning", Action: "start_plan"},
					overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
					overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				)
			case taskstate.StatusImplementing:
				items = append(items,
					overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
					overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				)
			case taskstate.StatusReviewing:
				items = append(items,
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
					overlay.ContextMenuItem{Label: "mark finished", Action: "mark_plan_done"},
				)
			case taskstate.StatusDone:
				items = append(items,
					overlay.ContextMenuItem{Label: "request review", Action: "request_review"},
					overlay.ContextMenuItem{Label: "resume implement", Action: "resume_implement"},
				)
			}
		}
	}
	// History plans get an "inspect task" option to move them to the dead section.
	if m.nav.IsSelectedHistoryPlan() {
		items = append(items,
			overlay.ContextMenuItem{Label: "inspect task", Action: "inspect_plan"},
		)
	}
	autoAdvanceLabel := "auto-advance waves: off"
	if m.appConfig != nil && m.appConfig.AutoAdvanceWaves {
		autoAdvanceLabel = "auto-advance waves: on"
	}
	items = append(items,
		overlay.ContextMenuItem{Label: "chat about this", Action: "chat_about_plan"},
		overlay.ContextMenuItem{Label: "view task", Action: "view_plan"},
		overlay.ContextMenuItem{Label: "rename task", Action: "rename_plan"},
		overlay.ContextMenuItem{Label: "set topic", Action: "change_topic"},
		overlay.ContextMenuItem{Label: autoAdvanceLabel, Action: "toggle_auto_advance"},
		overlay.ContextMenuItem{Label: "set status", Action: "set_status"},
		overlay.ContextMenuItem{Label: "merge to main", Action: "merge_plan"},
		overlay.ContextMenuItem{Label: "mark done", Action: "mark_plan_done"},
		overlay.ContextMenuItem{Label: "start over", Action: "start_over_plan"},
		overlay.ContextMenuItem{Label: "cancel task", Action: "cancel_plan"},
	)

	x := m.navWidth
	y := 1 + 4 + m.nav.GetSelectedIdx()
	m.overlays.ShowPositioned(overlay.NewContextMenu(items), x, y, false)
	m.state = stateContextMenu
	return m, nil
}

// pushSelectedInstance pushes the selected instance's branch changes.
func (m *home) pushSelectedInstance() (tea.Model, tea.Cmd) {
	selected := m.nav.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	capturedTitle := selected.Title
	capturedBranch := selected.Branch
	pushAction := func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}
		commitMsg := "update from kas"
		if err := worktree.PushChanges(commitMsg, true); err != nil {
			return err
		}
		m.audit(auditlog.EventGitPush, fmt.Sprintf("pushed branch %s", capturedBranch),
			auditlog.WithInstance(capturedTitle),
		)
		return nil
	}
	message := "push changes from '" + selected.Title + "'?"
	return m, m.confirmAction(message, pushAction)
}

// triggerTaskStage handles a user action on a plan lifecycle stage row.
// It checks if the stage is locked, applies the concurrency gate for the
// implement stage, and then executes the stage transition.
func (m *home) triggerTaskStage(planFile, stage string) (tea.Model, tea.Cmd) {
	if m.taskState == nil {
		return m, m.handleError(fmt.Errorf("no task state loaded"))
	}
	entry, ok := m.taskState.Plans[planFile]
	if !ok {
		return m, m.handleError(fmt.Errorf("missing task state for %s", planFile))
	}

	// Backfill branch name for plans created before the branch field existed.
	if entry.Branch == "" {
		entry.Branch = gitpkg.TaskBranchFromFile(planFile)
		if err := m.taskState.SetBranch(planFile, entry.Branch); err != nil {
			return m, m.handleError(fmt.Errorf("failed to assign branch for plan: %w", err))
		}
	}

	// Check if stage is locked
	if isLocked(entry.Status, stage) {
		prev := map[string]string{
			"implement": "plan",
			"review":    "implement",
			"finished":  "review",
		}[stage]
		m.toastManager.Error(fmt.Sprintf("complete '%s' first", prev))
		return m, m.toastTickCmd()
	}

	// Concurrency gate for coder stages
	if (stage == "implement" || stage == "solo") && entry.Topic != "" {
		if hasConflict, conflictPlan := m.taskState.HasRunningCoderInTopic(entry.Topic, planFile); hasConflict {
			conflictName := taskstate.DisplayName(conflictPlan)
			message := fmt.Sprintf("⚠ %s is already running in topic \"%s\"\n\nrunning both plans may cause issues.\ncontinue anyway?", conflictName, entry.Topic)
			proceedAction := func() tea.Msg {
				return taskStageConfirmedMsg{planFile: planFile, stage: stage}
			}
			return m, m.confirmAction(message, proceedAction)
		}
	}

	return m.executeTaskStage(planFile, stage)
}

// executeTaskStage runs the actual stage logic (agent spawn, wave orchestration)
// after all gates (lock check, concurrency) have passed. Called directly from
// triggerTaskStage on the normal path, and via taskStageConfirmedMsg when the
// user confirms past the topic-concurrency gate.
func (m *home) executeTaskStage(planFile, stage string) (tea.Model, tea.Cmd) {
	if m.taskState == nil {
		return m, m.handleError(fmt.Errorf("no task state loaded"))
	}
	entry, ok := m.taskState.Plans[planFile]
	if !ok {
		return m, m.handleError(fmt.Errorf("missing task state for %s", planFile))
	}

	// Backfill branch name for plans created before the branch field existed.
	if entry.Branch == "" {
		entry.Branch = gitpkg.TaskBranchFromFile(planFile)
		if err := m.taskState.SetBranch(planFile, entry.Branch); err != nil {
			return m, m.handleError(fmt.Errorf("failed to assign branch for plan: %w", err))
		}
	}

	switch stage {
	case "plan":
		if err := m.fsm.Transition(planFile, taskfsm.PlanStart); err != nil {
			return m, m.handleError(err)
		}
		m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → planning",
			auditlog.WithPlan(planFile))
		m.loadTaskState()
		m.updateSidebarTasks()
		return m.spawnTaskAgent(planFile, "plan", buildPlanningPrompt(taskstate.DisplayName(planFile), entry.Description))
	case "solo":
		// Check store content before fsmSetImplementing — the FSM transition calls
		// store.Update which overwrites the content field with an empty string.
		// Reading before the transition preserves any ingested plan content.
		planName := taskstate.DisplayName(planFile)
		refFile := ""
		if m.taskStore != nil {
			if c, err := m.taskStore.GetContent(m.taskStoreProject, planFile); err == nil && c != "" {
				refFile = planFile
			}
		}
		if err := m.fsmSetImplementing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → implementing (solo)",
			auditlog.WithPlan(planFile))
		m.loadTaskState()
		m.updateSidebarTasks()
		prompt := buildSoloPrompt(planName, entry.Description, refFile)
		return m.spawnTaskAgent(planFile, "solo", prompt)
	case "implement":
		// Read and parse plan from the task store — this also validates wave headers.
		rawContent := ""
		if m.taskStore != nil {
			c, err := m.taskStore.GetContent(m.taskStoreProject, planFile)
			if err != nil {
				return m, m.handleError(err)
			}
			rawContent = c
		}
		plan, err := taskparser.Parse(rawContent)
		if err != nil {
			// No wave headers — revert to planning and respawn the planner with a
			// wave-annotation prompt so the agent adds the required ## Wave sections.
			if setErr := m.fsmRevertToPlanning(planFile); setErr != nil {
				return m, m.handleError(setErr)
			}
			m.loadTaskState()
			m.updateSidebarTasks()
			m.toastManager.Info("task needs ## Wave headers — respawning planner to annotate.")
			_, spawnCmd := m.spawnTaskAgent(planFile, "plan", buildWaveAnnotationPrompt(planFile))
			return m, tea.Batch(m.toastTickCmd(), func() tea.Msg { return taskRefreshMsg{} }, spawnCmd)
		}

		orch := NewWaveOrchestrator(planFile, plan)
		m.waveOrchestrators[planFile] = orch

		if err := m.fsmSetImplementing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → implementing",
			auditlog.WithPlan(planFile))
		m.loadTaskState()
		m.updateSidebarTasks()
		return m.startNextWave(orch, entry)
	case "review":
		if err := m.fsmSetReviewing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → reviewing",
			auditlog.WithPlan(planFile))
		m.loadTaskState()
		m.updateSidebarTasks()
		planName := taskstate.DisplayName(planFile)
		reviewPrompt := scaffold.LoadReviewPrompt("docs/plans/"+planFile, planName)
		return m.spawnTaskAgent(planFile, "review", reviewPrompt)
	}

	// Non-agent stages (finished): mark plan done via FSM.
	if err := m.fsm.Transition(planFile, taskfsm.ReviewApproved); err != nil {
		return m, m.handleError(err)
	}
	m.audit(auditlog.EventPlanTransition, string(entry.Status)+" → done",
		auditlog.WithPlan(planFile))
	m.loadTaskState()
	m.updateSidebarTasks()
	return m, nil
}

// validatePlanContent checks if plan content has ## Wave headers.
// Returns an error if the plan lacks wave annotations or content is empty.
func validatePlanContent(content string) error {
	_, err := taskparser.Parse(content)
	return err
}

// handleTmuxBrowserAction dispatches actions from the tmux session browser overlay.
// browser is the TmuxBrowserOverlay captured BEFORE HandleKey was called (so SelectedItem is valid).
// action is the Result.Action string returned by HandleKey.
func (m *home) handleTmuxBrowserAction(browser *overlay.TmuxBrowserOverlay, action string) (tea.Model, tea.Cmd) {
	switch action {
	case "": // dismissed without action
		m.overlays.Dismiss()
		m.state = stateDefault
		return m, nil

	case "kill":
		if browser == nil {
			return m, nil
		}
		item := browser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		name := item.Name
		browser.RemoveSelected()
		if browser.IsEmpty() {
			m.overlays.Dismiss()
			m.state = stateDefault
		}
		return m, func() tea.Msg {
			killCmd := exec.Command("tmux", "kill-session", "-t", name)
			err := killCmd.Run()
			return tmuxKillResultMsg{name: name, err: err}
		}

	case "adopt":
		if browser == nil {
			return m, nil
		}
		item := browser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		m.state = stateDefault
		return m.adoptOrphanSession(overlay.TmuxBrowserItem{
			Name:  item.Name,
			Title: item.Title,
		})

	case "attach":
		if browser == nil {
			return m, nil
		}
		item := browser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		m.state = stateDefault
		name := item.Name
		return m, tea.ExecProcess(exec.Command("tmux", "attach-session", "-t", name), func(err error) tea.Msg {
			return tmuxAttachReturnMsg{}
		})

	default:
		return m, nil
	}
}

// isLocked returns true if the given stage cannot be triggered given the current plan status.
// The context menu already gates which forward stages are offered, so this only
// guards against truly nonsensical transitions (e.g. marking "finished" when already done).
func isLocked(status taskstate.Status, stage string) bool {
	switch stage {
	case "plan", "implement", "solo", "review":
		// Forward progression is always allowed — the FSM helpers
		// (fsmSetImplementing, fsmSetReviewing) walk through intermediate states.
		return false
	case "finished":
		return status != taskstate.StatusReviewing
	default:
		return true
	}
}
