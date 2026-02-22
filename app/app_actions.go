package app

import (
	"github.com/kastheco/klique/session"
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

	case "view_plan":
		return m.viewSelectedPlan()
	}

	return m, nil
}

// openContextMenu builds a context menu for the currently focused/selected item
// (sidebar topic/plan or instance) and positions it next to the selected item.
func (m *home) openContextMenu() (tea.Model, tea.Cmd) {
	if m.focusedPanel == 0 {
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
		{Label: "View plan", Action: "view_plan"},
		{Label: "Push branch", Action: "push_plan_branch"},
		{Label: "Create PR", Action: "create_plan_pr"},
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
