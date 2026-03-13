package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	tea "charm.land/bubbletea/v2"
)

// shouldCreatePR returns true when a plan entry is eligible for automatic PR creation:
// the plan is done, has a branch, and does not already have a PR URL.
func shouldCreatePR(entry taskstore.TaskEntry) bool {
	return entry.Status == taskstore.StatusDone && entry.Branch != "" && entry.PRURL == ""
}

// toTaskFSMHooks converts a slice of config.TOMLHook to taskfsm.HookConfig entries.
func toTaskFSMHooks(entries []config.TOMLHook) []taskfsm.HookConfig {
	out := make([]taskfsm.HookConfig, len(entries))
	for i, h := range entries {
		out[i] = taskfsm.HookConfig{
			Type:    h.Type,
			URL:     h.URL,
			Headers: h.Headers,
			Command: h.Command,
			Events:  h.Events,
		}
	}
	return out
}

// ensureProcessor lazily initializes and returns the signal Processor.
// Returns nil when taskStore is not set (e.g. in tests that don't need signal processing),
// in which case the caller must fall back to the legacy FSM signal handling code.
func (m *home) ensureProcessor() *loop.Processor {
	autoReviewFix := false
	maxCycles := 0
	if m.appConfig != nil {
		autoReviewFix = m.appConfig.AutoReviewFix
		maxCycles = m.appConfig.MaxReviewFixCycles
	}
	if m.processor != nil {
		m.processor.SetReviewFixConfig(autoReviewFix, maxCycles)
		return m.processor
	}
	if m.taskStore == nil {
		return nil
	}
	var hooks *taskfsm.HookRegistry
	if m.appConfig != nil {
		if len(m.appConfig.Hooks) > 0 {
			hooks = taskfsm.BuildHookRegistry(toTaskFSMHooks(m.appConfig.Hooks))
		}
	}
	m.processor = loop.NewProcessor(loop.ProcessorConfig{
		AutoReviewFix:      autoReviewFix,
		Store:              m.taskStore,
		Project:            m.taskStoreProject,
		Dir:                m.taskStateDir,
		MaxReviewFixCycles: maxCycles,
		Hooks:              hooks,
	})
	return m.processor
}

func (m *home) handleReviewChangesRequested(planFile, feedback string) tea.Cmd {
	m.pendingReviewFeedback[planFile] = feedback

	var cmds []tea.Cmd
	truncated := feedback
	if len(truncated) > 200 {
		truncated = truncated[:200] + "..."
	}
	if cmd := m.postClickUpProgress(planFile, "review_changes_requested", truncated); cmd != nil {
		cmds = append(cmds, cmd)
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.IsReviewer {
			_ = inst.Pause()
			break
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func assemblePRMetadata(
	entry taskstore.TaskEntry,
	subtasks []taskstore.SubtaskEntry,
	reviewerSummary string,
	reviewCycle int,
	gitChanges, gitCommits, gitStats string,
) gitpkg.PRMetadata {
	meta := gitpkg.PRMetadata{
		Description:     strings.TrimSpace(entry.Description),
		Goal:            strings.TrimSpace(entry.Goal),
		ReviewerSummary: strings.TrimSpace(reviewerSummary),
		ReviewCycle:     reviewCycle,
		GitChanges:      strings.TrimSpace(gitChanges),
		GitCommits:      strings.TrimSpace(gitCommits),
		GitStats:        strings.TrimSpace(gitStats),
		Subtasks:        make([]gitpkg.PRSubtask, 0, len(subtasks)),
	}

	if strings.TrimSpace(entry.Content) != "" {
		if plan, err := taskparser.Parse(entry.Content); err == nil {
			meta.Architecture = strings.TrimSpace(plan.Architecture)
			meta.TechStack = strings.TrimSpace(plan.TechStack)
		}
	}

	for _, s := range subtasks {
		meta.Subtasks = append(meta.Subtasks, gitpkg.PRSubtask{
			Number: s.TaskNumber,
			Title:  strings.TrimSpace(s.Title),
			Status: string(s.Status),
		})
	}

	return meta
}

// mapPRReviewDecision maps GitHub review decision strings to internal representation.
func mapPRReviewDecision(ghValue string) string {
	switch ghValue {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes_requested"
	default:
		return "pending"
	}
}

// mapPRCheckStatus maps GitHub check status strings to internal representation.
func mapPRCheckStatus(ghValue string) string {
	switch ghValue {
	case "SUCCESS":
		return "passing"
	case "FAILURE", "ERROR":
		return "failing"
	default:
		return "pending"
	}
}

// createPRAfterApproval returns an async tea.Cmd that creates a GitHub PR for the given
// plan file, posts an approving review with the reviewer's body, and reports the URL back.
func (m *home) createPRAfterApproval(planFile, reviewBody string) tea.Cmd {
	repoPath := m.activeRepoPath
	store := m.taskStore
	project := m.taskStoreProject
	planName := taskstate.DisplayName(planFile)

	return func() tea.Msg {
		entry, err := store.Get(project, planFile)
		if err != nil {
			log.WarningLog.Printf("createPRAfterApproval: could not get entry for %q: %v", planFile, err)
			return nil
		}
		if entry.Branch == "" {
			log.WarningLog.Printf("createPRAfterApproval: no branch for %q — skipping PR creation", planFile)
			return nil
		}

		shared := gitpkg.NewSharedTaskWorktree(repoPath, entry.Branch)
		if err := shared.Setup(); err != nil {
			log.WarningLog.Printf("createPRAfterApproval: worktree setup failed for %q: %v", planFile, err)
			return nil
		}

		subtasks := []taskstore.SubtaskEntry(nil)
		if subtasksFromStore, err := store.GetSubtasks(project, planFile); err == nil {
			subtasks = subtasksFromStore
		} else {
			log.WarningLog.Printf("createPRAfterApproval: failed to load subtasks for %q: %v", planFile, err)
		}

		base := shared.GetBaseCommitSHA()
		gitChanges, gitCommits, gitStats := "", "", ""
		if base != "" {
			if files, err := exec.Command("git", "-C", shared.GetWorktreePath(), "diff", "--name-only", base).CombinedOutput(); err == nil {
				gitChanges = strings.TrimSpace(string(files))
			}
			if commits, err := exec.Command("git", "-C", shared.GetWorktreePath(), "log", "--oneline", base+"..HEAD").CombinedOutput(); err == nil {
				gitCommits = strings.TrimSpace(string(commits))
			}
			if stats, err := exec.Command("git", "-C", shared.GetWorktreePath(), "diff", "--stat", base).CombinedOutput(); err == nil {
				gitStats = strings.TrimSpace(string(stats))
			}
		}

		meta := assemblePRMetadata(entry, subtasks, reviewBody, entry.ReviewCycle, gitChanges, gitCommits, gitStats)
		title := gitpkg.BuildPRTitle(entry.Description, planName)
		body := gitpkg.BuildPRBody(meta)
		commitMsg := fmt.Sprintf("[kas] implementation of '%s'", planName)
		if err := shared.CreatePR(title, body, commitMsg); err != nil {
			log.WarningLog.Printf("createPRAfterApproval: PR creation failed for %q: %v", planFile, err)
			return nil
		}

		state, err := shared.QueryPRState()
		if err != nil {
			log.WarningLog.Printf("createPRAfterApproval: QueryPRState failed for %q: %v", planFile, err)
			return nil
		}
		if state.URL == "" {
			log.WarningLog.Printf("createPRAfterApproval: empty URL for %q after PR creation", planFile)
			return nil
		}

		if state.Number > 0 {
			if err := shared.PostGitHubReview(state.Number, body, true); err != nil {
				log.WarningLog.Printf("createPRAfterApproval: PostGitHubReview failed for %q: %v", planFile, err)
				// Non-fatal — PR was created, review posting failed.
			}
		}

		return prCreatedForPlanMsg{planFile: planFile, url: state.URL}
	}
}

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
	if inst.TaskFile == "" {
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

// uiToTmuxStatusBarData adapts a ui.StatusBarData to the tmux-package mirror type.
// Direct integer cast for TaskGlyph is safe — both packages use iota in the same
// order (Complete=0, Running=1, Failed=2, Pending=3).
func uiToTmuxStatusBarData(d ui.StatusBarData) tmux.StatusBarData {
	glyphs := make([]tmux.TaskGlyph, len(d.TaskGlyphs))
	for i, g := range d.TaskGlyphs {
		glyphs[i] = tmux.TaskGlyph(g)
	}
	return tmux.StatusBarData{
		Branch:           d.Branch,
		Version:          d.Version,
		PlanName:         d.PlanName,
		PlanStatus:       d.PlanStatus,
		WaveLabel:        d.WaveLabel,
		TaskGlyphs:       glyphs,
		FocusMode:        d.FocusMode,
		TmuxSessionCount: d.TmuxSessionCount,
		ProjectDir:       d.ProjectDir,
		PRState:          d.PRState,
		PRChecks:         d.PRChecks,
	}
}

// updateTmuxStatusBarCmd returns a tea.Cmd that asynchronously applies the
// rendered status bar strings to the outer tmux layout session.
//
// Guards:
//   - Returns nil when layoutSessionName is empty (not inside the two-pane layout).
//   - Returns nil when KASMOS_LAYOUT env var is not "1".
//   - Returns nil when the rendered strings are identical to the last applied
//     values (m.lastTmuxStatusLeft / m.lastTmuxStatusRight), avoiding redundant
//     subprocess calls on every metadata tick.
//
// On error the cmd logs the failure and returns nil so the metadata loop continues.
func (m *home) updateTmuxStatusBarCmd(data ui.StatusBarData) tea.Cmd {
	if m.layoutSessionName == "" || os.Getenv("KASMOS_LAYOUT") != "1" {
		return nil
	}
	render := tmux.RenderStatusBar(uiToTmuxStatusBarData(data))
	if render.Left == m.lastTmuxStatusLeft && render.Right == m.lastTmuxStatusRight {
		return nil // no-op: nothing changed
	}
	// Update the cache before spawning the cmd so a second tick that fires
	// before the first cmd completes does not send a duplicate request.
	m.lastTmuxStatusLeft = render.Left
	m.lastTmuxStatusRight = render.Right
	sessionName := m.layoutSessionName
	return func() tea.Msg {
		ex := cmd2.MakeExecutor()
		if err := tmux.ApplyStatusBar(ex, sessionName, render); err != nil {
			log.WarningLog.Printf("updateTmuxStatusBarCmd: %v", err)
		}
		return nil
	}
}

// computeStatusBarData builds the StatusBarData from the current app state.
func (m *home) computeStatusBarData() ui.StatusBarData {
	data := ui.StatusBarData{
		Version:          m.version,
		TmuxSessionCount: m.tmuxSessionCount,
		ProjectDir:       filepath.Base(m.activeRepoPath),
	}

	if m.nav == nil {
		if data.Branch == "" {
			data.Branch = currentBranch(m.activeRepoPath)
		}
		return data
	}

	planFile := m.nav.GetSelectedPlanFile()
	selected := m.nav.GetSelectedInstance()

	switch {
	case planFile != "" && m.taskState != nil:
		entry, ok := m.taskState.Entry(planFile)
		if ok {
			data.Branch = entry.Branch
			data.PlanName = taskstate.DisplayName(planFile)
			data.PlanStatus = string(entry.Status)

			if orch, orchOK := m.waveOrchestrators[planFile]; orchOK {
				waveNum := orch.CurrentWaveNumber()
				totalWaves := orch.TotalWaves()
				if waveNum > 0 {
					data.WaveLabel = fmt.Sprintf("wave %d/%d", waveNum, totalWaves)
					tasks := orch.CurrentWaveTasks()
					data.TaskGlyphs = make([]ui.TaskGlyph, len(tasks))
					for i, task := range tasks {
						switch {
						case orch.IsTaskComplete(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphComplete
						case orch.IsTaskFailed(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphFailed
						case orch.IsTaskRunning(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphRunning
						default:
							data.TaskGlyphs[i] = ui.TaskGlyphPending
						}
					}
				}
			}

			// Populate PR state from the task store (has full PR metadata).
			if m.taskStore != nil {
				if storeEntry, err := m.taskStore.Get(m.taskStoreProject, planFile); err == nil && storeEntry.PRURL != "" {
					data.PRState = mapPRReviewDecision(storeEntry.PRReviewDecision)
					data.PRChecks = mapPRCheckStatus(storeEntry.PRCheckStatus)
				}
			}
		}
	case selected != nil && selected.Branch != "":
		data.Branch = selected.Branch
		if selected.TaskFile != "" && m.taskState != nil {
			entry, ok := m.taskState.Entry(selected.TaskFile)
			if ok {
				data.PlanName = taskstate.DisplayName(selected.TaskFile)
				data.PlanStatus = string(entry.Status)

				// Populate PR state from the task store.
				if m.taskStore != nil {
					if storeEntry, err := m.taskStore.Get(m.taskStoreProject, selected.TaskFile); err == nil && storeEntry.PRURL != "" {
						data.PRState = mapPRReviewDecision(storeEntry.PRReviewDecision)
						data.PRChecks = mapPRCheckStatus(storeEntry.PRCheckStatus)
					}
				}
			}
		}
	}

	if data.Branch == "" {
		data.Branch = currentBranch(m.activeRepoPath)
	}

	return data
}

// currentBranch returns the name of the currently checked-out branch in repoPath.
// Falls back to "main" if the branch cannot be determined (e.g. detached HEAD).
func currentBranch(repoPath string) string {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "main" // detached HEAD
	}
	return branch
}

// computePlanStatuses builds per-plan instance status flags (running/notification)
// from the current instance list.
func (m *home) computePlanStatuses() map[string]ui.TopicStatus {
	planStatuses := make(map[string]ui.TopicStatus)
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == "" {
			continue
		}
		planSt := planStatuses[inst.TaskFile]
		planStatuses[inst.TaskFile] = mergePlanStatus(planSt, inst, inst.Started())
	}
	return planStatuses
}

// updateNavPanelStatus recomputes plan instance statuses and triggers a row
// rebuild. Use this after instance mutations (kill, remove, pause) where the
// plan list itself hasn't changed. When updateSidebarTasks is also called,
// skip this — updateSidebarTasks already includes plan statuses in its rebuild.
func (m *home) updateNavPanelStatus() {
	m.nav.SetItems(nil, nil, 0, nil, nil, m.computePlanStatuses())
}

// setFocusSlot updates which pane has focus and syncs visual state.
// With the tabbed window removed, only slotNav is meaningful; this method
// is kept as a thin shim so call sites in app_input.go compile unchanged.
func (m *home) setFocusSlot(_ int) {
	m.nav.SetFocused(true)
	m.menu.SetFocusSlot(0)
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

// cleanupPausedDoneReviewers removes paused reviewer instances whose plan is
// done and that the user has already navigated away from (i.e. they are not
// the currently-selected instance). This is called at the start of
// instanceChanged() to GC the reviewer once the user moves on.
func (m *home) cleanupPausedDoneReviewers(selected *session.Instance) {
	if m.taskState == nil {
		return
	}
	var toCleanup []*session.Instance
	for _, inst := range m.nav.GetInstances() {
		if !inst.IsReviewer {
			continue
		}
		if inst.Status != session.Paused {
			continue
		}
		// Don't remove the instance the user is currently looking at.
		if selected != nil && inst == selected {
			continue
		}
		entry, ok := m.taskState.Entry(inst.TaskFile)
		if !ok {
			continue
		}
		if entry.Status != taskstate.StatusDone {
			continue
		}
		toCleanup = append(toCleanup, inst)
	}
	if len(toCleanup) == 0 {
		return
	}
	for _, inst := range toCleanup {
		m.nav.RemoveByTitle(inst.Title)
		m.removeFromAllInstances(inst.Title)
		if err := inst.Kill(); err != nil {
			log.WarningLog.Printf("cleanupPausedDoneReviewers: could not kill reviewer %q: %v", inst.Title, err)
		}
	}
	m.updateNavPanelStatus()
}

// isInstanceSwappable reports whether inst is eligible to be swapped into the
// layout's right pane as the active agent view. Paused and exited instances
// are never eligible — their tmux session is either suspended or gone, and
// calling swapPaneCmd on them would leave the right pane in an undefined state.
//
// This is the authoritative check used by instanceChanged and maybeSwapPane
// to ensure a paused/exited instance is never treated as the active swap target.
func isInstanceSwappable(inst *session.Instance) bool {
	if inst == nil {
		return false
	}
	if inst.Exited || inst.Paused() {
		return false
	}
	return inst.Started()
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance.
// It returns a tea.Cmd when an async operation is needed (terminal spawn).
// Guarantee: paused and exited instances are never treated as swap targets —
// maybeSwapPane() guards against this via isInstanceSwappable.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.nav.GetSelectedInstance()
	m.cleanupPausedDoneReviewers(selected)
	selected = m.nav.GetSelectedInstance() // refresh in case list mutation changed selection

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

	m.updateInfoPane()
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// Collect async commands.
	var cmds []tea.Cmd

	// When running inside a tmux layout, swap the right pane to the selected
	// instance's session (or restore the workspace shell when no valid instance).
	if swapCmd := m.maybeSwapPane(); swapCmd != nil {
		cmds = append(cmds, swapCmd)
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
		if inst.TaskFile != planFile {
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

// findTaskTitle returns the title of the task with the given number in the plan, or "".
func findTaskTitle(plan *taskparser.Plan, taskNumber int) string {
	if plan == nil || taskNumber == 0 {
		return ""
	}
	for _, wave := range plan.Waves {
		for _, task := range wave.Tasks {
			if task.Number == taskNumber {
				return task.Title
			}
		}
	}
	return ""
}

// buildSubtaskProgress fetches subtasks from the store and groups them by wave using the
// orchestrator's plan structure. On error, prior* values are returned unchanged so that a
// transient store failure does not blank out previously displayed subtask data.
func (m *home) buildSubtaskProgress(
	planFile string,
	orch *orchestration.WaveOrchestrator,
	priorCompleted, priorTotal int,
	priorGroups []ui.WaveSubtaskGroup,
) (completed, total int, groups []ui.WaveSubtaskGroup) {
	if m.taskState == nil || orch == nil {
		return priorCompleted, priorTotal, priorGroups
	}
	subtasks, err := m.taskState.GetSubtasks(planFile)
	if err != nil {
		log.WarningLog.Printf("updateInfoPane: could not read subtasks for %q: %v", planFile, err)
		return priorCompleted, priorTotal, priorGroups
	}

	// Index subtasks by task number.
	byNumber := make(map[int]taskstore.SubtaskEntry, len(subtasks))
	for _, s := range subtasks {
		byNumber[s.TaskNumber] = s
	}

	plan := orch.Plan()
	if plan == nil {
		return priorCompleted, priorTotal, priorGroups
	}

	total = len(subtasks)
	for _, s := range subtasks {
		switch string(s.Status) {
		case "complete", "done", "closed":
			completed++
		}
	}

	groups = make([]ui.WaveSubtaskGroup, 0, len(plan.Waves))
	for _, wave := range plan.Waves {
		group := ui.WaveSubtaskGroup{WaveNumber: wave.Number}
		for _, task := range wave.Tasks {
			entry, ok := byNumber[task.Number]
			statusStr := "pending"
			if ok {
				statusStr = string(entry.Status)
			}
			title := task.Title
			if ok && entry.Title != "" {
				title = entry.Title
			}
			group.Subtasks = append(group.Subtasks, ui.SubtaskDisplay{
				Number: task.Number,
				Title:  title,
				Status: statusStr,
			})
		}
		groups = append(groups, group)
	}
	return completed, total, groups
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
	if planFile == "" || m.taskState == nil {
		m.currentDetailData = ui.InfoData{IsPlanHeaderSelected: true}
		m.nav.SetDetailData(ui.NavDetailData{InfoData: m.currentDetailData})
		return
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		m.currentDetailData = ui.InfoData{IsPlanHeaderSelected: true}
		m.nav.SetDetailData(ui.NavDetailData{InfoData: m.currentDetailData})
		return
	}
	data := ui.InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             taskstate.DisplayName(planFile),
		PlanDescription:      entry.Description,
		PlanStatus:           string(entry.Status),
		PlanTopic:            entry.Topic,
		PlanBranch:           entry.Branch,
	}
	if !entry.CreatedAt.IsZero() {
		data.PlanCreated = entry.CreatedAt.Format("2006-01-02")
	}
	// Count instances belonging to this plan.
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile != planFile {
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
	// Enrich with goal and lifecycle timestamps.
	data.PlanGoal = entry.Goal
	data.PlanningAt = entry.PlanningAt
	data.ImplementingAt = entry.ImplementingAt
	data.ReviewingAt = entry.ReviewingAt
	data.DoneAt = entry.DoneAt

	// Include wave progress if an orchestrator exists for this plan.
	var orch *orchestration.WaveOrchestrator
	if o, ok := m.waveOrchestrators[planFile]; ok {
		orch = o
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

	// Subtask progress (preserve prior zeros as initial values — plan header has no prior state).
	data.CompletedTasks, data.TotalSubtasks, data.AllWaveSubtasks =
		m.buildSubtaskProgress(planFile, orch, 0, 0, nil)

	// Review outcome — shown when the plan has been approved.
	if entry.Status == taskstate.StatusDone {
		data.ReviewOutcome = "approved"
		data.ReviewCycle = 1 // default display cycle
		if cycle, err := m.taskState.ReviewCycle(planFile); err == nil {
			data.ReviewCycle = cycle + 1
		}
	}
	if m.appConfig != nil && m.appConfig.MaxReviewFixCycles > 0 {
		data.MaxReviewFixCycles = m.appConfig.MaxReviewFixCycles
	}

	m.currentDetailData = data
	m.nav.SetDetailData(ui.NavDetailData{InfoData: data, RenderedPlan: m.cachedPlanRendered})
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
		m.currentDetailData = ui.InfoData{HasInstance: false}
		m.nav.SetDetailData(ui.NavDetailData{InfoData: ui.InfoData{HasInstance: false}})
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

	// Capture prior subtask data so we can preserve it on error.
	prior := m.currentDetailData

	if selected.TaskFile != "" {
		var orch *orchestration.WaveOrchestrator
		if m.taskState != nil {
			entry, ok := m.taskState.Entry(selected.TaskFile)
			if ok {
				data.HasPlan = true
				data.PlanName = taskstate.DisplayName(selected.TaskFile)
				data.PlanDescription = entry.Description
				data.PlanStatus = string(entry.Status)
				data.PlanTopic = entry.Topic
				data.PlanBranch = entry.Branch
				if !entry.CreatedAt.IsZero() {
					data.PlanCreated = entry.CreatedAt.Format("2006-01-02")
				}
				// Enrich with goal and lifecycle timestamps.
				data.PlanGoal = entry.Goal
				data.PlanningAt = entry.PlanningAt
				data.ImplementingAt = entry.ImplementingAt
				data.ReviewingAt = entry.ReviewingAt
				data.DoneAt = entry.DoneAt
				// Review outcome — shown when the plan has been approved.
				if entry.Status == taskstate.StatusDone {
					data.ReviewOutcome = "approved"
					data.ReviewCycle = 1 // default display cycle
					if cycle, err := m.taskState.ReviewCycle(selected.TaskFile); err == nil {
						data.ReviewCycle = cycle + 1
					}
				}
			}
		}

		if o, ok := m.waveOrchestrators[selected.TaskFile]; ok {
			orch = o
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
			// Populate TaskTitle from the plan structure.
			data.TaskTitle = findTaskTitle(orch.Plan(), selected.TaskNumber)
		}

		// Subtask progress — preserve prior values if GetSubtasks fails.
		data.CompletedTasks, data.TotalSubtasks, data.AllWaveSubtasks =
			m.buildSubtaskProgress(selected.TaskFile, orch,
				prior.CompletedTasks, prior.TotalSubtasks, prior.AllWaveSubtasks)
	}

	m.currentDetailData = data
	m.nav.SetDetailData(ui.NavDetailData{InfoData: data, RenderedPlan: m.cachedPlanRendered})
}

// loadTaskState reads plan state from the store for the active repo.
// Called on user-triggered events (plan creation, repo switch, etc.). The periodic
// metadata tick loads plan state in its goroutine instead.
// Silently no-ops if the store is not configured.
func (m *home) loadTaskState() {
	if m.taskStateDir == "" || m.taskStore == nil {
		return
	}
	ps, err := taskstate.Load(m.taskStore, m.taskStoreProject, m.taskStateDir)
	if err != nil {
		log.WarningLog.Printf("could not load plan state: %v", err)
		if m.toastManager != nil {
			m.toastManager.Error("task store error: " + err.Error())
		}
		return
	}
	m.taskState = ps
}

// updateSidebarTasks pushes the current plans into the sidebar using the three-level tree API.
func (m *home) updateSidebarTasks() {
	if m.taskState == nil {
		m.nav.SetTopicsAndPlans(nil, nil, nil)
		return
	}

	// Build topic displays
	topicInfos := m.taskState.Topics()
	topics := make([]ui.TopicDisplay, 0, len(topicInfos))
	for _, t := range topicInfos {
		plans := m.taskState.TasksByTopic(t.Name)
		planDisplays := make([]ui.PlanDisplay, 0, len(plans))
		for _, p := range plans {
			if p.Status == taskstate.StatusDone || p.Status == taskstate.StatusCancelled {
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
	ungroupedInfos := m.taskState.UngroupedTasks()
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
		if len(t.Plans) == 1 && t.Name == taskstate.DisplayName(t.Plans[0].Filename) {
			ungrouped = append(ungrouped, t.Plans[0])
		} else {
			filtered = append(filtered, t)
		}
	}
	topics = filtered

	// Build history
	finishedInfos := m.taskState.Finished()
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
	if m.taskState == nil {
		return nil
	}
	// Guard: if a reviewer already exists for a plan, do not spawn another.
	// The async metadata tick can overwrite m.taskState with a stale snapshot
	// that still shows StatusDone after transitionToReview already ran and set
	// StatusReviewing. Without this guard, a second reviewer is spawned.
	reviewerPlans := make(map[string]bool)
	for _, inst := range m.nav.GetInstances() {
		if inst.IsReviewer && inst.TaskFile != "" {
			reviewerPlans[inst.TaskFile] = true
		}
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == "" || inst.IsReviewer {
			continue
		}
		if inst.ImplementationComplete {
			continue // already went through review cycle — don't re-trigger
		}
		if reviewerPlans[inst.TaskFile] {
			continue // reviewer already spawned; skip regardless of stale plan state
		}
		if !m.taskState.IsDone(inst.TaskFile) {
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
	planFile := coderInst.TaskFile
	if err := m.fsm.Transition(planFile, taskfsm.ImplementFinished); err != nil {
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
	if !m.requireDaemonForAgents() {
		return nil
	}
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.SoloAgent {
			return nil
		}
	}
	planName := taskstate.DisplayName(planFile)
	prompt := scaffold.LoadReviewPrompt(planFile, planName)

	// Kill any previous reviewer for this plan so the new session gets a fresh
	// tmux session instead of reattaching to a stale/errored one.
	m.killExistingPlanAgent(planFile, session.AgentTypeReviewer)

	// Resolve the plan's branch for the shared worktree.
	branch := m.taskBranch(planFile)
	if branch == "" {
		log.WarningLog.Printf("could not resolve branch for plan %q", planFile)
		return nil
	}

	cycle, _ := m.taskState.ReviewCycle(planFile)
	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:         fmt.Sprintf("%s-review-%d", planName, cycle+1),
		Path:          m.activeRepoPath,
		Program:       m.programForAgent(session.AgentTypeReviewer),
		ExecutionMode: m.executionModeForAgent(session.AgentTypeReviewer),
		TaskFile:      planFile,
		AgentType:     session.AgentTypeReviewer,
		ReviewCycle:   cycle + 1,
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

	shared := gitpkg.NewSharedTaskWorktree(m.activeRepoPath, branch)
	agents := m.opencodeAgentConfigs()
	return func() tea.Msg {
		if err := shared.Setup(); err != nil {
			return instanceStartedMsg{instance: reviewerInst, err: err}
		}
		if err := scaffold.PatchWorktreeConfig(shared.GetWorktreePath(), agents); err != nil {
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

func (m *home) profileForAgent(agentType string) config.AgentProfile {
	if m.appConfig == nil {
		return config.AgentProfile{Program: m.program, ExecutionMode: config.ExecutionModeTmux}
	}

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
	case session.AgentTypeElaborator:
		profile = m.appConfig.ResolveProfile("elaborating", m.program)
	default:
		if p, ok := m.appConfig.Profiles["chat"]; ok && p.Enabled && p.Program != "" {
			profile = p
		} else {
			return config.AgentProfile{Program: m.program, ExecutionMode: config.ExecutionModeTmux}
		}
	}
	profile.ExecutionMode = config.NormalizeExecutionMode(profile.ExecutionMode)
	return profile
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
	profile := m.profileForAgent(agentType)
	if agentType == "" {
		return withOpenCodeModelFlag(profile.BuildCommand(), profile.Model)
	}
	return profile.BuildCommand()
}

func (m *home) executionModeForAgent(agentType string) session.ExecutionMode {
	mode := session.ExecutionMode(config.NormalizeExecutionMode(m.profileForAgent(agentType).ExecutionMode))
	// Headless execution is only wired for coder sessions right now.
	// Other agent roles remain tmux-attached for visibility and interactive
	// control, while still allowing them to pick up any future shared config.
	if agentType != session.AgentTypeCoder {
		return session.ExecutionModeTmux
	}
	return mode
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

func (m *home) opencodeAgentConfigs() []harness.AgentConfig {
	if m.appConfig == nil {
		return nil
	}

	configsByRole := make(map[string]harness.AgentConfig)
	resolve := func(phase, fallbackRole string) {
		profile := m.appConfig.ResolveProfile(phase, m.program)
		if !isOpenCodeProfile(profile) || !profile.Enabled {
			return
		}

		role := fallbackRole
		if mappedRole, ok := m.appConfig.PhaseRoles[phase]; ok && mappedRole != "" {
			role = mappedRole
		}

		if role == "" {
			return
		}

		if profile.Model == "" && profile.Temperature == nil && profile.Effort == "" {
			return
		}

		programFields := strings.Fields(profile.Program)
		if len(programFields) == 0 {
			return
		}

		configsByRole[role] = harness.AgentConfig{
			Role:        role,
			Harness:     filepath.Base(programFields[0]),
			Model:       normalizeOpenCodeModelID(profile.Model),
			Temperature: profile.Temperature,
			Effort:      profile.Effort,
			Enabled:     profile.Enabled,
		}
	}

	resolve("implementing", session.AgentTypeCoder)
	resolve("planning", session.AgentTypePlanner)
	resolve("quality_review", session.AgentTypeReviewer)
	resolve("fixer", session.AgentTypeFixer)
	resolve("elaborating", session.AgentTypeElaborator)

	if len(configsByRole) == 0 {
		return nil
	}

	configs := make([]harness.AgentConfig, 0, len(configsByRole))
	for _, phaseRole := range []string{session.AgentTypeCoder, session.AgentTypePlanner, session.AgentTypeReviewer, session.AgentTypeFixer, session.AgentTypeElaborator} {
		mappedRole := phaseRole
		if mappedRoleFromConfig, ok := m.appConfig.PhaseRoles[phaseToLookup(phaseRole)]; ok && mappedRoleFromConfig != "" {
			mappedRole = mappedRoleFromConfig
		}
		if cfg, ok := configsByRole[mappedRole]; ok {
			configs = append(configs, cfg)
		}
	}

	return configs
}

func isOpenCodeProfile(profile config.AgentProfile) bool {
	fields := strings.Fields(profile.Program)
	if len(fields) == 0 {
		return false
	}

	return filepath.Base(fields[0]) == "opencode"
}

func phaseToLookup(phase string) string {
	switch phase {
	case session.AgentTypeCoder:
		return "implementing"
	case session.AgentTypePlanner:
		return "planning"
	case session.AgentTypeReviewer:
		return "quality_review"
	case session.AgentTypeFixer:
		return "fixer"
	case session.AgentTypeElaborator:
		return "elaborating"
	default:
		return phase
	}
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
		if inst.TaskFile != planFile {
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

// spawnFixerWithFeedback creates and starts a fixer session for the given plan,
// injecting reviewer feedback into the implementation prompt. Uses the plan's
// shared worktree so fixes are applied to the actual implementation branch.
// Does NOT perform any FSM transition — the caller is responsible for that.
func (m *home) spawnFixerWithFeedback(planFile, feedback string) tea.Cmd {
	if !m.requireDaemonForAgents() {
		return nil
	}
	planName := taskstate.DisplayName(planFile)
	prompt := buildImplementPrompt(planFile)
	if feedback != "" {
		prompt += fmt.Sprintf("\n\nReviewer feedback from previous round:\n%s", feedback)
	}

	// Kill any previous fixer (and any legacy feedback-coder) for this plan so
	// the new session gets a fresh tmux session instead of reattaching to a
	// stale/errored one.
	m.killExistingPlanAgent(planFile, session.AgentTypeFixer)
	m.killExistingPlanAgent(planFile, session.AgentTypeCoder)

	// Clear the push-prompt dedup flag so this new coder round can trigger
	// the push dialog when it finishes.
	delete(m.coderPushPrompted, planFile)

	// Resolve the plan's branch for the shared worktree.
	branch := m.taskBranch(planFile)
	if branch == "" {
		log.WarningLog.Printf("could not resolve branch for plan %q", planFile)
		return nil
	}

	cycle, _ := m.taskState.ReviewCycle(planFile)
	fixerInst, err := session.NewInstance(session.InstanceOptions{
		Title:         fmt.Sprintf("%s-fix-%d", planName, cycle),
		Path:          m.activeRepoPath,
		Program:       m.programForAgent(session.AgentTypeFixer),
		ExecutionMode: m.executionModeForAgent(session.AgentTypeFixer),
		TaskFile:      planFile,
		AgentType:     session.AgentTypeFixer,
		ReviewCycle:   cycle,
	})
	if err != nil {
		log.WarningLog.Printf("could not create fixer instance for %q: %v", planFile, err)
		return nil
	}
	fixerInst.QueuedPrompt = prompt
	fixerInst.SetStatus(session.Loading)

	m.addInstanceFinalizer(fixerInst, m.nav.AddInstance(fixerInst))
	m.nav.SelectInstance(fixerInst)

	detail := ""
	if feedback != "" {
		if len(feedback) > 200 {
			detail = feedback[:200] + "..."
		} else {
			detail = feedback
		}
	}
	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned fixer with reviewer feedback for %s", planName),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(fixerInst.Title),
		auditlog.WithAgent(session.AgentTypeFixer),
		auditlog.WithDetail(detail),
	)

	m.toastManager.Info(fmt.Sprintf("review changes requested → applying fixes to %s", planName))

	shared := gitpkg.NewSharedTaskWorktree(m.activeRepoPath, branch)
	agents := m.opencodeAgentConfigs()
	return func() tea.Msg {
		if err := shared.Setup(); err != nil {
			return instanceStartedMsg{instance: fixerInst, err: err}
		}
		if err := scaffold.PatchWorktreeConfig(shared.GetWorktreePath(), agents); err != nil {
			return instanceStartedMsg{instance: fixerInst, err: err}
		}
		err := fixerInst.StartInSharedWorktree(shared, branch)
		return instanceStartedMsg{instance: fixerInst, err: err}
	}
}

// spawnElaborator creates and starts an elaborator agent session for the given plan.
// The elaborator runs on the main branch (not in a worktree) since it only reads the
// codebase and updates the task store — it does not modify files. When it finishes,
// it writes an elaborator-finished-<planFile> sentinel that the metadata tick picks up
// to advance the orchestrator from WaveStateElaborating to wave 1.
func (m *home) spawnElaborator(planFile string) (tea.Model, tea.Cmd) {
	if !m.requireDaemonForAgents() {
		return m, nil
	}
	planName := taskstate.DisplayName(planFile)
	prompt := orchestration.BuildElaborationPrompt(planFile)

	// Clear any stale elaborator-finished sentinel from a prior run before
	// spawning a new elaborator. Without this, a leftover file (e.g. from a
	// TUI restart mid-elaboration) would be picked up on the next tick and
	// advance the orchestrator to wave 1 before the new elaborator finishes.
	taskfsm.ClearElaborationSignal(m.signalsDir, planFile)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:         fmt.Sprintf("%s-elaborator", planName),
		Path:          m.activeRepoPath,
		Program:       m.programForAgent(session.AgentTypeElaborator),
		ExecutionMode: m.executionModeForAgent(session.AgentTypeElaborator),
		TaskFile:      planFile,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	inst.AgentType = session.AgentTypeElaborator
	inst.QueuedPrompt = prompt
	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 6
	inst.LoadingMessage = "elaborating plan..."

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))

	if err := scaffold.PatchWorktreeConfig(m.activeRepoPath, m.opencodeAgentConfigs()); err != nil {
		return m, m.handleError(err)
	}

	startCmd := func() tea.Msg {
		return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned elaborator for %s", planName),
		auditlog.WithPlan(planFile),
		auditlog.WithAgent(session.AgentTypeElaborator))

	m.toastManager.Info(fmt.Sprintf("elaborating plan '%s' before implementation", planName))
	return m, tea.Batch(tea.RequestWindowSize, startCmd, m.toastTickCmd())
}

// blueprintSkipThreshold returns the configured threshold for blueprint-skip mode.
// When the plan's total task count is <= this value, elaboration and wave orchestration
// are skipped and a single coder agent implements all tasks sequentially.
// Returns the default of 2 when appConfig is nil or not explicitly configured.
func (m *home) blueprintSkipThreshold() int {
	if m.appConfig == nil {
		return 2
	}
	return m.appConfig.BlueprintSkipThreshold()
}

// clearWaveOrchestratorState removes any wave-orchestrator bookkeeping for the
// given plan from both the home model and the processor-backed signal gate.
// This is required before switching an implementing plan onto the single-agent
// blueprint-skip path so later implement_finished signals are not suppressed.
func (m *home) clearWaveOrchestratorState(planFile string) {
	delete(m.waveOrchestrators, planFile)
	if proc := m.ensureProcessor(); proc != nil {
		proc.SetWaveOrchestratorActive(planFile, false)
	}
}

// hasActiveBlueprintSkipCoder reports whether a non-wave coder is already
// active for this plan. Used to prevent duplicate small-plan implement spawns
// when the user re-triggers "implement" while a single-agent implementation is
// already in flight.
func (m *home) hasActiveBlueprintSkipCoder(planFile string) bool {
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile != planFile || inst.AgentType != session.AgentTypeCoder {
			continue
		}
		if inst.WaveNumber != 0 || inst.SoloAgent {
			continue
		}
		if inst.ImplementationComplete || inst.Exited || inst.Paused() {
			continue
		}
		return true
	}
	return false
}

// spawnBlueprintSkipAgent transitions the plan to implementing and spawns a single
// coder agent to implement all tasks sequentially. Used when the plan's task count
// is at or below blueprintSkipThreshold(). No WaveOrchestrator is created — the
// agent signals implement_finished directly, which triggers the existing review flow.
func (m *home) spawnBlueprintSkipAgent(planFile string, plan *taskparser.Plan) (tea.Model, tea.Cmd) {
	if err := m.fsmSetImplementing(planFile); err != nil {
		return m, m.handleError(err)
	}
	m.loadTaskState()
	m.updateSidebarTasks()

	totalTasks := 0
	for _, wave := range plan.Waves {
		totalTasks += len(wave.Tasks)
	}
	m.toastManager.Info(fmt.Sprintf("small plan (%d tasks) - running single agent", totalTasks))

	model, cmd := m.spawnTaskAgent(planFile, "implement", orchestration.BuildBlueprintSkipPrompt(planFile, plan))
	return model, tea.Batch(cmd, m.toastTickCmd())
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
		m.nav.SetDetailData(ui.NavDetailData{InfoData: m.currentDetailData, RenderedPlan: m.cachedPlanRendered})
		return m, nil
	}

	// Cache miss — render async so the UI doesn't freeze.
	const wordWrap = 80

	return m, func() tea.Msg {
		data, err := m.taskStore.GetContent(m.taskStoreProject, planFile)
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

		rendered, err := renderer.Render(data)
		if err != nil {
			return planRenderedMsg{err: fmt.Errorf("could not render markdown: %w", err)}
		}

		return planRenderedMsg{planFile: planFile, rendered: rendered}
	}
}

// createTaskEntry creates a new plan entry in the store.
func (m *home) createTaskEntry(name, description, topic string) error {
	if m.taskState == nil {
		if m.taskStore == nil {
			return fmt.Errorf("task store not configured")
		}
		ps, err := taskstate.Load(m.taskStore, m.taskStoreProject, m.taskStateDir)
		if err != nil {
			return err
		}
		m.taskState = ps
	}

	slug := slugifyPlanName(name)
	filename := slug
	branch := "plan/" + slug
	if err := m.taskState.Create(filename, description, branch, topic, time.Now().UTC()); err != nil {
		if m.toastManager != nil {
			m.toastManager.Error("task store error: " + err.Error())
		}
		return err
	}
	if err := m.taskState.SetContent(filename, renderPlanStub(name, description, filename)); err != nil {
		if m.toastManager != nil {
			m.toastManager.Error("task store error: " + err.Error())
		}
		return err
	}
	m.audit(auditlog.EventPlanCreated, "created plan", auditlog.WithPlan(filename))
	m.updateSidebarTasks()
	return nil
}

// slugifyPlanName converts a plan name to a URL-safe slug.
func slugifyPlanName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// buildPlanFilename derives the plan filename from a human name.
// "Auth Refactor" → "auth-refactor"
func buildPlanFilename(name string, _ time.Time) string {
	slug := slugifyPlanName(name)
	if slug == "" {
		slug = "plan"
	}
	return slug
}

// renderPlanStub returns the initial markdown content for a new plan file.
func renderPlanStub(name, description, filename string) string {
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by kas lifecycle flow\n- Plan file: %s\n", name, description, filename)
}

// createPlanRecord registers the plan in the store.
func (m *home) createPlanRecord(planFile, description, branch string, now time.Time) error {
	if m.taskState == nil {
		if m.taskStore == nil {
			return fmt.Errorf("task store not configured")
		}
		ps, err := taskstate.Load(m.taskStore, m.taskStoreProject, m.taskStateDir)
		if err != nil {
			return err
		}
		m.taskState = ps
	}
	if err := m.taskState.Register(planFile, description, branch, now); err != nil {
		if m.toastManager != nil {
			m.toastManager.Error("task store error: " + err.Error())
		}
		return err
	}
	return nil
}

// finalizePlanCreation writes the plan stub content to the store, registers it,
// and creates the feature branch. Called at the end of the plan creation wizard.
func (m *home) finalizePlanCreation(name, description string) error {
	now := time.Now().UTC()
	planFile := buildPlanFilename(name, now)
	branch := gitpkg.TaskBranchFromFile(planFile)
	content := renderPlanStub(name, description, planFile)
	if err := m.createPlanRecord(planFile, description, branch, now); err != nil {
		return err
	}
	if err := m.taskState.SetContent(planFile, content); err != nil {
		return err
	}
	if err := gitpkg.EnsureTaskBranch(m.activeRepoPath, branch); err != nil {
		return err
	}

	m.loadTaskState()
	m.updateSidebarTasks()
	return nil
}

func (m *home) importClickUpTask(task *clickup.Task) (tea.Model, tea.Cmd) {
	if task == nil {
		m.toastManager.Error("clickup fetch failed: empty task payload")
		return m, m.toastTickCmd()
	}

	filename := clickup.ScaffoldFilename(task.Name)

	if m.taskState == nil {
		m.loadTaskState()
	}
	if m.taskState == nil {
		m.toastManager.Error("failed to register imported plan: plan state unavailable")
		return m, m.toastTickCmd()
	}

	filename = dedupePlanFilenameInState(m.taskState, filename)

	scaffold := clickup.ScaffoldPlan(*task)

	branch := gitpkg.TaskBranchFromFile(filename)
	if err := m.taskState.Register(filename, task.Name, branch, time.Now()); err != nil {
		m.toastManager.Error("failed to register imported plan: " + err.Error())
		return m, m.toastTickCmd()
	}
	if err := m.taskState.SetContent(filename, scaffold); err != nil {
		m.toastManager.Error("failed to save imported plan content: " + err.Error())
		return m, m.toastTickCmd()
	}
	if task.ID != "" {
		if err := m.taskState.SetClickUpTaskID(filename, task.ID); err != nil {
			log.WarningLog.Printf("importClickUpTask: failed to set clickup task id for %q: %v", filename, err)
		}
	}

	if err := m.fsm.Transition(filename, taskfsm.PlanStart); err != nil {
		log.WarningLog.Printf("clickup import transition failed for %q: %v", filename, err)
	}

	m.loadTaskState()
	m.updateSidebarTasks()

	prompt := fmt.Sprintf(`Analyze this imported ClickUp task. The task details and subtasks are included as reference in the plan.

Determine if the task is well-specified enough for implementation or needs further analysis. Write a proper implementation plan with ## Wave sections, task breakdowns, architecture notes, and tech stack. Use the ClickUp subtasks as a starting point but reorganize into waves based on dependencies.

Retrieve the current plan content with: kas task show %s`, filename)

	m.toastManager.Success("imported! spawning planner...")
	model, cmd := m.spawnTaskAgent(filename, "plan", prompt)
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

	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", filename, i)
		if _, err := os.Stat(filepath.Join(plansDir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}

	return filename
}

func dedupePlanFilenameInState(ps *taskstate.TaskState, filename string) string {
	if ps == nil {
		return filename
	}
	if _, ok := ps.Entry(filename); !ok {
		return filename
	}

	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", filename, i)
		if _, ok := ps.Entry(candidate); !ok {
			return candidate
		}
	}

	return filename
}

// shouldPromptPushAfterImplementerExit returns true when a non-solo coder or
// fixer session has finished and the plan is still in the implementing state.
// An implementer is considered finished when its tmux session has exited
// (tmuxAlive == false) OR when it has returned to a prompt after completing
// its queued work (PromptDetected && !AwaitingWork).
func shouldPromptPushAfterImplementerExit(entry taskstate.TaskEntry, inst *session.Instance, tmuxAlive bool) bool {
	if inst == nil {
		return false
	}
	if inst.TaskFile == "" {
		return false
	}
	if inst.AgentType != session.AgentTypeCoder && inst.AgentType != session.AgentTypeFixer {
		return false
	}
	if inst.SoloAgent {
		return false
	}
	if entry.Status != taskstate.StatusImplementing {
		return false
	}
	// Tmux exited — original single-coder completion path.
	if !tmuxAlive {
		return true
	}
	if config.NormalizeExecutionMode(string(inst.ExecutionMode)) == config.ExecutionModeHeadless && inst.Exited {
		return true
	}
	// Agent returned to prompt after finishing queued work — covers the
	// review-feedback fixer ("applying fixes") which stays alive in tmux.
	if inst.PromptDetected && !inst.AwaitingWork {
		return true
	}
	return false
}

// promptPushBranchThenAdvance shows a confirmation overlay asking the user to
// push the implementation branch, then advances the plan to reviewing and
// spawns a reviewer agent via coderCompleteMsg.
func (m *home) promptPushBranchThenAdvance(inst *session.Instance) tea.Cmd {
	capturedPlanFile := inst.TaskFile
	// Mark as prompted so the metadata tick doesn't re-trigger the dialog
	// while the user is deciding or after they dismiss it.
	if m.coderPushPrompted == nil {
		m.coderPushPrompted = make(map[string]bool)
	}
	m.coderPushPrompted[capturedPlanFile] = true
	message := fmt.Sprintf("[!] implementation finished for '%s'. push branch now?", taskstate.DisplayName(capturedPlanFile))
	pushAction := func() tea.Msg {
		worktree, err := inst.GetGitWorktree()
		if err == nil {
			_ = worktree.Push(false)
		}
		return coderCompleteMsg{planFile: capturedPlanFile}
	}
	return m.confirmAction(message, func() tea.Msg { return pushAction() })
}

// taskBranch resolves the branch name for a plan, backfilling if needed.
func (m *home) taskBranch(planFile string) string {
	if m.taskState == nil {
		return ""
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return ""
	}
	if entry.Branch == "" {
		entry.Branch = gitpkg.TaskBranchFromFile(planFile)
		_ = m.taskState.SetBranch(planFile, entry.Branch)
	}
	return entry.Branch
}

// buildPlanningPrompt returns the initial prompt for a planner agent session.
// The prompt explicitly requires ## Wave N headers because kasmos uses them
// for wave orchestration — without them, implementation cannot start.
func buildPlanningPrompt(planFile, planName, description string) string {
	return fmt.Sprintf(
		"Plan %s. Goal: %s. "+
			"Use the `kasmos-planner` skill. "+
			"The plan MUST include ## Wave N sections (at minimum ## Wave 1) "+
			"grouping all tasks — kasmos requires Wave headers to orchestrate implementation. "+
			"After writing the plan, store it with `kas task update-content %s` and then signal completion with `touch .kasmos/signals/planner-finished-%s`.",
		planName, description, planFile, planFile,
	)
}

// buildImplementPrompt returns the prompt for a coder agent session.
// Agents retrieve plan content from the task store via `kas task show` and write
// sentinel signals to .kasmos/signals/ in their worktree; the TUI ingests them on completion.
func buildImplementPrompt(planFile string) string {
	return fmt.Sprintf(
		"Implement %s. Retrieve the full plan with `kas task show %s` and execute all tasks sequentially. "+
			"Use rg/sd/fd instead of grep/sed/find. Scope tests with -run TestName. Do not load skills.",
		planFile, planFile,
	)
}

// buildSoloPrompt returns a minimal prompt for a solo agent session.
// If planFile is non-empty, it references the plan via kas task show. Otherwise just name + description.
func buildSoloPrompt(planName, description, planFile string) string {
	const rules = "Commit with task number in message. Use rg/sd/fd instead of grep/sed/find. Scope tests with -run TestName. Do not load skills."
	if planFile != "" {
		return fmt.Sprintf(
			"Implement %s. Goal: %s. Retrieve the full plan with `kas task show %s`. %s",
			planName, description, planFile, rules,
		)
	}
	return fmt.Sprintf("Implement %s. Goal: %s. %s", planName, description, rules)
}

// buildModifyTaskPrompt returns the prompt for modifying an existing plan.
func buildModifyTaskPrompt(planFile string) string {
	return fmt.Sprintf(
		"Modify existing plan %s. Retrieve current content with `kas task show %s`. "+
			"Keep the same filename and update only what changed.",
		planFile, planFile,
	)
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
	if !m.requireDaemonForAgents() {
		return m, nil
	}
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
	return m, tea.Batch(tea.RequestWindowSize, startCmd)
}

// spawnTaskAgent creates and starts an agent session for the given plan and action.
func (m *home) spawnTaskAgent(planFile, action, prompt string) (tea.Model, tea.Cmd) {
	if !m.requireDaemonForAgents() {
		return m, nil
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("task not found: %s", planFile))
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

	planName := taskstate.DisplayName(planFile)
	title := planName + "-" + action
	// reviewCycle is resolved once and reused for both the title and the instance field.
	var reviewCycle int
	if action == "solo" {
		// Solo sessions run on main branch, so their tmux session names must stay
		// unique to avoid accidentally reattaching to another solo session.
		title = planName + "-solo"
	} else if action == "review" {
		// Use cycle-suffixed title so each review round gets a unique tmux session name.
		reviewCycle, _ = m.taskState.ReviewCycle(planFile)
		title = fmt.Sprintf("%s-review-%d", planName, reviewCycle+1)
	}
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:         title,
		Path:          m.activeRepoPath,
		Program:       m.programForAgent(agentType),
		ExecutionMode: m.executionModeForAgent(agentType),
		TaskFile:      planFile,
		AgentType:     agentType,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	// Keep IsReviewer in sync with AgentType so the reviewer-completion check
	// in the metadata tick handler (which gates on inst.IsReviewer) fires for
	// sidebar-spawned reviewers as well as auto-spawned ones.
	if agentType == session.AgentTypeReviewer {
		inst.IsReviewer = true
		// Set ReviewCycle so the instance carries the same cycle number used in the title.
		inst.ReviewCycle = reviewCycle + 1
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
		if err := scaffold.PatchWorktreeConfig(m.activeRepoPath, m.opencodeAgentConfigs()); err != nil {
			return m, m.handleError(err)
		}
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
		}
	} else {
		// Backfill branch name for plans created before the branch field was introduced.
		if entry.Branch == "" {
			entry.Branch = gitpkg.TaskBranchFromFile(planFile)
			if err := m.taskState.SetBranch(planFile, entry.Branch); err != nil {
				return m, m.handleError(fmt.Errorf("failed to assign branch for plan: %w", err))
			}
		}

		// Coder and reviewer share the plan's feature branch worktree
		shared := gitpkg.NewSharedTaskWorktree(m.activeRepoPath, entry.Branch)
		if err := shared.Setup(); err != nil {
			return m, m.handleError(err)
		}
		if err := scaffold.PatchWorktreeConfig(shared.GetWorktreePath(), m.opencodeAgentConfigs()); err != nil {
			return m, m.handleError(err)
		}
		startCmd = func() tea.Msg {
			err := inst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: inst, err: err}
		}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned %s for plan %s", agentType, taskstate.DisplayName(planFile)),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(title),
		auditlog.WithAgent(agentType),
	)

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)
	return m, tea.Batch(tea.RequestWindowSize, startCmd)
}

// getTopicNames returns existing topic names for the picker.
func (m *home) getTopicNames() []string {
	if m.taskState == nil {
		return nil
	}
	topics := m.taskState.Topics()
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
// For each plan that is StatusImplementing and has active wave-task instances
// (TaskNumber > 0, started, not paused, not exited) but no orchestrator, we:
//  1. Parse the plan file to get the wave/task structure.
//  2. Fast-forward the orchestrator to the wave the instances are on.
//  3. Mark tasks as complete for instances that are already paused (finished their work).
//
// Tasks that are still running remain in taskRunning state so the metadata tick can
// detect their completion normally (or the user can mark them complete manually).
func (m *home) rebuildOrphanedOrchestrators() {
	if m.taskState == nil || m.taskStateDir == "" {
		return
	}

	// Group task instances by plan file.
	type taskInst struct {
		taskNumber int
		waveNumber int
		paused     bool
	}
	byPlan := make(map[string][]taskInst)
	hasActiveByPlan := make(map[string]bool)
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskNumber == 0 || inst.TaskFile == "" {
			continue
		}
		if !inst.Started() || inst.Exited {
			continue
		}
		byPlan[inst.TaskFile] = append(byPlan[inst.TaskFile], taskInst{
			taskNumber: inst.TaskNumber,
			waveNumber: inst.WaveNumber,
			paused:     inst.Paused(),
		})
		if !inst.Paused() {
			hasActiveByPlan[inst.TaskFile] = true
		}
	}

	for planFile, tasks := range byPlan {
		if !hasActiveByPlan[planFile] {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: skipping %s — no active wave instances", planFile)
			continue
		}

		// Skip if orchestrator already exists.
		if _, exists := m.waveOrchestrators[planFile]; exists {
			continue
		}
		// Only reconstruct for implementing plans.
		entry, ok := m.taskState.Entry(planFile)
		if !ok || entry.Status != taskstate.StatusImplementing {
			continue
		}

		// Parse the plan content from store.
		content, err := m.taskStore.GetContent(m.taskStoreProject, planFile)
		if err != nil {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: cannot read %s: %v", planFile, err)
			continue
		}
		plan, err := taskparser.Parse(content)
		if err != nil {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: cannot parse %s: %v", planFile, err)
			continue
		}

		// Determine which wave the instances are on (use the max wave number seen).
		targetWave := 0
		for _, t := range tasks {
			if t.waveNumber > targetWave {
				targetWave = t.waveNumber
			}
		}

		// Guard against malformed legacy task instances with no wave metadata.
		if targetWave <= 0 {
			log.WarningLog.Printf("rebuildOrphanedOrchestrators: skipping %s — invalid target wave %d", planFile, targetWave)
			continue
		}

		orch := orchestration.NewWaveOrchestrator(planFile, plan)
		orch.SetStore(m.taskStore, m.taskStoreProject)

		// Collect completed tasks for the target wave.
		var completedTasks []int
		for _, t := range tasks {
			if t.waveNumber == targetWave && t.paused {
				completedTasks = append(completedTasks, t.taskNumber)
			}
		}

		// Fast-forward the orchestrator to the target wave, marking earlier waves
		// as complete and applying actual task states for the target wave.
		orch.RestoreToWave(targetWave, completedTasks)

		m.waveOrchestrators[planFile] = orch
		log.WarningLog.Printf("rebuildOrphanedOrchestrators: restored orchestrator for %s (wave %d, %d tasks)",
			planFile, targetWave, len(tasks))
	}
}

// spawnWaveTasks creates and starts instances for the given task list within an orchestrator.
// Used by both startNextWave (initial spawn) and retryFailedWaveTasks (re-spawn failed tasks).
func (m *home) spawnWaveTasks(orch *orchestration.WaveOrchestrator, tasks []taskparser.Task, entry taskstate.TaskEntry) (tea.Model, tea.Cmd) {
	if !m.requireDaemonForAgents() {
		return m, nil
	}
	planFile := orch.TaskFile()
	planName := taskstate.DisplayName(planFile)

	// Set up shared worktree for all tasks in this batch.
	shared := gitpkg.NewSharedTaskWorktree(m.activeRepoPath, entry.Branch)
	if err := shared.Setup(); err != nil {
		return m, m.handleError(err)
	}
	if err := scaffold.PatchWorktreeConfig(shared.GetWorktreePath(), m.opencodeAgentConfigs()); err != nil {
		return m, m.handleError(err)
	}

	var cmds []tea.Cmd
	for _, task := range tasks {
		prompt := orch.BuildTaskPrompt(task, len(tasks))

		inst, err := session.NewInstance(session.InstanceOptions{
			Title:         fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number),
			Path:          m.activeRepoPath,
			Program:       m.programForAgent(session.AgentTypeCoder),
			ExecutionMode: m.executionModeForAgent(session.AgentTypeCoder),
			TaskFile:      planFile,
			AgentType:     session.AgentTypeCoder,
			TaskNumber:    task.Number,
			WaveNumber:    orch.CurrentWaveNumber(),
			PeerCount:     len(tasks),
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

		taskInst := inst // capture for closure
		startCmd := func() tea.Msg {
			err := taskInst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: taskInst, err: err}
		}
		cmds = append(cmds, startCmd)
	}

	cmds = append(cmds, tea.RequestWindowSize, m.toastTickCmd())
	return m, tea.Batch(cmds...)
}

// startNextWave advances the orchestrator to the next wave and spawns its task instances.
func (m *home) startNextWave(orch *orchestration.WaveOrchestrator, entry taskstate.TaskEntry) (tea.Model, tea.Cmd) {
	tasks := orch.StartNextWave()
	if len(tasks) == 0 {
		return m, nil
	}

	waveNum := orch.CurrentWaveNumber()
	m.toastManager.Info(fmt.Sprintf("wave %d started: %d task(s) running", waveNum, len(tasks)))
	m.audit(auditlog.EventWaveStarted,
		fmt.Sprintf("wave %d started: %d task(s)", waveNum, len(tasks)),
		auditlog.WithPlan(orch.TaskFile()),
		auditlog.WithWave(waveNum, 0))
	return m.spawnWaveTasks(orch, tasks, entry)
}

// retryFailedWaveTasks retries all failed tasks in the current wave by re-spawning them.
// Old failed instances are removed first to prevent ghost duplicates that accumulate
// across retries and all get marked ImplementationComplete when waves finish.
func (m *home) retryFailedWaveTasks(orch *orchestration.WaveOrchestrator, entry taskstate.TaskEntry) (tea.Model, tea.Cmd) {
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
	planFile := orch.TaskFile()
	var staleInsts []*session.Instance
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && retryingTasks[inst.TaskNumber] {
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

// buildChatAboutTaskPrompt builds the custodian prompt for a chat-about-plan session.
func buildChatAboutTaskPrompt(planFile string, entry taskstate.TaskEntry, question string) string {
	name := taskstate.DisplayName(planFile)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are answering a question about the plan '%s'.\n\n", name))
	sb.WriteString("## Plan Context\n\n")
	sb.WriteString(fmt.Sprintf("- **Plan:** %s\n", planFile))
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
	sb.WriteString(fmt.Sprintf("\nRetrieve the full plan with `kas task show %s` for details.\n\n", planFile))
	sb.WriteString("## User Question\n\n")
	sb.WriteString(question)
	return sb.String()
}

// spawnChatAboutTask spawns a custodian agent pre-loaded with the plan context and user question.
func (m *home) spawnChatAboutTask(planFile, question string) (tea.Model, tea.Cmd) {
	if !m.requireDaemonForAgents() {
		return m, nil
	}
	if m.taskState == nil {
		return m, m.handleError(fmt.Errorf("no task state loaded"))
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("task not found: %s", planFile))
	}
	prompt := buildChatAboutTaskPrompt(planFile, entry, question)
	planName := taskstate.DisplayName(planFile)
	title := planName + "-chat"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      m.activeRepoPath,
		Program:   m.programForAgent(session.AgentTypeFixer),
		TaskFile:  planFile,
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
	branch := m.taskBranch(planFile)
	if branch != "" {
		shared := gitpkg.NewSharedTaskWorktree(m.activeRepoPath, branch)
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

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned custodian chat for %s", taskstate.DisplayName(planFile)),
		auditlog.WithPlan(planFile),
		auditlog.WithInstance(title),
		auditlog.WithAgent(session.AgentTypeFixer),
	)

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
	m.nav.SelectInstance(inst)
	return m, tea.Batch(tea.RequestWindowSize, startCmd)
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
// field from m.taskStoreProject. Optional fields (PlanFile, InstanceTitle,
// AgentType, WaveNumber, TaskNumber, Detail, Level) can be set via EventOption
// functional options: WithPlan, WithInstance, WithAgent, WithWave, WithDetail,
// WithLevel.
func (m *home) audit(kind auditlog.EventKind, msg string, opts ...auditlog.EventOption) {
	if m.auditLogger == nil {
		return
	}
	e := auditlog.Event{
		Kind:    kind,
		Project: m.taskStoreProject,
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
		Project: m.taskStoreProject,
		Limit:   200,
	}

	events, err := m.auditLogger.Query(filter)
	if err != nil {
		return
	}

	displays := make([]ui.AuditEventDisplay, 0, len(events))
	for _, e := range events {
		icon, color := ui.EventKindIcon(string(e.Kind))
		timeStr := e.Timestamp.Local().Format("15:04")
		msg := e.Message
		// Prepend [plan-name] when the event has a plan context and the message
		// doesn't already embed the plan name (some messages include it inline).
		if e.TaskFile != "" {
			label := taskstate.DisplayName(e.TaskFile)
			if !strings.Contains(msg, label) {
				msg = "[" + label + "] " + msg
			}
		}
		displays = append(displays, ui.AuditEventDisplay{
			Time:          timeStr,
			Kind:          string(e.Kind),
			Icon:          icon,
			Message:       msg,
			Color:         color,
			Level:         e.Level,
			TaskFile:      e.TaskFile,
			InstanceTitle: e.InstanceTitle,
			AgentType:     e.AgentType,
		})
	}

	// Coalesce consecutive stopped+started pairs (newest-first order) with the
	// same HH:MM timestamp into a single "kasmos restarted" event.
	displays = coalesceRestarts(displays)

	m.auditPane.SetEvents(displays)

	// Push updated audit view into the nav panel.
	if m.nav != nil && m.auditPane.Visible() {
		m.nav.SetAuditView(m.auditPane.String(), m.auditPane.ContentLines())
	}
}

// buildClickUpProgressComment formats a concise markdown comment for ClickUp.
// Prefixes with "🤖 kasmos:" so comments are identifiable.
// Events:
//   - plan_ready: "plan finalized — {detail}"
//   - wave_complete: "wave {detail} complete"
//   - review_approved: "review approved — implementation complete"
//   - review_changes_requested: "review: changes requested — {detail}"
//   - fixer_complete: "fixer agent completed — {detail}"
func buildClickUpProgressComment(event, planName, detail string) string {
	var body string
	switch event {
	case "plan_ready":
		if detail != "" {
			body = "plan finalized — " + detail
		} else {
			body = "plan finalized"
		}
	case "wave_complete":
		if detail != "" {
			body = "wave " + detail + " complete"
		} else {
			body = "wave complete"
		}
	case "review_approved":
		body = "review approved — implementation complete"
	case "review_changes_requested":
		if detail != "" {
			body = "review: changes requested — " + detail
		} else {
			body = "review: changes requested"
		}
	case "fixer_complete":
		if detail != "" {
			body = "fixer agent completed — " + detail
		} else {
			body = "fixer agent completed"
		}
	default:
		if detail != "" {
			body = event + " — " + detail
		} else {
			body = event
		}
	}
	return "🤖 kasmos: **" + planName + "** — " + body
}

// postClickUpProgress resolves the ClickUp task ID for the given plan and posts
// a progress comment. Returns a fire-and-forget tea.Cmd. Returns nil if no task
// ID is associated with the plan or the commenter is unavailable. All errors
// are logged, never surfaced to the user.
func (m *home) postClickUpProgress(planFile, event, detail string) tea.Cmd {
	if m.taskState == nil {
		return nil
	}
	entry, ok := m.taskState.Entry(planFile)
	if !ok {
		return nil
	}

	// Fetch content for fallback task ID resolution only when the field is empty.
	var content string
	if entry.ClickUpTaskID == "" && m.taskStore != nil {
		content, _ = m.taskStore.GetContent(m.taskStoreProject, planFile)
	}
	taskID := resolveClickUpTaskID(entry, content)

	planName := taskstate.DisplayName(planFile)
	comment := buildClickUpProgressComment(event, planName, detail)

	commenter := m.getOrCreateCommenter(m.ctx)
	return postClickUpProgress(commenter, taskID, comment)
}

// getOrCreateCommenter returns a Commenter backed by the same MCP client as
// the Importer if it already exists. Returns nil when no MCP client has been
// initialized yet — progress comments are best-effort and the importer is
// always initialized before plans acquire ClickUp task IDs, so this fallback
// path is never hit in practice. Lazy initialization via getOrCreateImporter
// is deliberately avoided: that call does blocking I/O (MCP subprocess spawn)
// and must not run inside the synchronous Update() path.
func (m *home) getOrCreateCommenter(_ context.Context) *clickup.Commenter {
	if m.clickUpCommenter != nil {
		return m.clickUpCommenter
	}
	if m.clickUpConfig == nil || m.clickUpMCPClient == nil {
		return nil
	}

	// Reuse the shared MCP client initialized by the importer.
	m.clickUpCommenter = clickup.NewCommenter(m.clickUpMCPClient)
	if projCfg := clickup.LoadProjectConfig(m.activeRepoPath); projCfg.WorkspaceID != "" {
		m.clickUpCommenter.SetWorkspaceID(projCfg.WorkspaceID)
	}
	return m.clickUpCommenter
}

// coalesceRestarts merges adjacent session_started + session_stopped pairs
// (in newest-first order) that share the same HH:MM into a single "restarted"
// event. The slice is newest-first, so started appears before stopped.
func coalesceRestarts(displays []ui.AuditEventDisplay) []ui.AuditEventDisplay {
	if len(displays) < 2 {
		return displays
	}
	out := make([]ui.AuditEventDisplay, 0, len(displays))
	i := 0
	for i < len(displays) {
		// Newest-first: started at [i], stopped at [i+1].
		if i+1 < len(displays) &&
			displays[i].Kind == "session_started" &&
			displays[i+1].Kind == "session_stopped" &&
			displays[i].Time == displays[i+1].Time {
			icon, color := ui.EventKindIcon("session_started")
			out = append(out, ui.AuditEventDisplay{
				Time:    displays[i].Time,
				Kind:    "session_restarted",
				Icon:    icon,
				Message: "kasmos restarted",
				Color:   color,
				Level:   "info",
			})
			i += 2
			continue
		}
		out = append(out, displays[i])
		i++
	}
	return out
}
