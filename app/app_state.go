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
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/ui"

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
		RepoName:  filepath.Base(m.activeRepoPath),
		FocusMode: m.state == stateFocusAgent,
	}

	if m.sidebar == nil || m.list == nil {
		if data.Branch == "" {
			data.Branch = "main"
		}
		return data
	}

	planFile := m.sidebar.GetSelectedPlanFile()
	selected := m.list.GetSelectedInstance()

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

func (m *home) updateSidebarItems() {
	// Count running/notification instances for status (used by the "All" count badge).
	// Since topics are now plan-state-based (not instance-based), we still track
	// instance activity for the top-level status indicators.
	topicStatuses := make(map[string]ui.TopicStatus)
	planStatuses := make(map[string]ui.TopicStatus)
	ungroupedCount := 0

	for _, inst := range m.list.GetInstances() {
		ungroupedCount++
		started := inst.Started()

		st := topicStatuses[""]
		topicStatuses[""] = mergeTopicStatus(st, inst, started)

		if inst.PlanFile == "" {
			continue
		}
		planSt := planStatuses[inst.PlanFile]
		planStatuses[inst.PlanFile] = mergePlanStatus(planSt, inst, started)
	}

	m.sidebar.SetItems(nil, nil, ungroupedCount, nil, topicStatuses, planStatuses)
}

// focusSlot constants for readability.
const (
	slotSidebar = 0
	slotAgent   = 1
	slotDiff    = 2
	slotInfo    = 3
	slotList    = 4
	slotCount   = 5
)

// setFocusSlot updates which pane has focus and syncs visual state.
func (m *home) setFocusSlot(slot int) {
	m.focusSlot = slot
	m.sidebar.SetFocused(slot == slotSidebar)
	m.list.SetFocused(slot == slotList)
	m.menu.SetFocusSlot(slot)

	// Center pane is focused when any of the 3 center tabs is active.
	centerFocused := slot >= slotAgent && slot <= slotInfo
	m.tabbedWindow.SetFocused(centerFocused)

	// When focusing a center tab, switch the visible tab to match and track which tab is focused.
	if centerFocused {
		m.tabbedWindow.SetActiveTab(slot - slotAgent) // slotAgent=1 → PreviewTab=0, etc.
		m.tabbedWindow.SetFocusedTab(slot - slotAgent)
	} else {
		m.tabbedWindow.SetFocusedTab(-1)
	}
}

// nextFocusSlot advances the focus ring forward through the 3 center tabs only.
// Tab only cycles agent → diff → info → agent. Use 's'/'t' to reach the sidebars.
func (m *home) nextFocusSlot() {
	switch m.focusSlot {
	case slotAgent:
		m.setFocusSlot(slotDiff)
	case slotDiff:
		m.setFocusSlot(slotInfo)
	default: // slotInfo, slotSidebar, slotList — all land on agent
		m.setFocusSlot(slotAgent)
	}
}

// prevFocusSlot moves the focus ring backward through the 3 center tabs only.
func (m *home) prevFocusSlot() {
	switch m.focusSlot {
	case slotDiff:
		m.setFocusSlot(slotAgent)
	case slotInfo:
		m.setFocusSlot(slotDiff)
	default: // slotAgent, slotSidebar, slotList — all land on info
		m.setFocusSlot(slotInfo)
	}
}

// enterFocusMode enters focus/insert mode and starts the fast preview ticker.
// enterFocusMode reuses the existing previewTerminal if it is already attached to
// the selected instance — entering focus just toggles key forwarding to the same
// terminal. Only spawns a new terminal if none is attached yet (rare fallback).
func (m *home) enterFocusMode() tea.Cmd {
	m.tabbedWindow.ClearDocumentMode()
	selected := m.list.GetSelectedInstance()
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

// switchToTab switches to the specified tab slot.
func (m *home) switchToTab(name keys.KeyName) (tea.Model, tea.Cmd) {
	var targetSlot int
	switch name {
	case keys.KeyTabAgent:
		targetSlot = slotAgent
	case keys.KeyTabDiff:
		targetSlot = slotDiff
	case keys.KeyTabInfo:
		targetSlot = slotInfo
	default:
		return m, nil
	}

	if m.focusSlot == targetSlot {
		return m, nil
	}

	m.setFocusSlot(targetSlot)
	return m, m.instanceChanged()
}

// filterInstancesByTopic updates the instance list highlight filter based on the
// current sidebar selection. In tree mode, this highlights matching instances and
// boosts them to the top. In flat mode, it falls back to the existing SetFilter behavior.
func (m *home) filterInstancesByTopic() {
	selectedID := m.sidebar.GetSelectedID()

	if !m.sidebar.IsTreeMode() {
		// Flat mode fallback: reachable during startup before SetTopicsAndPlans
		// is first called (e.g. rebuildInstanceList calls us before updateSidebarPlans).
		// Once tree mode is active it is never disabled, so this path is transient.
		switch {
		case selectedID == ui.SidebarAll:
			m.list.SetFilter("")
		case selectedID == ui.SidebarUngrouped:
			m.list.SetFilter(ui.SidebarUngrouped)
		case strings.HasPrefix(selectedID, ui.SidebarPlanPrefix):
			m.list.SetFilter("")
		default:
			m.list.SetFilter(selectedID)
		}
		return
	}

	// Tree mode: use highlight filter
	switch {
	case strings.HasPrefix(selectedID, ui.SidebarPlanPrefix):
		planFile := selectedID[len(ui.SidebarPlanPrefix):]
		m.list.SetHighlightFilter("plan", planFile)
	case strings.HasPrefix(selectedID, ui.SidebarTopicPrefix):
		topicName := selectedID[len(ui.SidebarTopicPrefix):]
		m.list.SetHighlightFilter("topic", topicName)
	case strings.HasPrefix(selectedID, ui.SidebarPlanStagePrefix):
		// Stage selected — highlight parent plan
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile != "" {
			m.list.SetHighlightFilter("plan", planFile)
		} else {
			m.list.SetHighlightFilter("", "")
		}
	default:
		m.list.SetHighlightFilter("", "")
	}
}

// filterSearchWithTopic applies the search query scoped to the currently selected topic.
func (m *home) filterSearchWithTopic() {
	query := strings.ToLower(m.sidebar.GetSearchQuery())
	selectedID := m.sidebar.GetSelectedID()
	topicFilter := ""
	switch selectedID {
	case ui.SidebarAll:
		topicFilter = ""
	case ui.SidebarUngrouped:
		topicFilter = ui.SidebarUngrouped
	default:
		topicFilter = selectedID
	}
	m.list.SetSearchFilterWithTopic(query, topicFilter)
}

func (m *home) filterBySearch() {
	query := strings.ToLower(m.sidebar.GetSearchQuery())
	if query == "" {
		m.sidebar.UpdateMatchCounts(nil, 0)
		m.filterInstancesByTopic()
		return
	}
	m.list.SetSearchFilter(query)

	// Calculate match counts for sidebar dimming
	matchesByTopic := make(map[string]int)
	totalMatches := 0
	for _, inst := range m.list.GetInstances() {
		if strings.Contains(strings.ToLower(inst.Title), query) {
			matchesByTopic[""]++
			totalMatches++
		}
	}
	m.sidebar.UpdateMatchCounts(matchesByTopic, totalMatches)
}

// rebuildInstanceList clears the list and repopulates with instances matching activeRepoPath.
func (m *home) rebuildInstanceList() {
	m.list.Clear()
	for _, inst := range m.allInstances {
		repoPath := inst.GetRepoPath()
		if repoPath == "" || repoPath == m.activeRepoPath {
			m.list.AddInstance(inst)()
		}
	}
	m.filterInstancesByTopic()
	// Reload plan state for the new active repo.
	m.planStateDir = filepath.Join(m.activeRepoPath, "docs", "plans")
	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateSidebarItems()
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
	m.sidebar.SetRepoName(filepath.Base(rp))
	if state, ok := m.appState.(*config.State); ok {
		state.AddRecentRepo(rp)
	}
	m.rebuildInstanceList()
}

// saveAllInstances saves allInstances (all repos) to storage.
func (m *home) saveAllInstances() error {
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
	selected := m.list.GetSelectedInstance()

	// Clear notification when user selects this instance — they've seen it
	if selected != nil && selected.Notified {
		selected.Notified = false
		m.updateSidebarItems()
	}

	// Manage preview terminal lifecycle on selection change.
	var spawnCmd tea.Cmd
	if selected == nil || !selected.Started() || selected.Status == session.Paused {
		// No valid instance — tear down terminal.
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

// updateInfoPane refreshes the info tab data from the selected instance.
func (m *home) updateInfoPane() {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
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

// loadPlanState reads plan-state.json from the active repo's docs/plans/ directory.
// Called on user-triggered events (plan creation, repo switch, etc.). The periodic
// metadata tick loads plan state in its goroutine instead.
// Silently no-ops if the file is missing (project may not use plans).
func (m *home) loadPlanState() {
	if m.planStateDir == "" {
		return
	}
	ps, err := planstate.Load(m.planStateDir)
	if err != nil {
		log.WarningLog.Printf("could not load plan state: %v", err)
		return
	}
	m.planState = ps
}

// updateSidebarPlans pushes the current plans into the sidebar using the three-level tree API.
func (m *home) updateSidebarPlans() {
	if m.planState == nil {
		m.sidebar.SetTopicsAndPlans(nil, nil, nil)
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

	// Feed flat-mode plan list (active plans only — cancelled are hidden)
	allVisiblePlans := make([]ui.PlanDisplay, 0, len(ungrouped))
	allVisiblePlans = append(allVisiblePlans, ungrouped...)
	for _, t := range topics {
		allVisiblePlans = append(allVisiblePlans, t.Plans...)
	}
	m.sidebar.SetPlans(allVisiblePlans)

	m.sidebar.SetTopicsAndPlans(topics, ungrouped, history)

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
	for _, inst := range m.list.GetInstances() {
		if inst.IsReviewer && inst.PlanFile != "" {
			reviewerPlans[inst.PlanFile] = true
		}
	}
	for _, inst := range m.list.GetInstances() {
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
	for _, inst := range m.list.GetInstances() {
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

	// Use the reviewer profile if configured, otherwise fall back to default program.
	reviewProfile := m.appConfig.ResolveProfile("spec_review", m.program)
	reviewProgram := reviewProfile.BuildCommand()
	reviewProgram = withOpenCodeModelFlag(reviewProgram, reviewProfile.Model)

	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     planName + "-review",
		Path:      m.activeRepoPath,
		Program:   reviewProgram,
		PlanFile:  planFile,
		AgentType: session.AgentTypeReviewer,
	})
	if err != nil {
		log.WarningLog.Printf("could not create reviewer instance for %q: %v", planFile, err)
		return nil
	}
	reviewerInst.IsReviewer = true
	reviewerInst.QueuedPrompt = prompt

	m.addInstanceFinalizer(reviewerInst, m.list.AddInstance(reviewerInst))
	m.list.SelectInstance(reviewerInst) // sort-order safe, unlike index arithmetic

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
	for _, inst := range m.list.GetInstances() {
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
		inst := m.list.RemoveByTitle(title)
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
		Program:   m.program,
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	if err != nil {
		log.WarningLog.Printf("could not create coder instance for %q: %v", planFile, err)
		return nil
	}
	coderInst.QueuedPrompt = prompt

	m.addInstanceFinalizer(coderInst, m.list.AddInstance(coderInst))
	m.list.SelectInstance(coderInst)

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
	planFile := m.sidebar.GetSelectedPlanFile()
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

// createPlanEntry creates a new plan entry in plan-state.json.
func (m *home) createPlanEntry(name, description, topic string) error {
	if m.planState == nil {
		ps, err := planstate.Load(m.planStateDir)
		if err != nil {
			return err
		}
		m.planState = ps
	}

	slug := slugifyPlanName(name)
	filename := fmt.Sprintf("%s-%s.md", time.Now().UTC().Format("2006-01-02"), slug)
	branch := "plan/" + slug
	if err := m.planState.Create(filename, description, branch, topic, time.Now().UTC()); err != nil {
		return err
	}
	m.updateSidebarPlans()
	m.updateSidebarItems()
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

// createPlanRecord registers the plan in plan-state.json (in-memory + persisted).
func (m *home) createPlanRecord(planFile, description, branch string, now time.Time) error {
	if m.planState == nil {
		ps, err := planstate.Load(m.planStateDir)
		if err != nil {
			return err
		}
		m.planState = ps
	}
	return m.planState.Register(planFile, description, branch, now)
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
	m.updateSidebarItems()
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
	m.updateSidebarItems()

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
func buildPlanPrompt(planName, description string) string {
	return fmt.Sprintf("Plan %s. Goal: %s.", planName, description)
}

// buildImplementPrompt returns the prompt for a coder agent session.
func buildImplementPrompt(planFile string) string {
	return fmt.Sprintf(
		"Implement docs/plans/%s using the executing-plans superpowers skill. Execute all tasks sequentially.",
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
		Program: m.program,
	})
	if err != nil {
		return m, m.handleError(err)
	}

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

	m.addInstanceFinalizer(inst, m.list.AddInstance(inst))
	m.list.SelectInstance(inst)
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

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     planstate.DisplayName(planFile) + "-" + action,
		Path:      m.activeRepoPath,
		Program:   m.program,
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

	m.addInstanceFinalizer(inst, m.list.AddInstance(inst))
	m.list.SelectInstance(inst)
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
	for _, inst := range m.list.GetInstances() {
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
		prompt := buildTaskPrompt(orch.plan, task, orch.CurrentWaveNumber(), orch.TotalWaves())

		inst, err := session.NewInstance(session.InstanceOptions{
			Title:      fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number),
			Path:       m.activeRepoPath,
			Program:    m.program,
			PlanFile:   planFile,
			AgentType:  session.AgentTypeCoder,
			TaskNumber: task.Number,
			WaveNumber: orch.CurrentWaveNumber(),
		})
		if err != nil {
			return m, m.handleError(err)
		}
		inst.QueuedPrompt = prompt
		inst.SetStatus(session.Loading)
		inst.LoadingTotal = 6
		inst.LoadingMessage = "Connecting to shared worktree..."

		// AddInstance registers in the list immediately; finalizer sets repo name after start.
		m.addInstanceFinalizer(inst, m.list.AddInstance(inst))

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
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == planFile && retryingTasks[inst.TaskNumber] {
			staleInsts = append(staleInsts, inst)
		}
	}
	for _, inst := range staleInsts {
		m.list.RemoveByTitle(inst.Title)
		m.removeFromAllInstances(inst.Title)
	}

	m.toastManager.Info(fmt.Sprintf("retrying %d failed task(s) in wave %d",
		len(tasks), orch.CurrentWaveNumber()))
	return m.spawnWaveTasks(orch, tasks, entry)
}
