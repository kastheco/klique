package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
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
			m.saveAllInstances()
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())

	case "resume_instance":
		selected := m.nav.GetSelectedInstance()
		if selected != nil && selected.Status == session.Paused {
			if err := selected.Resume(); err != nil {
				return m, m.handleError(err)
			}
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
		m.textInputOverlay = overlay.NewTextInputOverlay("pr title", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
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
		m.textInputOverlay = overlay.NewTextInputOverlay("rename instance", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil

	case "mark_task_complete":
		selected := m.nav.GetSelectedInstance()
		if selected == nil || selected.TaskNumber == 0 {
			return m, nil
		}
		orch, ok := m.waveOrchestrators[selected.PlanFile]
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
		m.pendingChangeTopicPlan = planFile
		topicNames := m.getTopicNames()
		topicNames = append([]string{"(No topic)"}, topicNames...)
		m.pickerOverlay = overlay.NewPickerOverlay("Move to topic", topicNames)
		m.pickerOverlay.SetAllowCustom(true)
		m.state = stateChangeTopic
		return m, nil

	case "start_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "plan")

	case "start_implement":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "implement")

	case "start_solo":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "solo")

	case "start_review":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		return m.triggerPlanStage(planFile, "review")

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
			if err := worktree.PushChanges("update from kas", true); err != nil {
				return err
			}
			return nil
		}
		message := fmt.Sprintf("push changes from plan '%s'?", planInst.Title)
		return m, m.confirmAction(message, pushAction)

	case "create_plan_pr":
		planInst := m.findPlanInstance()
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		// Select the plan's instance so the PR flow can find it via GetSelectedInstance().
		m.nav.SelectInstance(planInst)
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("pr title", planInst.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil

	case "merge_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		entry, ok := m.planState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
		}
		if entry.Branch == "" {
			return m, m.handleError(fmt.Errorf("plan has no branch to merge"))
		}
		planName := planstate.DisplayName(planFile)
		mergeAction := func() tea.Msg {
			// Kill all instances bound to this plan.
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].PlanFile == planFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.MergePlanBranch(m.activeRepoPath, entry.Branch); err != nil {
				return err
			}
			if err := m.fsm.Transition(planFile, planfsm.ReviewApproved); err != nil {
				return err
			}
			_ = m.saveAllInstances()
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateNavPanelStatus()
			return planRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("merge '%s' branch into main?", planName), mergeAction)

	case "mark_plan_done":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		entry, ok := m.planState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
		}
		if entry.Status != planstate.StatusDone {
			// Walk through any missing lifecycle stages before approval so mark-done
			// works from ready/implementing/reviewing states.
			if entry.Status != planstate.StatusReviewing {
				if err := m.fsmSetReviewing(planFile); err != nil {
					return m, m.handleError(err)
				}
			}
			if err := m.fsm.Transition(planFile, planfsm.ReviewApproved); err != nil {
				return m, m.handleError(err)
			}
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		return m, tea.WindowSize()

	case "request_review":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, planfsm.RequestReview); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			return m, cmd
		}
		return m, tea.WindowSize()

	case "resume_implement":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, planfsm.Reimplement); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		return m, tea.WindowSize()

	case "cancel_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		planName := planstate.DisplayName(planFile)
		cancelAction := func() tea.Msg {
			if err := m.fsm.Transition(planFile, planfsm.Cancel); err != nil {
				return err
			}
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateNavPanelStatus()
			return nil
		}
		return m, m.confirmAction(fmt.Sprintf("cancel plan '%s'?", planName), cancelAction)

	case "modify_plan":
		planFile := m.nav.GetSelectedPlanFile()
		if planFile == "" {
			return m, nil
		}
		if err := m.fsm.Transition(planFile, planfsm.PlanStart); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		return m.spawnPlanAgent(planFile, "plan", buildModifyPlanPrompt(planFile))

	case "start_over_plan":
		planFile := m.nav.GetSelectedPlanFile()
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
			if err := m.fsmForceToPlanning(planFile); err != nil {
				return err
			}
			_ = m.saveAllInstances()
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateNavPanelStatus()
			return planRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("start over plan '%s'? this resets the branch.", planName), startOverAction)
	}

	return m, nil
}

// fsmSetImplementing transitions the plan to implementing, handling the
// planning→ready→implementing two-step when called after a planner finishes.
func (m *home) fsmSetImplementing(planFile string) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	current := planfsm.Status(entry.Status)
	if current == planfsm.StatusImplementing {
		return nil // already implementing (re-spawning coder), no status change
	}
	if current == planfsm.StatusPlanning {
		// Planner finished without writing a sentinel — transition through ready.
		if err := m.fsm.Transition(planFile, planfsm.PlannerFinished); err != nil {
			return err
		}
	}
	return m.fsm.Transition(planFile, planfsm.ImplementStart)
}

// fsmSetReviewing walks the FSM to reviewing from any earlier state.
// If already reviewing, it's a no-op (allows re-spawning a reviewer).
func (m *home) fsmSetReviewing(planFile string) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	current := planfsm.Status(entry.Status)
	if current == planfsm.StatusReviewing {
		return nil // already reviewing, no-op
	}
	// Walk through intermediate states to reach implementing first.
	if current != planfsm.StatusImplementing {
		if err := m.fsmSetImplementing(planFile); err != nil {
			return err
		}
	}
	return m.fsm.Transition(planFile, planfsm.ImplementFinished)
}

// fsmRevertToPlanning moves the plan back to planning state from implementing.
// Used when implementation can't start (e.g., missing wave headers).
func (m *home) fsmRevertToPlanning(planFile string) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	if planfsm.Status(entry.Status) == planfsm.StatusPlanning {
		return nil // already there
	}
	if err := m.fsm.Transition(planFile, planfsm.Cancel); err != nil {
		return err
	}
	return m.fsm.Transition(planFile, planfsm.Reopen)
}

// fsmForceToPlanning moves the plan to planning from any current state.
// Used for start-over scenarios where branch history is reset.
func (m *home) fsmForceToPlanning(planFile string) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	switch planfsm.Status(entry.Status) {
	case planfsm.StatusPlanning:
		return nil
	case planfsm.StatusCancelled:
		return m.fsm.Transition(planFile, planfsm.Reopen)
	case planfsm.StatusDone:
		return m.fsm.Transition(planFile, planfsm.StartOver)
	default:
		// ready, planning, implementing, reviewing → Cancel then Reopen
		if err := m.fsm.Transition(planFile, planfsm.Cancel); err != nil {
			return err
		}
		return m.fsm.Transition(planFile, planfsm.Reopen)
	}
}

// findPlanInstance returns the instance bound to the currently selected plan in the sidebar.
// Returns nil if no plan is selected or no instance is bound to it.
func (m *home) findPlanInstance() *session.Instance {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		return nil
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile == planFile {
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
			return m.openPlanContextMenu()
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
		if orch, ok := m.waveOrchestrators[selected.PlanFile]; ok && orch.IsTaskRunning(selected.TaskNumber) {
			items = append(items, overlay.ContextMenuItem{Label: "mark complete", Action: "mark_task_complete"})
		}
	}
	// Position at the left edge of the instance list (middle column)
	x := m.navWidth
	y := 1 + 4 + m.nav.GetSelectedIdx()*4 // PaddingTop(1) + header rows + item offset
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}

func (m *home) openPlanContextMenu() (tea.Model, tea.Cmd) {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}

	var items []overlay.ContextMenuItem
	if m.planState != nil {
		if entry, ok := m.planState.Plans[planFile]; ok {
			// Offer every forward lifecycle stage from the current state so the
			// user can manually advance through plan → implement → review → done.
			switch entry.Status {
			case planstate.StatusReady, planstate.StatusPlanning:
				items = append(items,
					overlay.ContextMenuItem{Label: "start plan", Action: "start_plan"},
					overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
					overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				)
			case planstate.StatusImplementing:
				items = append(items,
					overlay.ContextMenuItem{Label: "start implement", Action: "start_implement"},
					overlay.ContextMenuItem{Label: "start solo agent", Action: "start_solo"},
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				)
			case planstate.StatusReviewing:
				items = append(items,
					overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
					overlay.ContextMenuItem{Label: "mark finished", Action: "mark_plan_done"},
				)
			case planstate.StatusDone:
				items = append(items,
					overlay.ContextMenuItem{Label: "request review", Action: "request_review"},
					overlay.ContextMenuItem{Label: "resume implement", Action: "resume_implement"},
				)
			}
		}
	}
	items = append(items,
		overlay.ContextMenuItem{Label: "view plan", Action: "view_plan"},
		overlay.ContextMenuItem{Label: "set topic", Action: "change_topic"},
		overlay.ContextMenuItem{Label: "merge to main", Action: "merge_plan"},
		overlay.ContextMenuItem{Label: "mark done", Action: "mark_plan_done"},
		overlay.ContextMenuItem{Label: "start over", Action: "start_over_plan"},
		overlay.ContextMenuItem{Label: "cancel plan", Action: "cancel_plan"},
	)

	x := m.navWidth
	y := 1 + 4 + m.nav.GetSelectedIdx()
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}

// pushSelectedInstance pushes the selected instance's branch changes.
func (m *home) pushSelectedInstance() (tea.Model, tea.Cmd) {
	selected := m.nav.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	pushAction := func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}
		commitMsg := "update from kas"
		if err := worktree.PushChanges(commitMsg, true); err != nil {
			return err
		}
		return nil
	}
	message := "push changes from '" + selected.Title + "'?"
	return m, m.confirmAction(message, pushAction)
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

	// Backfill branch name for plans created before the branch field existed.
	if entry.Branch == "" {
		entry.Branch = gitpkg.PlanBranchFromFile(planFile)
		if err := m.planState.SetBranch(planFile, entry.Branch); err != nil {
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
		if hasConflict, conflictPlan := m.planState.HasRunningCoderInTopic(entry.Topic, planFile); hasConflict {
			conflictName := planstate.DisplayName(conflictPlan)
			message := fmt.Sprintf("⚠ %s is already running in topic \"%s\"\n\nrunning both plans may cause issues.\ncontinue anyway?", conflictName, entry.Topic)
			proceedAction := func() tea.Msg {
				return planStageConfirmedMsg{planFile: planFile, stage: stage}
			}
			return m, m.confirmAction(message, proceedAction)
		}
	}

	return m.executePlanStage(planFile, stage)
}

// executePlanStage runs the actual stage logic (agent spawn, wave orchestration)
// after all gates (lock check, concurrency) have passed. Called directly from
// triggerPlanStage on the normal path, and via planStageConfirmedMsg when the
// user confirms past the topic-concurrency gate.
func (m *home) executePlanStage(planFile, stage string) (tea.Model, tea.Cmd) {
	if m.planState == nil {
		return m, m.handleError(fmt.Errorf("no plan state loaded"))
	}
	entry, ok := m.planState.Plans[planFile]
	if !ok {
		return m, m.handleError(fmt.Errorf("missing plan state for %s", planFile))
	}

	// Backfill branch name for plans created before the branch field existed.
	if entry.Branch == "" {
		entry.Branch = gitpkg.PlanBranchFromFile(planFile)
		if err := m.planState.SetBranch(planFile, entry.Branch); err != nil {
			return m, m.handleError(fmt.Errorf("failed to assign branch for plan: %w", err))
		}
	}

	switch stage {
	case "plan":
		if err := m.fsm.Transition(planFile, planfsm.PlanStart); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		return m.spawnPlanAgent(planFile, "plan", buildPlanPrompt(planstate.DisplayName(planFile), entry.Description))
	case "solo":
		if err := m.fsmSetImplementing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		// Check if plan .md file exists on disk to decide prompt content.
		planName := planstate.DisplayName(planFile)
		planPath := filepath.Join(m.activeRepoPath, "docs", "plans", planFile)
		refFile := ""
		if _, err := os.Stat(planPath); err == nil {
			refFile = planFile
		}
		prompt := buildSoloPrompt(planName, entry.Description, refFile)
		return m.spawnPlanAgent(planFile, "solo", prompt)
	case "implement":
		// Read and parse plan — this also validates wave headers.
		plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
		content, err := os.ReadFile(filepath.Join(plansDir, planFile))
		if err != nil {
			return m, m.handleError(err)
		}
		plan, err := planparser.Parse(string(content))
		if err != nil {
			// No wave headers — revert to planning and respawn the planner with a
			// wave-annotation prompt so the agent adds the required ## Wave sections.
			if setErr := m.fsmRevertToPlanning(planFile); setErr != nil {
				return m, m.handleError(setErr)
			}
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateNavPanelStatus()
			m.toastManager.Info("plan needs ## Wave headers — respawning planner to annotate.")
			wavePrompt := fmt.Sprintf(
				"The plan at docs/plans/%s is missing ## Wave N headers required for wave-based implementation. "+
					"Please annotate the plan by grouping tasks under ## Wave 1, ## Wave 2, … sections. "+
					"Keep all existing task content intact; only add the Wave headers.",
				planFile,
			)
			_, spawnCmd := m.spawnPlanAgent(planFile, "plan", wavePrompt)
			return m, tea.Batch(m.toastTickCmd(), func() tea.Msg { return planRefreshMsg{} }, spawnCmd)
		}

		orch := NewWaveOrchestrator(planFile, plan)
		m.waveOrchestrators[planFile] = orch

		if err := m.fsmSetImplementing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		return m.startNextWave(orch, entry)
	case "review":
		if err := m.fsmSetReviewing(planFile); err != nil {
			return m, m.handleError(err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateNavPanelStatus()
		planName := planstate.DisplayName(planFile)
		reviewPrompt := scaffold.LoadReviewPrompt("docs/plans/"+planFile, planName)
		return m.spawnPlanAgent(planFile, "review", reviewPrompt)
	}

	// Non-agent stages (finished): mark plan done via FSM.
	if err := m.fsm.Transition(planFile, planfsm.ReviewApproved); err != nil {
		return m, m.handleError(err)
	}
	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateNavPanelStatus()
	return m, nil
}

// validatePlanHasWaves reads a plan file and checks it has ## Wave headers.
// Returns an error if the plan lacks wave annotations.
func validatePlanHasWaves(plansDir, planFile string) error {
	content, err := os.ReadFile(filepath.Join(plansDir, planFile))
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	_, err = planparser.Parse(string(content))
	return err
}

// handleTmuxBrowserAction dispatches actions from the tmux session browser overlay.
func (m *home) handleTmuxBrowserAction(action overlay.BrowserAction) (tea.Model, tea.Cmd) {
	switch action {
	case overlay.BrowserDismiss:
		m.tmuxBrowser = nil
		m.state = stateDefault
		return m, nil

	case overlay.BrowserKill:
		if m.tmuxBrowser == nil {
			return m, nil
		}
		item := m.tmuxBrowser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		name := item.Name
		m.tmuxBrowser.RemoveSelected()
		if m.tmuxBrowser.IsEmpty() {
			m.tmuxBrowser = nil
			m.state = stateDefault
		}
		return m, func() tea.Msg {
			killCmd := exec.Command("tmux", "kill-session", "-t", name)
			err := killCmd.Run()
			return tmuxKillResultMsg{name: name, err: err}
		}

	case overlay.BrowserAdopt:
		if m.tmuxBrowser == nil {
			return m, nil
		}
		item := m.tmuxBrowser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		m.tmuxBrowser = nil
		m.state = stateDefault
		return m.adoptOrphanSession(overlay.TmuxBrowserItem{
			Name:  item.Name,
			Title: item.Title,
		})

	case overlay.BrowserAttach:
		if m.tmuxBrowser == nil {
			return m, nil
		}
		item := m.tmuxBrowser.SelectedItem()
		if item.Name == "" {
			return m, nil
		}
		m.tmuxBrowser = nil
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
func isLocked(status planstate.Status, stage string) bool {
	switch stage {
	case "plan", "implement", "solo", "review":
		// Forward progression is always allowed — the FSM helpers
		// (fsmSetImplementing, fsmSetReviewing) walk through intermediate states.
		return false
	case "finished":
		return status != planstate.StatusReviewing
	default:
		return true
	}
}
