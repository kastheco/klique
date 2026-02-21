package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/internal/initcmd/scaffold"
	"github.com/kastheco/klique/keys"
	"github.com/kastheco/klique/log"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *home) updateSidebarItems() {
	topicNames := make([]string, len(m.topics))
	countByTopic := make(map[string]int)
	sharedTopics := make(map[string]bool)
	topicStatuses := make(map[string]ui.TopicStatus)
	ungroupedCount := 0

	for i, t := range m.topics {
		topicNames[i] = t.Name
		if t.SharedWorktree {
			sharedTopics[t.Name] = true
		}
	}

	for _, inst := range m.list.GetInstances() {
		if inst.TopicName == "" {
			ungroupedCount++
		} else {
			countByTopic[inst.TopicName]++
		}

		// Track running and notification status per topic key.
		// An instance is "active" if it's started, not paused, and hasn't shown
		// a prompt yet (meaning the program is still working).
		topicKey := inst.TopicName // "" for ungrouped
		st := topicStatuses[topicKey]
		if inst.Started() && !inst.Paused() && !inst.PromptDetected {
			st.HasRunning = true
		}
		if inst.Notified {
			st.HasNotification = true
		}
		topicStatuses[topicKey] = st
	}

	m.sidebar.SetItems(topicNames, countByTopic, ungroupedCount, sharedTopics, topicStatuses)
}

// getMovableTopicNames returns topic names that a non-shared instance can be moved to.
func (m *home) getMovableTopicNames() []string {
	names := []string{"(Ungrouped)"}
	for _, t := range m.topics {
		names = append(names, t.Name)
	}
	return names
}

// setFocus updates which panel has focus and syncs the focused state to sidebar and list.
// 0 = sidebar (left), 1 = preview/center, 2 = instance list (right).
func (m *home) setFocus(panel int) {
	m.focusedPanel = panel
	m.sidebar.SetFocused(panel == 0)
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

func (m *home) filterInstancesByTopic() {
	selectedID := m.sidebar.GetSelectedID()
	switch {
	case selectedID == ui.SidebarAll:
		m.list.SetFilter("")
	case selectedID == ui.SidebarUngrouped:
		m.list.SetFilter(ui.SidebarUngrouped)
	case strings.HasPrefix(selectedID, ui.SidebarPlanPrefix):
		// Plan items: show all instances (no topic filter)
		m.list.SetFilter("")
	default:
		m.list.SetFilter(selectedID)
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

	// Calculate match counts per topic for sidebar dimming
	matchesByTopic := make(map[string]int)
	totalMatches := 0
	for _, inst := range m.list.GetInstances() {
		if strings.Contains(strings.ToLower(inst.Title), query) ||
			strings.Contains(strings.ToLower(inst.TopicName), query) {
			matchesByTopic[inst.TopicName]++
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
	m.topics = m.filterTopicsByRepo(m.allTopics, m.activeRepoPath)
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

// filterTopicsByRepo returns topics that belong to the given repo path.
func (m *home) filterTopicsByRepo(topics []*session.Topic, repoPath string) []*session.Topic {
	var filtered []*session.Topic
	for _, t := range topics {
		if t.Path == repoPath {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// saveAllTopics saves all topics (across all repos) to storage.
func (m *home) saveAllTopics() error {
	return m.storage.SaveTopics(m.allTopics)
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

// updateSidebarPlans pushes the current unfinished plans into the sidebar.
func (m *home) updateSidebarPlans() {
	if m.planState == nil {
		m.sidebar.SetPlans(nil)
		return
	}
	unfinished := m.planState.Unfinished()
	plans := make([]ui.PlanDisplay, 0, len(unfinished))
	for _, p := range unfinished {
		plans = append(plans, ui.PlanDisplay{Filename: p.Filename, Status: string(p.Status)})
	}
	m.sidebar.SetPlans(plans)
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

// checkReviewerCompletion detects reviewer sessions whose tmux pane has died
// (agent exited after completing the review) and marks the plan as done.
func (m *home) checkReviewerCompletion() {
	if m.planState == nil {
		return
	}
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == "" || !inst.IsReviewer || !inst.Started() || inst.Paused() {
			continue
		}
		if inst.TmuxAlive() {
			continue
		}
		// Reviewer's tmux session is gone — mark plan completed (terminal) only if still reviewing.
		// Using StatusCompleted (not StatusDone) breaks the infinite spawn cycle: IsDone()
		// only matches StatusDone, so the coder instance will not trigger another reviewer.
		entry := m.planState.Plans[inst.PlanFile]
		if entry.Status != planstate.StatusReviewing {
			continue
		}
		if err := m.planState.SetStatus(inst.PlanFile, planstate.StatusCompleted); err != nil {
			log.WarningLog.Printf("could not mark plan %q completed: %v", inst.PlanFile, err)
		}
	}
}

// transitionToReview marks a plan as "reviewing", spawns a reviewer session
// pre-loaded with the review prompt, and returns the start cmd.
func (m *home) transitionToReview(coderInst *session.Instance) tea.Cmd {
	planFile := coderInst.PlanFile

	// Guard: update in-memory state before next tick re-reads disk, preventing double-spawn.
	if err := m.planState.SetStatus(planFile, planstate.StatusReviewing); err != nil {
		log.WarningLog.Printf("could not set plan %q to reviewing: %v", planFile, err)
	}

	planName := planstate.DisplayName(planFile)
	planPath := "docs/plans/" + planFile
	prompt := scaffold.LoadReviewPrompt(planPath, planName)

	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:    planName + "-review",
		Path:     m.activeRepoPath,
		Program:  m.program,
		PlanFile: planFile,
	})
	if err != nil {
		log.WarningLog.Printf("could not create reviewer instance for %q: %v", planFile, err)
		return nil
	}
	reviewerInst.IsReviewer = true
	reviewerInst.QueuedPrompt = prompt

	m.newInstanceFinalizer = m.list.AddInstance(reviewerInst)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)

	return func() tea.Msg {
		err := reviewerInst.Start(true)
		return instanceStartedMsg{instance: reviewerInst, err: err}
	}
}

// spawnPlanSession creates a new coder session bound to the given plan file.
// Returns early with an error toast if a session for this plan already exists.
func (m *home) spawnPlanSession(planFile string) (tea.Model, tea.Cmd) {
	if m.list.TotalInstances() >= GlobalInstanceLimit {
		return m, m.handleError(fmt.Errorf("cannot spawn plan session: instance limit reached"))
	}

	// Prevent duplicate sessions for the same plan.
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == planFile {
			return m, m.handleError(fmt.Errorf("plan %q already has an active session", planstate.DisplayName(planFile)))
		}
	}

	planName := planstate.DisplayName(planFile)
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    planName,
		Path:     m.activeRepoPath,
		Program:  m.program,
		PlanFile: planFile,
	})
	if err != nil {
		return m, m.handleError(err)
	}

	inst.QueuedPrompt = fmt.Sprintf(
		"Implement docs/plans/%s using the executing-plans superpowers skill. Execute ALL tasks sequentially without stopping to ask for confirmation between tasks. Do NOT delete the git worktree or feature branch when done — your orchestrator (klique) manages worktree lifecycle and will clean up after you.",
		planFile,
	)

	m.newInstanceFinalizer = m.list.AddInstance(inst)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)

	startCmd := func() tea.Msg {
		err := inst.Start(true)
		return instanceStartedMsg{instance: inst, err: err}
	}
	return m, tea.Batch(tea.WindowSize(), startCmd)
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
			glamour.WithAutoStyle(),
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
