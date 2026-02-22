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
	"github.com/kastheco/klique/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *home) updateSidebarItems() {
	countByPlan := make(map[string]int)
	groupStatuses := make(map[string]ui.GroupStatus)
	ungroupedCount := 0

	for _, inst := range m.list.GetInstances() {
		planKey := inst.PlanFile // "" => ungrouped
		if planKey == "" {
			ungroupedCount++
		} else {
			countByPlan[planKey]++
		}

		st := groupStatuses[planKey]
		if inst.Started() && !inst.Paused() && !inst.PromptDetected {
			st.HasRunning = true
		}
		if inst.Notified {
			st.HasNotification = true
		}
		groupStatuses[planKey] = st
	}

	m.sidebar.SetItems(countByPlan, ungroupedCount, groupStatuses)
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

	// Build cancelled plans
	cancelledInfos := m.planState.Cancelled()
	cancelled := make([]ui.PlanDisplay, 0, len(cancelledInfos))
	for _, p := range cancelledInfos {
		cancelled = append(cancelled, ui.PlanDisplay{
			Filename:    p.Filename,
			Status:      string(p.Status),
			Description: p.Description,
			Topic:       p.Topic,
		})
	}

	// Feed flat-mode plan list (active plans + cancelled for rendering)
	allVisiblePlans := make([]ui.PlanDisplay, 0, len(ungrouped)+len(cancelled))
	allVisiblePlans = append(allVisiblePlans, ungrouped...)
	for _, t := range topics {
		allVisiblePlans = append(allVisiblePlans, t.Plans...)
	}
	allVisiblePlans = append(allVisiblePlans, cancelled...)
	m.sidebar.SetPlans(allVisiblePlans)

	m.sidebar.SetTopicsAndPlans(topics, ungrouped, history, cancelled)

	// NOTE: Tree mode navigation is disabled until String() is updated to
	// render s.rows. Currently String() renders s.items (flat list) while
	// tree mode makes Up()/Down() navigate s.rows, causing an index mismatch
	// where the visual highlight doesn't match the logical selection.
	m.sidebar.DisableTreeMode()
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
	m.list.SelectInstance(inst) // sort-order safe

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
