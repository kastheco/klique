package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	tea "github.com/charmbracelet/bubbletea"
)

func mergeTopicStatus(status ui.TopicStatus, inst *session.Instance, started bool) ui.TopicStatus {
	if started && !inst.Paused() && !inst.PromptDetected {
		status.HasRunning = true
	}
	if inst.Notified {
		status.HasNotification = true
	}
	return status
}

func mergePlanStatus(status ui.TopicStatus, inst *session.Instance, started bool) ui.TopicStatus {
	if inst.PlanFile == "" {
		return status
	}
	if started && !inst.Paused() {
		if inst.IsReviewer {
			status.HasNotification = true
		} else {
			status.HasRunning = true
		}
	}
	if inst.Notified && inst.IsReviewer {
		status.HasNotification = true
	}
	return status
}

// computeStatusBarData builds the StatusBarData from the current app state.
func (m *home) computeStatusBarData() ui.StatusBarData {
	data := ui.StatusBarData{
		RepoName:         filepath.Base(m.activeRepoPath),
		FocusMode:        m.state == stateFocusAgent,
		TmuxSessionCount: m.tmuxSessionCount,
	}

	if m.nav == nil {
		if data.Branch == "" {
			data.Branch = "main"
		}
		return data
	}

	planFile := m.nav.GetSelectedPlanFile()
	selected := m.nav.GetSelectedInstance()

	switch {
	case planFile != "" && m.planState != nil:
		entry, ok := m.planState.Entry(planFile)
		if ok {
			data.Branch = entry.Branch
			data.PlanName = planstate.DisplayName(planFile)
			data.PlanStatus = string(entry.Status)

			if orch, orchOK := m.waveOrchestrators[planFile]; orchOK {
				waveNum := orch.CurrentWaveNumber()
				totalWaves := orch.TotalWaves()
				if waveNum > 0 {
					data.WaveLabel = fmt.Sprintf("wave %d/%d", waveNum, totalWaves)
					tasks := orch.CurrentWaveTasks()
					data.TaskGlyphs = make([]ui.TaskGlyph, len(tasks))
					for i, task := range tasks {
						switch orch.taskStates[task.Number] {
						case taskComplete:
							data.TaskGlyphs[i] = ui.TaskGlyphComplete
						case taskFailed:
							data.TaskGlyphs[i] = ui.TaskGlyphFailed
						case taskRunning:
							data.TaskGlyphs[i] = ui.TaskGlyphRunning
						default:
							data.TaskGlyphs[i] = ui.TaskGlyphPending
						}
					}
				}
			}
		}
	case selected != nil && selected.Branch != "":
		data.Branch = selected.Branch
		if selected.PlanFile != "" && m.planState != nil {
			entry, ok := m.planState.Entry(selected.PlanFile)
			if ok {
				data.PlanName = planstate.DisplayName(selected.PlanFile)
				data.PlanStatus = string(entry.Status)
			}
		}
	}

	if data.Branch == "" {
		data.Branch = "main"
	}

	return data
}

// computePlanStatuses builds per-plan instance status flags (running/notification)
// from the current instance list.
func (m *home) computePlanStatuses() map[string]ui.TopicStatus {
	planStatuses := make(map[string]ui.TopicStatus)
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile == "" {
			continue
		}
		planSt := planStatuses[inst.PlanFile]
		planStatuses[inst.PlanFile] = mergePlanStatus(planSt, inst, inst.Started())
	}
	return planStatuses
}

// updateNavPanelStatus recomputes plan instance statuses and triggers a row
// rebuild. Use this after instance mutations (kill, remove, pause) where the
// plan list itself hasn't changed. When updateSidebarPlans is also called,
// skip this — updateSidebarPlans already includes plan statuses in its rebuild.
func (m *home) updateNavPanelStatus() {
	m.nav.SetItems(nil, nil, 0, nil, nil, m.computePlanStatuses())
}

// focusSlot constants for readability.
// Order matches tab layout: info (first), agent (second), diff (third).
const (
	slotNav   = 0
	slotInfo  = 1
	slotAgent = 2
	slotDiff  = 3
	slotCount = 4
)

// setFocusSlot updates which pane has focus and syncs visual state.
func (m *home) setFocusSlot(slot int) {
	m.focusSlot = slot
	m.nav.SetFocused(slot == slotNav)
	m.menu.SetFocusSlot(slot)

	// Center pane is focused when any of the 3 center tabs is active.
	centerFocused := slot >= slotInfo && slot <= slotDiff
	m.tabbedWindow.SetFocused(centerFocused)

	// When focusing a center tab, switch the visible tab to match and track which tab is focused.
	if centerFocused {
		m.tabbedWindow.SetActiveTab(slot - slotInfo) // slotInfo=1 → InfoTab=0, etc.
	}
	// focusedTab always tracks activeTab so the gradient header is visible
	// regardless of which pane has keyboard focus.
}

// nextFocusSlot cycles the visible center tab forward (info → agent → diff → info).
// The sidebar always retains keyboard focus (focusSlot stays slotNav); only the
// displayed tab changes. This is called by Tab and →.
func (m *home) nextFocusSlot() {
	switch m.tabbedWindow.GetActiveTab() {
	case ui.InfoTab:
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
	case ui.PreviewTab:
		m.tabbedWindow.SetActiveTab(ui.DiffTab)
	default: // ui.DiffTab — wraps to info
		m.tabbedWindow.SetActiveTab(ui.InfoTab)
	}
}

// prevFocusSlot cycles the visible center tab backward (info → diff → agent → info).
// The sidebar always retains keyboard focus (focusSlot stays slotNav).
func (m *home) prevFocusSlot() {
	switch m.tabbedWindow.GetActiveTab() {
	case ui.PreviewTab:
		m.tabbedWindow.SetActiveTab(ui.InfoTab)
	case ui.DiffTab:
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
	default: // ui.InfoTab — wraps to diff
		m.tabbedWindow.SetActiveTab(ui.DiffTab)
	}
}

// enterFocusMode enters focus/insert mode and starts the fast preview ticker.
// enterFocusMode reuses the existing previewTerminal if it is already attached to
// the selected instance — entering focus just toggles key forwarding to the same
// terminal. Only spawns a new terminal if none is attached yet (rare fallback).
func (m *home) enterFocusMode() tea.Cmd {
	m.tabbedWindow.ClearDocumentMode()
	selected := m.nav.GetSelectedInstance()
	if selected == nil || !selected.Started() || selected.Status == session.Paused {
		return nil
	}

	// If previewTerminal is already attached to this instance, just enter focus mode.
	if m.previewTerminal != nil && m.previewTerminalInstance == selected.Title {
		m.state = stateFocusAgent
		m.tabbedWindow.SetFocusMode(true)
		m.menu.SetFocusMode(true)
		return nil
	}

	// No terminal yet (shouldn't normally happen) — spawn one synchronously-ish.
	cols, rows := m.tabbedWindow.GetPreviewSize()
	if cols < 10 {
		cols = 80
	}
	if rows < 5 {
		rows = 24
	}
	term, err := selected.NewEmbeddedTerminalForInstance(cols, rows)
	if err != nil {
		return m.handleError(err)
	}
	m.previewTerminal = term
	m.previewTerminalInstance = selected.Title
	m.state = stateFocusAgent
	m.tabbedWindow.SetFocusMode(true)
	m.menu.SetFocusMode(true)

	return nil
}

// exitFocusMode resets focus state. previewTerminal stays alive — it continues
// rendering in normal preview mode after the user exits focus/insert mode.
func (m *home) exitFocusMode() {
	// previewTerminal stays alive — it continues rendering in normal preview mode.
	m.state = stateDefault
	m.tabbedWindow.SetFocusMode(false)
	m.menu.SetFocusMode(false)
}

// switchToTab changes the visible center tab without stealing focus from the sidebar.
// The sidebar (slotNav) always retains keyboard focus.
func (m *home) switchToTab(name keys.KeyName) (tea.Model, tea.Cmd) {
	var targetTab int
	switch name {
	case keys.KeyTabAgent:
		targetTab = ui.PreviewTab
	case keys.KeyTabDiff:
		targetTab = ui.DiffTab
	case keys.KeyTabInfo:
		targetTab = ui.InfoTab
	default:
		return m, nil
	}

	if m.tabbedWindow.GetActiveTab() == targetTab {
		return m, nil
	}

	m.tabbedWindow.SetActiveTab(targetTab)
	return m, nil
}

// rebuildInstanceList clears the list and repopulates with instances matching activeRepoPath.
func (m *home) rebuildInstanceList() {
	m.nav.Clear()
	for _, inst := range m.allInstances {
		repoPath := inst.GetRepoPath()
		if repoPath == "" || repoPath == m.activeRepoPath {
			m.nav.AddInstance(inst)()
		}
	}
	// Reload plan state for the new active repo.
	m.planStateDir = filepath.Join(m.activeRepoPath, "docs", "plans")
	m.loadPlanState()
	m.updateSidebarPlans()
}

// getKnownRepos returns distinct repo paths from allInstances, recent repos, plus activeRepoPath.
func (m *home) getKnownRepos() []string {
	seen := make(map[string]bool)
	seen[m.activeRepoPath] = true
	for _, inst := range m.allInstances {
		rp := inst.GetRepoPath()
		if rp != "" {
			seen[rp] = true
		}
	}
	// Include recent repos from persisted state
	if state, ok := m.appState.(*config.State); ok {
		for _, rp := range state.GetRecentRepos() {
			seen[rp] = true
		}
	}
	repos := make([]string, 0, len(seen))
	for rp := range seen {
		repos = append(repos, rp)
	}
	sort.Strings(repos)
	return repos
}

// buildRepoPickerItems returns display strings for the repo picker.
func (m *home) buildRepoPickerItems() []string {
	repos := m.getKnownRepos()
	countByRepo := make(map[string]int)
	for _, inst := range m.allInstances {
		rp := inst.GetRepoPath()
		if rp != "" {
			countByRepo[rp]++
		}
	}

	// Detect duplicate basenames to disambiguate
	baseCount := make(map[string]int)
	for _, rp := range repos {
		baseCount[filepath.Base(rp)]++
	}

	m.repoPickerMap = make(map[string]string)
	items := make([]string, 0, len(repos)+1)
	for _, rp := range repos {
		base := filepath.Base(rp)
		name := base
		if baseCount[base] > 1 {
			// Disambiguate with parent directory
			name = filepath.Base(filepath.Dir(rp)) + "/" + base
		}
		count := countByRepo[rp]
		var label string
		if rp == m.activeRepoPath {
			label = fmt.Sprintf("%s (%d) ●", name, count)
		} else {
			label = fmt.Sprintf("%s (%d)", name, count)
		}
		items = append(items, label)
		m.repoPickerMap[label] = rp
	}
	items = append(items, "Open folder...")
	return items
}

// switchToRepo switches the active repo based on picker selection text.
func (m *home) switchToRepo(selection string) {
	rp, ok := m.repoPickerMap[selection]
	if !ok {
		return
	}
	m.activeRepoPath = rp
	m.nav.SetRepoName(filepath.Base(rp))
	if state, ok := m.appState.(*config.State); ok {
		state.AddRecentRepo(rp)
	}
	m.rebuildInstanceList()
}

// saveAllInstances saves allInstances (all repos) to storage.
// No-ops gracefully when storage is nil (e.g. in unit tests).
func (m *home) saveAllInstances() error {
	if m.storage == nil {
		return nil
	}
	return m.storage.SaveInstances(m.allInstances)
}

// removeFromAllInstances removes an instance from the master list by title.
func (m *home) removeFromAllInstances(title string) {
	for i, inst := range m.allInstances {
		if inst.Title == title {
			m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
			return
		}
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance.
// It returns a tea.Cmd when an async operation is needed (terminal spawn).
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.nav.GetSelectedInstance()

	// Clear notification on the previously-viewed instance when the user
	// navigates away. This prevents the item from jumping out of "attention"
	// while the user is still looking at it.
	if m.seenNotified != nil && m.seenNotified != selected {
		m.seenNotified.Notified = false
		m.seenNotified = nil
		m.updateNavPanelStatus()
	}
	if selected != nil && selected.Notified {
		m.seenNotified = selected
	}

	// Manage preview terminal lifecycle on selection change.
	var spawnCmd tea.Cmd
	if selected == nil || !selected.Started() || selected.Status == session.Paused || selected.Exited {
		// No valid instance (or dead tmux session) — tear down terminal.
		if m.previewTerminal != nil {
			m.previewTerminal.Close()
		}
		m.previewTerminal = nil
		m.previewTerminalInstance = ""
	} else if selected.Title != m.previewTerminalInstance {
		// Different instance selected — swap terminal.
		if m.previewTerminal != nil {
			m.previewTerminal.Close()
		}
		m.previewTerminal = nil
		m.previewTerminalInstance = ""

		cols, rows := m.tabbedWindow.GetPreviewSize()
		if cols < 10 {
			cols = 80
		}
		if rows < 5 {
			rows = 24
		}
		capturedTitle := selected.Title
		capturedInstance := selected
		spawnCmd = func() tea.Msg {
			term, err := capturedInstance.NewEmbeddedTerminalForInstance(cols, rows)
			return previewTerminalReadyMsg{term: term, instanceTitle: capturedTitle, err: err}
		}
	}

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	m.updateInfoPane()
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// Auto-switch tab based on selection type: plan header → info tab, instance → agent tab.
	// Only auto-switch when the nav panel is focused to avoid hijacking explicit tab selection.
	if m.focusSlot == slotNav {
		if selected != nil {
			m.tabbedWindow.SetActiveTab(ui.PreviewTab)
		} else if m.nav.IsSelectedPlanHeader() {
			m.tabbedWindow.SetActiveTab(ui.InfoTab)
		}
	}

	// Collect async commands.
	var cmds []tea.Cmd
	if spawnCmd != nil {
		cmds = append(cmds, spawnCmd)
	}

	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// focusInstanceForOverlay selects the given instance in the nav panel and
// switches the preview to show its output behind an overlay dialog. The user
// can see the agent's terminal behind the overlay to understand context before
// responding. Returns a tea.Cmd if an async terminal spawn is needed.
func (m *home) focusInstanceForOverlay(inst *session.Instance) tea.Cmd {
	if inst == nil {
		return nil
	}
	m.nav.SelectInstance(inst)
	return m.instanceChanged()
}

// focusPlanInstanceForOverlay selects the best instance belonging to the given
// plan file, preferring running instances. This is used before showing overlay
// dialogs about plan lifecycle events (wave completion, etc.) so the user can
// see the agent output behind the overlay.
func (m *home) focusPlanInstanceForOverlay(planFile string) tea.Cmd {
	var best *session.Instance
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile != planFile {
			continue
		}
		if best == nil {
			best = inst
		}
		// Prefer running instances over ready/paused ones.
		if inst.Status == session.Running && best.Status != session.Running {
			best = inst
		}
	}
	return m.focusInstanceForOverlay(best)
}

func statusString(s session.Status) string {
	switch s {
	case session.Running:
		return "running"
	case session.Ready:
		return "ready"
	case session.Loading:
		return "loading"
	case session.Paused:
		return "paused"
	default:
		return "unknown"
	}
}

// updateInfoPaneForPlanHeader populates the info tab when a plan header is selected
// (no instance). Shows plan metadata and instance summary counts.
func (m *home) updateInfoPaneForPlanHeader() {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" || m.planState == nil {
		m.tabbedWindow.SetInfoData(ui.InfoData{IsPlanHeaderSelected: true})
		return
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		m.tabbedWindow.SetInfoData(ui.InfoData{IsPlanHeaderSelected: true})
		return
	}
	data := ui.InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             planstate.DisplayName(planFile),
		PlanStatus:           string(entry.Status),
		PlanTopic:            entry.Topic,
		PlanBranch:           entry.Branch,
	}
	if !entry.CreatedAt.IsZero() {
		data.PlanCreated = entry.CreatedAt.Format("2006-01-02")
	}
	// Count instances belonging to this plan.
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile != planFile {
			continue
		}
		data.PlanInstanceCount++
		switch {
		case inst.Status == session.Running || inst.Status == session.Loading:
			data.PlanRunningCount++
		case inst.Status == session.Paused:
			data.PlanPausedCount++
		default:
			data.PlanReadyCount++
		}
	}
	// Include wave progress if an orchestrator exists for this plan.
	if orch, ok := m.waveOrchestrators[planFile]; ok {
		data.TotalWaves = orch.TotalWaves()
		data.TotalTasks = orch.TotalTasks()
		tasks := orch.CurrentWaveTasks()
		data.WaveTasks = make([]ui.WaveTaskInfo, len(tasks))
		for i, task := range tasks {
			state := "pending"
			if orch.IsTaskComplete(task.Number) {
				state = "complete"
			} else if orch.IsTaskFailed(task.Number) {
				state = "failed"
			} else if orch.IsTaskRunning(task.Number) {
				state = "running"
			}
			data.WaveTasks[i] = ui.WaveTaskInfo{Number: task.Number, State: state}
		}
	}
	m.tabbedWindow.SetInfoData(data)
}

// updateInfoPane refreshes the info tab data from the selected instance or plan header.
func (m *home) updateInfoPane() {
	selected := m.nav.GetSelectedInstance()
	if selected == nil {
		// No instance selected — check if a plan header is selected.
		if m.nav.IsSelectedPlanHeader() {
			m.updateInfoPaneForPlanHeader()
			return
		}
		m.tabbedWindow.SetInfoData(ui.InfoData{HasInstance: false})
		return
	}

	data := ui.InfoData{
		HasInstance: true,
		Title:       selected.Title,
		Program:     selected.Program,
		Branch:      selected.Branch,
		Path:        selected.Path,
		Status:      statusString(selected.Status),
		AgentType:   selected.AgentType,
		TaskNumber:  selected.TaskNumber,
		WaveNumber:  selected.WaveNumber,
	}

	if !selected.CreatedAt.IsZero() {
		data.Created = selected.CreatedAt.Format("2006-01-02 15:04")
	}

	if selected.PlanFile != "" {
		if m.planState != nil {
			entry, ok := m.planState.Entry(selected.PlanFile)
			if ok {
				data.HasPlan = true
				data.PlanName = planstate.DisplayName(selected.PlanFile)
				data.PlanDescription = entry.Description
				data.PlanStatus = string(entry.Status)
				data.PlanTopic = entry.Topic
				data.PlanBranch = entry.Branch
				if !entry.CreatedAt.IsZero() {
					data.PlanCreated = entry.CreatedAt.Format("2006-01-02")
				}
			}
		}

		if orch, ok := m.waveOrchestrators[selected.PlanFile]; ok {
			data.TotalWaves = orch.TotalWaves()
			data.TotalTasks = orch.TotalTasks()
			tasks := orch.CurrentWaveTasks()
			data.WaveTasks = make([]ui.WaveTaskInfo, len(tasks))
			for i, task := range tasks {
				state := "pending"
				if orch.IsTaskComplete(task.Number) {
					state = "complete"
				} else if orch.IsTaskFailed(task.Number) {
					state = "failed"
				} else if orch.IsTaskRunning(task.Number) {
					state = "running"
				}
				data.WaveTasks[i] = ui.WaveTaskInfo{Number: task.Number, State: state}
			}
		}
	}

	m.tabbedWindow.SetInfoData(data)
}

// loadPlanState reads plan state from the active repo's docs/plans/ directory.
// When a remote store is configured, uses LoadWithStore to fetch from the server.
// Called on user-triggered events (plan creation, repo switch, etc.). The periodic
// metadata tick loads plan state in its goroutine instead.
// Silently no-ops if the file is missing (project may not use plans).
func (m *home) loadPlanState() {
	if m.planStateDir == "" {
		return
	}
	var ps *planstate.PlanState
	var err error
	if m.planStore != nil {
		ps, err = planstate.LoadWithStore(m.planStore, m.planStoreProject, m.planStateDir)
	} else {
		ps, err = planstate.Load(m.planStateDir)
	}
	if err != nil {
		log.WarningLog.Printf("could not load plan state: %v", err)
		if m.toastManager != nil {
			m.toastManager.Error("plan store error: " + err.Error())
		}
		return
	}
	m.planState = ps
}

// updateSidebarPlans pushes the current plans into the sidebar using the three-level tree API.
func (m *home) updateSidebarPlans() {
	if m.planState == nil {
		m.nav.SetTopicsAndPlans(nil, nil, nil)
		return
	}

	// Build topic displays
	topicInfos := m.planState.Topics()
	topics := make([]ui.TopicDisplay, 0, len(topicInfos))
	for _, t := range topicInfos {
		plans := m.planState.PlansByTopic(t.Name)
		planDisplays := make([]ui.PlanDisplay, 0, len(plans))
		for _, p := range plans {
			if p.Status == planstate.StatusDone || p.Status == planstate.StatusCancelled {
				continue // finished/cancelled plans handled separately
			}
			planDisplays = append(planDisplays, ui.PlanDisplay{
				Filename:    p.Filename,
				Status:      string(p.Status),
				Description: p.Description,
				Branch:      p.Branch,
				Topic:       p.Topic,
			})
		}
		if len(planDisplays) > 0 {
			topics = append(topics, ui.TopicDisplay{Name: t.Name, Plans: planDisplays})
		}
	}

	// Build ungrouped plans
	ungroupedInfos := m.planState.UngroupedPlans()
	ungrouped := make([]ui.PlanDisplay, 0, len(ungroupedInfos))
	for _, p := range ungroupedInfos {
		ungrouped = append(ungrouped, ui.PlanDisplay{
			Filename:    p.Filename,
			Status:      string(p.Status),
			Description: p.Description,
			Branch:      p.Branch,
		})
	}

	// Flatten single-plan topics where topic name matches the plan display name.
	// These don't benefit from a topic header — show the plan directly as ungrouped.
	filtered := topics[:0]
	for _, t := range topics {
		if len(t.Plans) == 1 && t.Name == planstate.DisplayName(t.Plans[0].Filename) {
			ungrouped = append(ungrouped, t.Plans[0])
		} else {
			filtered = append(filtered, t)
		}
	}
	topics = filtered

	// Build history
	finishedInfos := m.planState.Finished()
	history := make([]ui.PlanDisplay, 0, len(finishedInfos))
	for _, p := range finishedInfos {
		history = append(history, ui.PlanDisplay{
			Filename:    p.Filename,
			Status:      string(p.Status),
			Description: p.Description,
			Branch:      p.Branch,
			Topic:       p.Topic,
		})
	}

	// Set plan statuses before the rebuild so navPlanSortKey uses
	// up-to-date running/notification flags in a single pass.
	m.nav.SetPlanStatuses(m.computePlanStatuses())

	m.nav.SetTopicsAndPlans(topics, ungrouped, history)

}

// checkPlanCompletion scans running coder instances for plans that have been
// marked "done" by the agent and, if found, transitions them to reviewer sessions.
// Returns a cmd to start the reviewer (may be nil).
func (m *home) checkPlanCompletion() tea.Cmd {
	if m.planState == nil {
		return nil
	}
	// Guard: if a reviewer already exists for a plan, do not spawn another.
	// The async metadata tick can overwrite m.planState with a stale snapshot
	// that still shows StatusDone after transitionToReview already ran and set
	// StatusReviewing. Without this guard, a second reviewer is spawned.
	reviewerPlans := make(map[string]bool)
	for _, inst := range m.nav.GetInstances() {
		if inst.IsReviewer && inst.PlanFile != "" {
			reviewerPlans[inst.PlanFile] = true
		}
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile == "" || inst.IsReviewer {
			continue
		}
		if inst.ImplementationComplete {
			continue // already went through review cycle — don't re-trigger
		}
		if reviewerPlans[inst.PlanFile] {
			continue // reviewer already spawned; skip regardless of stale plan state
		}
		if !m.planState.IsDone(inst.PlanFile) {
			continue
		}
		return m.transitionToReview(inst)
	}
	return nil
}

// transitionToReview marks a plan as "reviewing", pauses the coder session,
// spawns a reviewer session with the reviewer profile, and returns the start cmd.
func (m *home) transitionToReview(coderInst *session.Instance) tea.Cmd {
	// Guard: transition via FSM before next tick re-reads disk, preventing double-spawn.
	planFile := coderInst.PlanFile
	if err := m.fsm.Transition(planFile, planfsm.ImplementFinished); err != nil {
		log.WarningLog.Printf("could not set plan %q to reviewing: %v", planFile, err)
		// Mark complete to break the retry loop — checkPlanCompletion fires
		// every tick and would re-attempt this transition indefinitely.
		coderInst.ImplementationComplete = true
		return nil // FSM rejected — plan is not in implementing state, don't spawn reviewer.
	}

	// Auto-pause the coder instance — its work is done.
	coderInst.ImplementationComplete = true
	if err := coderInst.Pause(); err != nil {
		log.WarningLog.Printf("could not pause coder instance for %q: %v", planFile, err)
	}

	return m.spawnReviewer(planFile)
}

// spawnReviewer creates and starts a reviewer session for the given plan,
// using the plan's shared worktree so it reviews the actual implementation branch.
// Does NOT perform any FSM transition — the caller is responsible for that.
// Solo agent plans are excluded — the user ends those manually.
func (m *home) spawnReviewer(planFile string) tea.Cmd {
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile == planFile && inst.SoloAgent {
			return nil
		}
	}
	planName := planstate.DisplayName(planFile)
	planPath := "docs/plans/" + planFile
	prompt := scaffold.LoadReviewPrompt(planPath, planName)

	// Kill any previous reviewer for this plan so the new session gets a fresh
	// tmux session instead of reattaching to a stale/errored one.
	m.killExistingPlanAgent(planFile, session.AgentTypeReviewer)

	// Resolve the plan's branch for the shared worktree.
	branch := m.planBranch(planFile)
	if branch == "" {
		log.WarningLog.Printf("could not resolve branch for plan %q", planFile)
		return nil
	}

	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     planName + "-review",
		Path:      m.activeRepoPath,
		Program:   m.programForAgent(session.AgentTypeReviewer),
		PlanFile:  planFile,
		AgentType: session.AgentTypeReviewer,
	})
	if err != nil {
		log.WarningLog.Printf("could not create reviewer instance for %q: %v", planFile, err)
		return nil
	}
	reviewerInst.IsReviewer = true
	reviewerInst.QueuedPrompt = prompt
	reviewerInst.SetStatus(session.Loading)

	m.addInstanceFinalizer(reviewerInst, m.nav.AddInstance(reviewerInst))
	m.nav.SelectInstance(reviewerInst) // sort-order safe, unlike index arithmetic

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned reviewer for %s", planName),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(reviewerInst.Title),
		auditlog.WithAgent(session.AgentTypeReviewer),
	)

	m.toastManager.Success(fmt.Sprintf("implementation complete → review started for %s", planName))

	shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, branch)
	return func() tea.Msg {
		if err := shared.Setup(); err != nil {
			return instanceStartedMsg{instance: reviewerInst, err: err}
		}
		err := reviewerInst.StartInSharedWorktree(shared, branch)
		return instanceStartedMsg{instance: reviewerInst, err: err}
	}
}

func withOpenCodeModelFlag(program, model string) string {
	model = normalizeOpenCodeModelID(model)
	if model == "" {
		return program
	}

	tokens := strings.Fields(program)
	if len(tokens) == 0 {
		return program
	}
	if filepath.Base(tokens[0]) != "opencode" {
		return program
	}

	for i, tok := range tokens {
		if tok == "--model" || tok == "-m" {
			if i+1 < len(tokens) {
				return program
			}
			return program
		}
		if strings.HasPrefix(tok, "--model=") {
			return program
		}
	}

	return program + " --model " + model
}

// programForAgent resolves the program command for a given agent type
// (e.g. "coder", "planner") using the kasmos config profile. Falls back to
// m.program if no profile is configured.
//
// For typed agents (coder/planner/reviewer), opencode's own --agent flag
// handles model selection via its agent config, so we do NOT append --model.
// For ad-hoc instances (no agent type), we append --model since there is no
// --agent flag to drive model selection.
func (m *home) programForAgent(agentType string) string {
	if m.appConfig == nil {
		return m.program
	}
	// Map agent types to config phases or profile names.
	var profile config.AgentProfile
	switch agentType {
	case session.AgentTypeCoder:
		profile = m.appConfig.ResolveProfile("implementing", m.program)
	case session.AgentTypePlanner:
		profile = m.appConfig.ResolveProfile("planning", m.program)
	case session.AgentTypeReviewer:
		profile = m.appConfig.ResolveProfile("quality_review", m.program)
	case session.AgentTypeFixer:
		profile = m.appConfig.ResolveProfile("fixer", m.program)
	default:
		// Ad-hoc — use the "chat" profile if available.
		if p, ok := m.appConfig.Profiles["chat"]; ok && p.Enabled && p.Program != "" {
			profile = p
		} else {
			return m.program
		}
		// Ad-hoc sessions have no --agent flag, so pass --model explicitly.
		prog := profile.BuildCommand()
		return withOpenCodeModelFlag(prog, profile.Model)
	}
	// Typed agents: opencode handles model via --agent <type> + its own config.
	return profile.BuildCommand()
}

func normalizeOpenCodeModelID(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.Contains(model, "/") {
		return model
	}
	if strings.HasPrefix(model, "claude-") {
		return "anthropic/" + model
	}
	return model
}

// killExistingPlanAgent finds and kills any existing instance for the given plan
// and agent type, removing it from both the UI list and persistence list.
//
// IMPORTANT: Instances are removed from both lists BEFORE killing the tmux
// session. This prevents the metadata-tick death-detection from seeing a dead
// reviewer in the list and auto-firing ReviewApproved (which would prematurely
// mark the plan as done).
//
// For reviewers, also matches legacy instances that only have IsReviewer set.
func (m *home) killExistingPlanAgent(planFile, agentType string) {
	// First pass: identify matching instances by title.
	var titles []string
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile != planFile {
			continue
		}
		match := inst.AgentType == agentType
		// Legacy reviewer instances may only have IsReviewer without AgentType.
		if !match && agentType == session.AgentTypeReviewer && inst.IsReviewer {
			match = true
		}
		if match {
			titles = append(titles, inst.Title)
		}
	}

	// Second pass: remove from both lists, then kill tmux.
	// Removal first ensures the death-detection tick cannot see the dead instance.
	for _, title := range titles {
		inst := m.nav.RemoveByTitle(title)
		m.removeFromAllInstances(title)
		if inst != nil {
			if err := inst.Kill(); err != nil {
				log.WarningLog.Printf("could not kill old %s for %q: %v", agentType, planFile, err)
			}
		}
	}
}

// spawnCoderWithFeedback creates and starts a coder session for the given plan,
// injecting reviewer feedback into the implementation prompt. Uses the plan's
// shared worktree so fixes are applied to the actual implementation branch.
// Does NOT perform any FSM transition — the caller is responsible for that.
func (m *home) spawnCoderWithFeedback(planFile, feedback string) tea.Cmd {
	planName := planstate.DisplayName(planFile)
	prompt := buildImplementPrompt(planFile)
	if feedback != "" {
		prompt += fmt.Sprintf("\n\nReviewer feedback from previous round:\n%s", feedback)
	}

	// Kill any previous coder for this plan so the new session gets a fresh
	// tmux session instead of reattaching to a stale/errored one.
	m.killExistingPlanAgent(planFile, session.AgentTypeCoder)

	// Resolve the plan's branch for the shared worktree.
	branch := m.planBranch(planFile)
	if branch == "" {
		log.WarningLog.Printf("could not resolve branch for plan %q", planFile)
		return nil
	}

	coderInst, err := session.NewInstance(session.InstanceOptions{
		Title:     planName + "-implement",
		Path:      m.activeRepoPath,
		Program:   m.programForAgent(session.AgentTypeCoder),
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	if err != nil {
		log.WarningLog.Printf("could not create coder instance for %q: %v", planFile, err)
		return nil
	}
	coderInst.QueuedPrompt = prompt
	coderInst.SetStatus(session.Loading)

	m.addInstanceFinalizer(coderInst, m.nav.AddInstance(coderInst))
	m.nav.SelectInstance(coderInst)

	detail := ""
	if feedback != "" {
		if len(feedback) > 200 {
			detail = feedback[:200] + "..."
		} else {
			detail = feedback
		}
	}
	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned coder with reviewer feedback for %s", planName),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(coderInst.Title),
		auditlog.WithAgent(session.AgentTypeCoder),
		auditlog.WithDetail(detail),
	)

	m.toastManager.Info(fmt.Sprintf("review changes requested → re-implementing %s", planName))

	shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, branch)
	return func() tea.Msg {
		if err := shared.Setup(); err != nil {
			return instanceStartedMsg{instance: coderInst, err: err}
		}
		err := coderInst.StartInSharedWorktree(shared, branch)
		return instanceStartedMsg{instance: coderInst, err: err}
	}
}

// viewSelectedPlan renders the selected plan's markdown in the preview pane.
// The rendered output is cached; on cache miss the glamour render runs async
// via a tea.Cmd so the UI stays responsive.
func (m *home) viewSelectedPlan() (tea.Model, tea.Cmd) {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}

	// Cache hit — reuse previously rendered content (instant).
	if planFile == m.cachedPlanFile && m.cachedPlanRendered != "" {
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
		m.tabbedWindow.SetDocumentContent(m.cachedPlanRendered)
		return m, nil
	}

	// Cache miss — render async so the UI doesn't freeze.
	planPath := filepath.Join(m.planStateDir, planFile)
	previewWidth, _ := m.tabbedWindow.GetPreviewSize()
	wordWrap := previewWidth - 4
	if wordWrap < 40 {
		wordWrap = 40
	}

	return m, func() tea.Msg {
		data, err := os.ReadFile(planPath)
		if err != nil {
			return planRenderedMsg{err: fmt.Errorf("could not read plan %s: %w", planFile, err)}
		}

		renderer, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wordWrap),
		)
		if err != nil {
			return planRenderedMsg{err: fmt.Errorf("could not create markdown renderer: %w", err)}
		}

		rendered, err := renderer.Render(string(data))
		if err != nil {
			return planRenderedMsg{err: fmt.Errorf("could not render markdown: %w", err)}
		}

		return planRenderedMsg{planFile: planFile, rendered: rendered}
	}
}

// createPlanEntry creates a new plan entry in plan-state.json (or remote store).
func (m *home) createPlanEntry(name, description, topic string) error {
	if m.planState == nil {
		var ps *planstate.PlanState
		var err error
		if m.planStore != nil {
			ps, err = planstate.LoadWithStore(m.planStore, m.planStoreProject, m.planStateDir)
		} else {
			ps, err = planstate.Load(m.planStateDir)
		}
		if err != nil {
			return err
		}
		m.planState = ps
	}

	slug := slugifyPlanName(name)
	filename := fmt.Sprintf("%s-%s.md", time.Now().UTC().Format("2006-01-02"), slug)
	branch := "plan/" + slug
	if err := m.planState.Create(filename, description, branch, topic, time.Now().UTC()); err != nil {
		if m.toastManager != nil {
			m.toastManager.Error("plan store error: " + err.Error())
		}
		return err
	}
	m.updateSidebarPlans()
	return nil
}

// slugifyPlanName converts a plan name to a URL-safe slug.
func slugifyPlanName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// buildPlanFilename derives the plan filename from a human name and creation time.
// "Auth Refactor" → "2026-02-21-auth-refactor.md"
func buildPlanFilename(name string, now time.Time) string {
	slug := slugifyPlanName(name)
	if slug == "" {
		slug = "plan"
	}
	return now.UTC().Format("2006-01-02") + "-" + slug + ".md"
}

// renderPlanStub returns the initial markdown content for a new plan file.
func renderPlanStub(name, description, filename string) string {
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by kas lifecycle flow\n- Plan file: %s\n", name, description, filename)
}

// createPlanRecord registers the plan in plan-state.json (or remote store).
func (m *home) createPlanRecord(planFile, description, branch string, now time.Time) error {
	if m.planState == nil {
		var ps *planstate.PlanState
		var err error
		if m.planStore != nil {
			ps, err = planstate.LoadWithStore(m.planStore, m.planStoreProject, m.planStateDir)
		} else {
			ps, err = planstate.Load(m.planStateDir)
		}
		if err != nil {
			return err
		}
		m.planState = ps
	}
	if err := m.planState.Register(planFile, description, branch, now); err != nil {
		if m.toastManager != nil {
			m.toastManager.Error("plan store error: " + err.Error())
		}
		return err
	}
	return nil
}

// finalizePlanCreation writes the plan stub file, registers it in plan-state.json,
// commits both to main, and creates the feature branch. Called at the end of the
// plan creation wizard.
func (m *home) finalizePlanCreation(name, description string) error {
	now := time.Now().UTC()
	planFile := buildPlanFilename(name, now)
	branch := gitpkg.PlanBranchFromFile(planFile)
	planPath := filepath.Join(m.planStateDir, planFile)

	if err := os.MkdirAll(m.planStateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(planPath, []byte(renderPlanStub(name, description, planFile)), 0o644); err != nil {
		return err
	}
	if err := m.createPlanRecord(planFile, description, branch, now); err != nil {
		return err
	}
	if err := gitpkg.CommitPlanScaffoldOnMain(m.activeRepoPath, planFile); err != nil {
		return err
	}
	if err := gitpkg.EnsurePlanBranch(m.activeRepoPath, branch); err != nil {
		return err
	}

	m.loadPlanState()
	m.updateSidebarPlans()
	return nil
}

func (m *home) importClickUpTask(task *clickup.Task) (tea.Model, tea.Cmd) {
	if task == nil {
		m.toastManager.Error("clickup fetch failed: empty task payload")
		return m, m.toastTickCmd()
	}

	date := time.Now().Format("2006-01-02")
	filename := clickup.ScaffoldFilename(task.Name, date)

	plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		m.toastManager.Error("failed to create plans dir: " + err.Error())
		return m, m.toastTickCmd()
	}
	filename = dedupePlanFilename(plansDir, filename)
	planPath := filepath.Join(plansDir, filename)

	scaffold := clickup.ScaffoldPlan(*task)
	if err := os.WriteFile(planPath, []byte(scaffold), 0o644); err != nil {
		m.toastManager.Error("failed to write plan: " + err.Error())
		return m, m.toastTickCmd()
	}

	if m.planState == nil {
		m.loadPlanState()
	}
	if m.planState == nil {
		m.toastManager.Error("failed to register imported plan: plan state unavailable")
		return m, m.toastTickCmd()
	}

	branch := gitpkg.PlanBranchFromFile(filename)
	if err := m.planState.Register(filename, task.Name, branch, time.Now()); err != nil {
		m.toastManager.Error("failed to register imported plan: " + err.Error())
		return m, m.toastTickCmd()
	}

	if err := m.fsm.Transition(filename, planfsm.PlanStart); err != nil {
		log.WarningLog.Printf("clickup import transition failed for %q: %v", filename, err)
	}

	m.loadPlanState()
	m.updateSidebarPlans()

	prompt := fmt.Sprintf(`Analyze this imported ClickUp task. The task details and subtasks are included as reference in the plan file.

Determine if the task is well-specified enough for implementation or needs further analysis. Write a proper implementation plan with ## Wave sections, task breakdowns, architecture notes, and tech stack. Use the ClickUp subtasks as a starting point but reorganize into waves based on dependencies.

The plan file is at: docs/plans/%s`, filename)

	m.toastManager.Success("imported! spawning planner...")
	model, cmd := m.spawnPlanAgent(filename, "plan", prompt)
	if cmd == nil {
		return model, m.toastTickCmd()
	}
	return model, tea.Batch(cmd, m.toastTickCmd())
}

func dedupePlanFilename(plansDir, filename string) string {
	planPath := filepath.Join(plansDir, filename)
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return filename
	}

	base := strings.TrimSuffix(filename, ".md")
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d.md", base, i)
		if _, err := os.Stat(filepath.Join(plansDir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}

	return filename
}

// shouldPromptPushAfterCoderExit returns true when a coder session has exited
// (tmuxAlive == false) and the plan is still in the implementing state.
func shouldPromptPushAfterCoderExit(entry planstate.PlanEntry, inst *session.Instance, tmuxAlive bool) bool {
	if inst == nil {
		return false
	}
	if inst.PlanFile == "" {
		return false
	}
	if inst.AgentType != session.AgentTypeCoder {
		return false
	}
	if inst.SoloAgent {
		return false
	}
	if entry.Status != planstate.StatusImplementing {
		return false
	}
	return !tmuxAlive
}

// promptPushBranchThenAdvance shows a confirmation overlay asking the user to
// push the implementation branch, then advances the plan to reviewing and
// spawns a reviewer agent via coderCompleteMsg.
func (m *home) promptPushBranchThenAdvance(inst *session.Instance) tea.Cmd {
	capturedPlanFile := inst.PlanFile
	capturedTitle := inst.Title
	message := fmt.Sprintf("[!] implementation finished for '%s'. push branch now?", planstate.DisplayName(capturedPlanFile))
	pushAction := func() tea.Msg {
		worktree, err := inst.GetGitWorktree()
		if err == nil {
			_ = worktree.PushChanges(
				fmt.Sprintf("[kas] push completed implementation for '%s'", capturedTitle),
				false,
			)
		}
		return coderCompleteMsg{planFile: capturedPlanFile}
	}
	return m.confirmAction(message, func() tea.Msg { return pushAction() })
}

// planBranch resolves the branch name for a plan, backfilling if needed.
func (m *home) planBranch(planFile string) string {
	if m.planState == nil {
		return ""
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return ""
	}
	if entry.Branch == "" {
		entry.Branch = gitpkg.PlanBranchFromFile(planFile)
		_ = m.planState.SetBranch(planFile, entry.Branch)
	}
	return entry.Branch
}

// buildPlanPrompt returns the initial prompt for a planner agent session.
// The prompt explicitly requires ## Wave N headers because kasmos uses them
// for wave orchestration — without them, implementation cannot start.
func buildPlanPrompt(planName, description string) string {
	return fmt.Sprintf(
		"Plan %s. Goal: %s. "+
			"Use the `kasmos-planner` skill. "+
			"The plan MUST include ## Wave N sections (at minimum ## Wave 1) "+
			"grouping all tasks — kasmos requires Wave headers to orchestrate implementation.",
		planName, description,
	)
}

// buildWaveAnnotationPrompt returns the prompt used when a planner is respawned
// to add ## Wave headers to an existing plan that is missing them.
// It instructs the planner to annotate the plan, commit the change, and write
// the sentinel signal so kasmos can resume the implementation flow.
func buildWaveAnnotationPrompt(planFile string) string {
	return fmt.Sprintf(
		"The plan at docs/plans/%[1]s is missing ## Wave N headers required for kasmos wave orchestration. "+
			"Please annotate the plan by wrapping all tasks under ## Wave N sections. "+
			"Every plan needs at least ## Wave 1 — even single-task trivial plans. "+
			"Keep all existing task content intact; only add the ## Wave headers.\n\n"+
			"After annotating:\n"+
			"1. Commit: git add docs/plans/%[1]s && git commit -m \"plan: add wave headers to %[1]s\"\n"+
			"2. Signal completion: touch docs/plans/.signals/planner-finished-%[1]s\n"+
			"Do not edit plan-state.json directly.",
		planFile,
	)
}

// buildImplementPrompt returns the prompt for a coder agent session.
func buildImplementPrompt(planFile string) string {
	return fmt.Sprintf(
		"Implement docs/plans/%s using the `kasmos-coder` skill. Execute all tasks sequentially.",
		planFile,
	)
}

// buildSoloPrompt returns a minimal prompt for a solo agent session.
// If planFile is non-empty, it references the plan file. Otherwise just name + description.
func buildSoloPrompt(planName, description, planFile string) string {
	if planFile != "" {
		return fmt.Sprintf("Implement %s. Goal: %s. Plan: docs/plans/%s", planName, description, planFile)
	}
	return fmt.Sprintf("Implement %s. Goal: %s.", planName, description)
}

// buildModifyPlanPrompt returns the prompt for modifying an existing plan.
func buildModifyPlanPrompt(planFile string) string {
	return fmt.Sprintf("Modify existing plan at docs/plans/%s. Keep the same filename and update only what changed.", planFile)
}

// agentTypeForSubItem maps a sidebar stage name to the corresponding AgentType constant.
func agentTypeForSubItem(action string) (string, bool) {
	switch action {
	case "plan":
		return session.AgentTypePlanner, true
	case "implement", "solo":
		return session.AgentTypeCoder, true
	case "review":
		return session.AgentTypeReviewer, true
	default:
		return "", false
	}
}

// spawnAdHocAgent creates and starts an ad-hoc agent session (no plan, no lifecycle).
// branch and workPath are optional overrides - empty strings use defaults.
func (m *home) spawnAdHocAgent(name, branch, workPath string) (tea.Model, tea.Cmd) {
	path := m.activeRepoPath
	if workPath != "" {
		path = workPath
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   name,
		Path:    path,
		Program: m.programForAgent(session.AgentTypeFixer),
	})
	if err != nil {
		return m, m.handleError(err)
	}

	inst.AgentType = session.AgentTypeFixer
	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 8
	inst.LoadingMessage = "preparing session..."

	m.state = stateDefault
	m.menu.SetState(ui.StateDefault)

	var startCmd tea.Cmd
	switch {
	case workPath != "" && branch == "":
		// Path override only - run in-place on main branch (no worktree)
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
		}

	case branch != "":
		// Branch override - create worktree on specified branch
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnBranch(branch)}
		}

	default:
		// No overrides - run in-place on current branch (no worktree)
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
		}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned fixer agent: %s", name),
		auditlog.WithInstance(name),
		auditlog.WithAgent(session.AgentTypeFixer),
	)

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)
	return m, tea.Batch(tea.WindowSize(), startCmd)
}

// spawnPlanAgent creates and starts an agent session for the given plan and action.
func (m *home) spawnPlanAgent(planFile, action, prompt string) (tea.Model, tea.Cmd) {
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
	}

	agentType, ok := agentTypeForSubItem(action)
	if !ok {
		return m, m.handleError(fmt.Errorf("unknown plan action: %s", action))
	}

	// Kill any existing instance of the same type for this plan to prevent
	// duplicates (e.g. user triggers "start review" when a reviewer already exists).
	if agentType == session.AgentTypeReviewer || agentType == session.AgentTypePlanner {
		m.killExistingPlanAgent(planFile, agentType)
	}

	title := planstate.DisplayName(planFile) + "-" + action
	if action == "solo" {
		title = "solo agent"
	}
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      m.activeRepoPath,
		Program:   m.programForAgent(agentType),
		PlanFile:  planFile,
		AgentType: agentType,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	// Keep IsReviewer in sync with AgentType so the reviewer-completion check
	// in the metadata tick handler (which gates on inst.IsReviewer) fires for
	// sidebar-spawned reviewers as well as auto-spawned ones.
	if agentType == session.AgentTypeReviewer {
		inst.IsReviewer = true
	}
	if action == "solo" {
		inst.SoloAgent = true
	}
	inst.QueuedPrompt = prompt

	// Set loading state immediately so the UI shows the progress bar
	// instead of the idle banner while the async start runs.
	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 5
	inst.LoadingMessage = "Preparing session..."

	var startCmd tea.Cmd
	if action == "plan" || action == "solo" {
		// Planner and solo agent run on main branch — no worktree created.
		startCmd = func() tea.Msg {
			err := inst.StartOnMainBranch()
			return instanceStartedMsg{instance: inst, err: err}
		}
	} else {
		// Backfill branch name for plans created before the branch field was introduced.
		if entry.Branch == "" {
			entry.Branch = gitpkg.PlanBranchFromFile(planFile)
			if err := m.planState.SetBranch(planFile, entry.Branch); err != nil {
				return m, m.handleError(fmt.Errorf("failed to assign branch for plan: %w", err))
			}
		}

		// Coder and reviewer share the plan's feature branch worktree
		shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, entry.Branch)
		if err := shared.Setup(); err != nil {
			return m, m.handleError(err)
		}
		startCmd = func() tea.Msg {
			err := inst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: inst, err: err}
		}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned %s for plan %s", agentType, planstate.DisplayName(planFile)),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(title),
		auditlog.WithAgent(agentType),
	)

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)
	return m, tea.Batch(tea.WindowSize(), startCmd)
}

// getTopicNames returns existing topic names for the picker.
func (m *home) getTopicNames() []string {
	if m.planState == nil {
		return nil
	}
	topics := m.planState.Topics()
	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.Name
	}
	return names
}

// rebuildOrphanedOrchestrators reconstructs in-memory WaveOrchestrators for plans that
// were mid-wave when kasmos was restarted. Without this, the wave completion monitor and
// the "Mark complete" context menu action are both inoperative after a restart.
//
// For each plan that is StatusImplementing and has task instances (TaskNumber > 0) but
// no orchestrator, we:
//  1. Parse the plan file to get the wave/task structure.
//  2. Fast-forward the orchestrator to the wave the instances are on.
//  3. Mark tasks as complete for instances that are already paused (finished their work).
//
// Tasks that are still running remain in taskRunning state so the metadata tick can
// detect their completion normally (or the user can mark them complete manually).
func (m *home) rebuildOrphanedOrchestrators() {
	if m.planState == nil || m.planStateDir == "" {
		return
	}

	// Group task instances by plan file.
	type taskInst struct {
		taskNumber int
		waveNumber int
		paused     bool
	}
	byPlan := make(map[string][]taskInst)
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskNumber == 0 || inst.PlanFile == "" {
			continue
		}
		byPlan[inst.PlanFile] = append(byPlan[inst.PlanFile], taskInst{
			taskNumber: inst.TaskNumber,
			waveNumber: inst.WaveNumber,
			paused:     inst.Paused(),
		})
	}

	for planFile, tasks := range byPlan {
		// Skip if orchestrator already exists.
		if _, exists := m.waveOrchestrators[planFile]; exists {
			continue
		}
		// Only reconstruct for implementing plans.
		entry, ok := m.planState.Entry(planFile)
		if !ok || entry.Status != planstate.StatusImplementing {
			continue
		}

		// Parse the plan file.
		content, err := os.ReadFile(filepath.Join(m.planStateDir, planFile))
		if err != nil {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: cannot read %s: %v", planFile, err)
			continue
		}
		plan, err := planparser.Parse(string(content))
		if err != nil {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: cannot parse %s: %v", planFile, err)
			continue
		}

		orch := NewWaveOrchestrator(planFile, plan)

		// Determine which wave the instances are on (use the max wave number seen).
		targetWave := 0
		for _, t := range tasks {
			if t.waveNumber > targetWave {
				targetWave = t.waveNumber
			}
		}

		// Fast-forward the orchestrator wave-by-wave up to the target wave.
		// Waves before the target are considered fully complete.
		for orch.currentWave < len(plan.Waves) {
			orch.StartNextWave()
			if orch.CurrentWaveNumber() == targetWave {
				break
			}
			// Mark all tasks in this earlier wave as complete to advance.
			for _, t := range plan.Waves[orch.currentWave].Tasks {
				orch.MarkTaskComplete(t.Number)
			}
		}

		// Now apply the actual task states for the current wave.
		for _, t := range tasks {
			if t.waveNumber != targetWave {
				continue
			}
			if t.paused {
				orch.MarkTaskComplete(t.taskNumber)
			}
			// Running tasks stay in taskRunning — metadata tick handles them.
		}

		m.waveOrchestrators[planFile] = orch
		log.WarningLog.Printf("rebuildOrphanedOrchestrators: restored orchestrator for %s (wave %d, %d tasks)",
			planFile, targetWave, len(tasks))
	}
}

// spawnWaveTasks creates and starts instances for the given task list within an orchestrator.
// Used by both startNextWave (initial spawn) and retryFailedWaveTasks (re-spawn failed tasks).
func (m *home) spawnWaveTasks(orch *WaveOrchestrator, tasks []planparser.Task, entry planstate.PlanEntry) (tea.Model, tea.Cmd) {
	planFile := orch.PlanFile()
	planName := planstate.DisplayName(planFile)

	// Set up shared worktree for all tasks in this batch.
	shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, entry.Branch)
	if err := shared.Setup(); err != nil {
		return m, m.handleError(err)
	}

	var cmds []tea.Cmd
	for _, task := range tasks {
		prompt := buildTaskPrompt(orch.plan, task, orch.CurrentWaveNumber(), orch.TotalWaves(), len(tasks))

		inst, err := session.NewInstance(session.InstanceOptions{
			Title:      fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number),
			Path:       m.activeRepoPath,
			Program:    m.programForAgent(session.AgentTypeCoder),
			PlanFile:   planFile,
			AgentType:  session.AgentTypeCoder,
			TaskNumber: task.Number,
			WaveNumber: orch.CurrentWaveNumber(),
			PeerCount:  len(tasks),
		})
		if err != nil {
			return m, m.handleError(err)
		}
		inst.QueuedPrompt = prompt
		inst.SetStatus(session.Loading)
		inst.LoadingTotal = 6
		inst.LoadingMessage = "Connecting to shared worktree..."

		// AddInstance registers in the list immediately; finalizer sets repo name after start.
		m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))

		m.audit(auditlog.EventAgentSpawned,
			fmt.Sprintf("spawned coder for wave %d task %d", orch.CurrentWaveNumber(), task.Number),
			auditlog.WithPlan(planFile),
			auditlog.WithInstance(inst.Title),
			auditlog.WithAgent(session.AgentTypeCoder),
			auditlog.WithWave(orch.CurrentWaveNumber(), task.Number),
		)

		taskInst := inst // capture for closure
		startCmd := func() tea.Msg {
			err := taskInst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: taskInst, err: err}
		}
		cmds = append(cmds, startCmd)
	}

	cmds = append(cmds, tea.WindowSize(), m.toastTickCmd())
	return m, tea.Batch(cmds...)
}

// startNextWave advances the orchestrator to the next wave and spawns its task instances.
func (m *home) startNextWave(orch *WaveOrchestrator, entry planstate.PlanEntry) (tea.Model, tea.Cmd) {
	tasks := orch.StartNextWave()
	if tasks == nil {
		return m, nil
	}

	waveNum := orch.CurrentWaveNumber()
	m.toastManager.Info(fmt.Sprintf("wave %d started: %d task(s) running", waveNum, len(tasks)))
	m.audit(auditlog.EventWaveStarted,
		fmt.Sprintf("wave %d started: %d task(s)", waveNum, len(tasks)),
		auditlog.WithPlan(orch.PlanFile()),
		auditlog.WithWave(waveNum, 0))
	return m.spawnWaveTasks(orch, tasks, entry)
}

// retryFailedWaveTasks retries all failed tasks in the current wave by re-spawning them.
// Old failed instances are removed first to prevent ghost duplicates that accumulate
// across retries and all get marked ImplementationComplete when waves finish.
func (m *home) retryFailedWaveTasks(orch *WaveOrchestrator, entry planstate.PlanEntry) (tea.Model, tea.Cmd) {
	tasks := orch.RetryFailedTasks()
	if len(tasks) == 0 {
		return m, nil
	}

	// Build a set of task numbers being retried for fast lookup.
	retryingTasks := make(map[int]bool, len(tasks))
	for _, t := range tasks {
		retryingTasks[t.Number] = true
	}

	// Remove old failed instances for the tasks being retried.
	// Collect first to avoid mutating the list while iterating.
	planFile := orch.PlanFile()
	var staleInsts []*session.Instance
	for _, inst := range m.nav.GetInstances() {
		if inst.PlanFile == planFile && retryingTasks[inst.TaskNumber] {
			staleInsts = append(staleInsts, inst)
		}
	}
	for _, inst := range staleInsts {
		m.nav.RemoveByTitle(inst.Title)
		m.removeFromAllInstances(inst.Title)
	}

	m.toastManager.Info(fmt.Sprintf("retrying %d failed task(s) in wave %d",
		len(tasks), orch.CurrentWaveNumber()))
	return m.spawnWaveTasks(orch, tasks, entry)
}

// discoverTmuxSessions returns a tea.Cmd that lists all kas_ tmux sessions (managed + orphaned).
func (m *home) discoverTmuxSessions() tea.Cmd {
	knownNames := make([]string, 0, len(m.allInstances))
	for _, inst := range m.allInstances {
		if inst.Started() && inst.TmuxAlive() {
			knownNames = append(knownNames, tmux.ToKasTmuxNamePublic(inst.Title))
		}
	}
	return func() tea.Msg {
		sessions, err := tmux.DiscoverAll(cmd2.MakeExecutor(), knownNames)
		return tmuxSessionsMsg{sessions: sessions, err: err}
	}
}

// buildChatAboutPlanPrompt builds the custodian prompt for a chat-about-plan session.
func buildChatAboutPlanPrompt(planFile string, entry planstate.PlanEntry, question string) string {
	name := planstate.DisplayName(planFile)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are answering a question about the plan '%s'.\n\n", name))
	sb.WriteString("## Plan Context\n\n")
	sb.WriteString(fmt.Sprintf("- **File:** docs/plans/%s\n", planFile))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", entry.Status))
	if entry.Description != "" {
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", entry.Description))
	}
	if entry.Branch != "" {
		sb.WriteString(fmt.Sprintf("- **Branch:** %s\n", entry.Branch))
	}
	if entry.Topic != "" {
		sb.WriteString(fmt.Sprintf("- **Topic:** %s\n", entry.Topic))
	}
	sb.WriteString(fmt.Sprintf("\nRead the plan file at docs/plans/%s for full details.\n\n", planFile))
	sb.WriteString("## User Question\n\n")
	sb.WriteString(question)
	return sb.String()
}

// spawnChatAboutPlan spawns a custodian agent pre-loaded with the plan context and user question.
func (m *home) spawnChatAboutPlan(planFile, question string) (tea.Model, tea.Cmd) {
	if m.planState == nil {
		return m, m.handleError(fmt.Errorf("no plan state loaded"))
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
	}
	prompt := buildChatAboutPlanPrompt(planFile, entry, question)
	planName := planstate.DisplayName(planFile)
	title := planName + "-chat"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      m.activeRepoPath,
		Program:   m.programForAgent(session.AgentTypeFixer),
		PlanFile:  planFile,
		AgentType: session.AgentTypeFixer,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	inst.QueuedPrompt = prompt
	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 5
	inst.LoadingMessage = "preparing chat..."

	// Use the plan's branch worktree if available, otherwise main.
	var startCmd tea.Cmd
	branch := m.planBranch(planFile)
	if branch != "" {
		shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, branch)
		startCmd = func() tea.Msg {
			if err := shared.Setup(); err != nil {
				return instanceStartedMsg{instance: inst, err: err}
			}
			err := inst.StartInSharedWorktree(shared, branch)
			return instanceStartedMsg{instance: inst, err: err}
		}
	} else {
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
		}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned custodian chat for %s", planstate.DisplayName(planFile)),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(title),
		auditlog.WithAgent(session.AgentTypeFixer),
	)

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)
	return m, tea.Batch(tea.WindowSize(), startCmd)
}

// adoptOrphanSession creates a new Instance backed by an existing orphaned tmux session.
func (m *home) adoptOrphanSession(item overlay.TmuxBrowserItem) (tea.Model, tea.Cmd) {
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   item.Title,
		Path:    m.activeRepoPath,
		Program: "unknown",
	})
	if err != nil {
		return m, m.handleError(err)
	}

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("adopted orphan session: %s", item.Title),
		auditlog.WithInstance(item.Title),
	)

	m.toastManager.Info(fmt.Sprintf("adopting session '%s'", item.Title))

	return m, func() tea.Msg {
		err := inst.AdoptOrphanTmuxSession(item.Name)
		return instanceStartedMsg{instance: inst, err: err}
	}
}

// audit emits a structured audit event, automatically filling in the Project
// field from m.planStoreProject. Optional fields (PlanFile, InstanceTitle,
// AgentType, WaveNumber, TaskNumber, Detail, Level) can be set via EventOption
// functional options: WithPlan, WithInstance, WithAgent, WithWave, WithDetail,
// WithLevel.
func (m *home) audit(kind auditlog.EventKind, msg string, opts ...auditlog.EventOption) {
	if m.auditLogger == nil {
		return
	}
	e := auditlog.Event{
		Kind:    kind,
		Project: m.planStoreProject,
		Message: msg,
	}
	for _, opt := range opts {
		opt(&e)
	}
	m.auditLogger.Emit(e)
	m.refreshAuditPane()
}

// refreshAuditPane queries the audit logger and updates the audit pane display.
// Shows a global activity feed — not filtered by sidebar selection.
func (m *home) refreshAuditPane() {
	if m.auditPane == nil || m.auditLogger == nil {
		return
	}

	filter := auditlog.QueryFilter{
		Project: m.planStoreProject,
		Limit:   50,
	}

	events, err := m.auditLogger.Query(filter)
	if err != nil {
		return
	}

	displays := make([]ui.AuditEventDisplay, 0, len(events))
	for _, e := range events {
		icon, color := ui.EventKindIcon(string(e.Kind))
		timeStr := e.Timestamp.Format("15:04")
		displays = append(displays, ui.AuditEventDisplay{
			Time:    timeStr,
			Kind:    string(e.Kind),
			Icon:    icon,
			Message: e.Message,
			Color:   color,
			Level:   e.Level,
		})
	}

	m.auditPane.SetEvents(displays)

	// Push updated audit view into the nav panel.
	if m.nav != nil && m.auditPane.Visible() {
		m.nav.SetAuditView(m.auditPane.String(), m.auditPane.Height())
	}
}
