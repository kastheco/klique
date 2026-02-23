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
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/internal/initcmd/scaffold"
	"github.com/kastheco/klique/keys"
	"github.com/kastheco/klique/log"
	"github.com/kastheco/klique/session"
	gitpkg "github.com/kastheco/klique/session/git"
	"github.com/kastheco/klique/ui"

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

// setFocus updates which panel has focus and syncs the focused state to sidebar and list.
// 0 = sidebar (left), 1 = preview/center, 2 = instance list (right).
func (m *home) setFocus(panel int) {
	m.focusedPanel = panel
	m.sidebar.SetFocused(panel == 0)
	m.tabbedWindow.SetFocused(panel == 1)
	m.list.SetFocused(panel == 2)
}

// enterFocusMode enters focus/insert mode and starts the fast preview ticker.
// enterFocusMode directly attaches to the selected instance's tmux session.
// This takes over the terminal for native performance. Ctrl+Q detaches.
// enterFocusMode creates an embedded terminal emulator connected to the instance's
// PTY and starts the 30fps render ticker. Input goes directly to the PTY (zero latency),
// display is rendered from the emulator's screen buffer (no subprocess calls).
func (m *home) enterFocusMode() tea.Cmd {
	m.tabbedWindow.ClearDocumentMode()
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

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

	m.embeddedTerminal = term
	m.state = stateFocusAgent
	m.tabbedWindow.SetFocusMode(true)

	// Start the 30fps render ticker
	return func() tea.Msg {
		return focusPreviewTickMsg{}
	}
}

// enterGitFocusMode enters focus mode for the git tab (lazygit).
// Spawns lazygit if it's not already running.
func (m *home) enterGitFocusMode() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil || !selected.Started() || selected.Paused() {
		return nil
	}

	gitPane := m.tabbedWindow.GetGitPane()
	if !gitPane.IsRunning() {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m.handleError(err)
		}
		gitPane.Spawn(worktree.GetWorktreePath(), selected.Title)
	}

	m.state = stateFocusAgent
	m.tabbedWindow.SetFocusMode(true)

	return func() tea.Msg {
		return gitTabTickMsg{}
	}
}

// exitFocusMode shuts down the embedded terminal and resets state.
func (m *home) exitFocusMode() {
	if m.embeddedTerminal != nil {
		m.embeddedTerminal.Close()
		m.embeddedTerminal = nil
	}
	m.state = stateDefault
	m.tabbedWindow.SetFocusMode(false)
}

// fkeyToTab maps F1/F2/F3 key strings to tab indices.
func fkeyToTab(key string) (int, bool) {
	switch key {
	case "f1":
		return ui.PreviewTab, true
	case "f2":
		return ui.DiffTab, true
	case "f3":
		return ui.GitTab, true
	default:
		return 0, false
	}
}

// switchToTab switches to the specified tab, handling git tab spawn/kill lifecycle.
func (m *home) switchToTab(name keys.KeyName) (tea.Model, tea.Cmd) {
	var targetTab int
	switch name {
	case keys.KeyTabAgent:
		targetTab = ui.PreviewTab
	case keys.KeyTabDiff:
		targetTab = ui.DiffTab
	case keys.KeyTabGit:
		targetTab = ui.GitTab
	default:
		return m, nil
	}

	if m.tabbedWindow.GetActiveTab() == targetTab {
		return m, nil
	}

	wasGitTab := m.tabbedWindow.IsInGitTab()
	m.tabbedWindow.SetActiveTab(targetTab)
	m.menu.SetInDiffTab(targetTab == ui.DiffTab)

	if wasGitTab && targetTab != ui.GitTab {
		m.killGitTab()
	}
	if targetTab == ui.GitTab {
		cmd := m.spawnGitTab()
		return m, tea.Batch(m.instanceChanged(), cmd)
	}
	return m, m.instanceChanged()
}

// filterInstancesByTopic updates the instance list highlight filter based on the
// current sidebar selection. In tree mode, this highlights matching instances and
// boosts them to the top. In flat mode, it falls back to the existing SetFilter behavior.
func (m *home) filterInstancesByTopic() {
	selectedID := m.sidebar.GetSelectedID()

	if !m.sidebar.IsTreeMode() {
		// Flat mode fallback
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

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	// Clear notification when user selects this instance — they've seen it
	if selected != nil && selected.Notified {
		selected.Notified = false
		m.updateSidebarItems()
	}

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	// Preview errors (e.g. dead tmux pane) are transient infrastructure failures —
	// log them silently rather than spamming the user with toast notifications.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		log.ErrorLog.Printf("preview update error: %v", err)
	}

	// Respawn lazygit if the selected instance changed while on the git tab
	if m.tabbedWindow.IsInGitTab() {
		gitPane := m.tabbedWindow.GetGitPane()
		title := ""
		if selected != nil {
			title = selected.Title
		}
		if gitPane.NeedsRespawn(title) {
			return m.spawnGitTab()
		}
	}

	return nil
}

// spawnGitTab spawns lazygit for the selected instance and starts the render ticker.
func (m *home) spawnGitTab() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil || !selected.Started() || selected.Paused() {
		return nil
	}

	worktree, err := selected.GetGitWorktree()
	if err != nil {
		return m.handleError(err)
	}

	gitPane := m.tabbedWindow.GetGitPane()
	gitPane.Spawn(worktree.GetWorktreePath(), selected.Title)

	return func() tea.Msg {
		return gitTabTickMsg{}
	}
}

// killGitTab kills the lazygit subprocess.
func (m *home) killGitTab() {
	m.tabbedWindow.GetGitPane().Kill()
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
			if p.Status == planstate.StatusDone || p.Status == planstate.StatusCompleted || p.Status == planstate.StatusCancelled {
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
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == "" || inst.IsReviewer {
			continue
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
	planFile := coderInst.PlanFile

	// Guard: update in-memory state before next tick re-reads disk, preventing double-spawn.
	if err := m.planState.SetStatus(planFile, planstate.StatusReviewing); err != nil {
		log.WarningLog.Printf("could not set plan %q to reviewing: %v", planFile, err)
	}

	// Auto-pause the coder instance — its work is done.
	coderInst.ImplementationComplete = true
	if err := coderInst.Pause(); err != nil {
		log.WarningLog.Printf("could not pause coder instance for %q: %v", planFile, err)
	}

	planName := planstate.DisplayName(planFile)
	planPath := "docs/plans/" + planFile
	prompt := scaffold.LoadReviewPrompt(planPath, planName)

	// Use the reviewer profile if configured, otherwise fall back to default program.
	reviewProfile := m.appConfig.ResolveProfile("spec_review", m.program)
	reviewProgram := reviewProfile.BuildCommand()

	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:    planName + "-review",
		Path:     m.activeRepoPath,
		Program:  reviewProgram,
		PlanFile: planFile,
	})
	if err != nil {
		log.WarningLog.Printf("could not create reviewer instance for %q: %v", planFile, err)
		return nil
	}
	reviewerInst.IsReviewer = true
	reviewerInst.QueuedPrompt = prompt

	m.newInstanceFinalizer = m.list.AddInstance(reviewerInst)
	m.list.SelectInstance(reviewerInst) // sort-order safe, unlike index arithmetic

	m.toastManager.Success(fmt.Sprintf("Implementation complete → review started for %s", planName))

	return func() tea.Msg {
		err := reviewerInst.Start(true)
		return instanceStartedMsg{instance: reviewerInst, err: err}
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
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by klique lifecycle flow\n- Plan file: %s\n", name, description, filename)
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
	if entry.Status != planstate.StatusImplementing {
		return false
	}
	return !tmuxAlive
}

// promptPushBranchThenAdvance shows a confirmation overlay asking the user to
// push the implementation branch, then advances the plan to reviewing.
func (m *home) promptPushBranchThenAdvance(inst *session.Instance) tea.Cmd {
	message := fmt.Sprintf("[!] Implementation finished for '%s'. Push branch now?", planstate.DisplayName(inst.PlanFile))
	pushAction := func() tea.Msg {
		worktree, err := inst.GetGitWorktree()
		if err == nil {
			// Push errors are non-fatal: the user can push manually later.
			_ = worktree.PushChanges(
				fmt.Sprintf("[klique] push completed implementation for '%s'", inst.Title),
				false,
			)
		}
		if err := m.planState.SetStatus(inst.PlanFile, planstate.StatusReviewing); err != nil {
			return err
		}
		return planRefreshMsg{}
	}
	return m.confirmAction(message, func() tea.Msg { return pushAction() })
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

// buildModifyPlanPrompt returns the prompt for modifying an existing plan.
func buildModifyPlanPrompt(planFile string) string {
	return fmt.Sprintf("Modify existing plan at docs/plans/%s. Keep the same filename and update only what changed.", planFile)
}

// agentTypeForSubItem maps a sidebar stage name to the corresponding AgentType constant.
func agentTypeForSubItem(action string) (string, bool) {
	switch action {
	case "plan":
		return session.AgentTypePlanner, true
	case "implement":
		return session.AgentTypeCoder, true
	case "review":
		return session.AgentTypeReviewer, true
	default:
		return "", false
	}
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
	inst.QueuedPrompt = prompt

	var startCmd tea.Cmd
	if action == "plan" {
		// Planner runs on main branch (no shared worktree needed)
		startCmd = func() tea.Msg {
			err := inst.Start(true)
			return instanceStartedMsg{instance: inst, err: err}
		}
	} else {
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

	m.newInstanceFinalizer = m.list.AddInstance(inst)
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
