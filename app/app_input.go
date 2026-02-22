package app

import (
	"fmt"
	"github.com/kastheco/klique/keys"
	"github.com/kastheco/klique/log"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
	"github.com/kastheco/klique/ui/overlay"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm || m.state == stateNewPlanName || m.state == stateNewPlanDescription || m.state == stateNewPlanTopic || m.state == stateSearch || m.state == stateContextMenu || m.state == statePRTitle || m.state == statePRBody || m.state == stateRenameInstance || m.state == stateSendPrompt || m.state == stateFocusAgent || m.state == stateRepoSwitch || m.state == stateMoveTo {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp || name == keys.KeyMoveTo {
		return nil, false
	}

	// Skip the menu highlighting if the key is not in the map or we are using the shift up and down keys.
	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

// handleMouse processes mouse events for click and scroll interactions.
func (m *home) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Track hover state for the repo button on any mouse event
	repoHovered := zone.Get(ui.ZoneRepoSwitch).InBounds(msg)
	m.sidebar.SetRepoHovered(repoHovered)

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// Handle scroll wheel — always scrolls content (never navigates files)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		selected := m.list.GetSelectedInstance()
		if selected != nil && selected.Status != session.Paused {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.tabbedWindow.ContentScrollUp()
			case tea.MouseButtonWheelDown:
				m.tabbedWindow.ContentScrollDown()
			}
		}
		return m, nil
	}

	// Dismiss overlays on click-outside
	if m.state == stateContextMenu && msg.Button == tea.MouseButtonLeft {
		m.contextMenu = nil
		m.state = stateDefault
		return m, nil
	}
	if m.state == stateRepoSwitch && msg.Button == tea.MouseButtonLeft {
		m.pickerOverlay = nil
		m.state = stateDefault
		return m, nil
	}
	if m.state != stateDefault {
		return m, nil
	}

	x, y := msg.X, msg.Y

	// Account for PaddingTop(1) on columns
	contentY := y - 1

	// Right-click: show context menu
	if msg.Button == tea.MouseButtonRight {
		return m.handleRightClick(x, y, contentY)
	}

	// Only handle left clicks from here
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Zone-based click: repo switch button (works regardless of sidebar layout)
	if zone.Get(ui.ZoneRepoSwitch).InBounds(msg) {
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	}

	// Determine which column was clicked
	if x < m.sidebarWidth {
		// Click in sidebar
		m.setFocus(0)

		// Search bar is at rows 0-2 in the sidebar content (border takes 3 rows)
		if contentY >= 0 && contentY <= 2 {
			m.sidebar.ActivateSearch()
			m.state = stateSearch
			return m, nil
		}

		// Sidebar items start after search bar (row 0) + border (2 rows) + blank line (1 row) = row 4
		itemRow := contentY - 4
		if itemRow >= 0 {
			m.tabbedWindow.ClearDocumentMode()
			m.sidebar.ClickItem(itemRow)
			m.filterInstancesByTopic()
			return m, m.instanceChanged()
		}
	} else if x < m.sidebarWidth+m.tabsWidth {
		// Click in preview/diff area (center column)
		m.setFocus(1)
		localX := x - m.sidebarWidth
		if m.tabbedWindow.HandleTabClick(localX, contentY) {
			m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
			return m, m.instanceChanged()
		}
	} else {
		// Click in instance list (right column)
		m.setFocus(2)

		localX := x - m.sidebarWidth - m.tabsWidth
		// Check if clicking on filter tabs
		if filter, ok := m.list.HandleTabClick(localX, contentY); ok {
			m.list.SetStatusFilter(filter)
			return m, m.instanceChanged()
		}

		// Instance list items start after the header (blank lines + tabs + blank lines)
		listY := contentY - 4
		if listY >= 0 {
			itemIdx := m.list.GetItemAtRow(listY)
			if itemIdx >= 0 {
				m.tabbedWindow.ClearDocumentMode()
				m.list.SetSelectedInstance(itemIdx)
				return m, m.instanceChanged()
			}
		}
	}

	return m, nil
}

// handleRightClick builds and shows a context menu based on what was right-clicked.
func (m *home) handleRightClick(x, y, contentY int) (tea.Model, tea.Cmd) {
	if x < m.sidebarWidth {
		// Right-click in sidebar
		itemRow := contentY - 4
		if itemRow >= 0 {
			m.sidebar.ClickItem(itemRow)
			m.filterInstancesByTopic()
		}
		// Plan header: show plan context menu
		if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
			return m.openPlanContextMenu()
		}
		// Topic header: show topic context menu
		if m.sidebar.IsSelectedTopicHeader() {
			return m.openTopicContextMenu()
		}
		return m, nil
	} else if x >= m.sidebarWidth+m.tabsWidth {
		// Right-click in instance list (right column) — select the item first
		listY := contentY - 4
		if listY >= 0 {
			itemIdx := m.list.GetItemAtRow(listY)
			if itemIdx >= 0 {
				m.list.SetSelectedInstance(itemIdx)
			}
		}
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
		m.contextMenu = overlay.NewContextMenu(x, y, items)
		m.state = stateContextMenu
		return m, nil
	}
	return m, nil
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateContextMenu {
		if m.contextMenu == nil {
			m.state = stateDefault
			return m, nil
		}
		action, closed := m.contextMenu.HandleKeyPress(msg)
		if closed {
			m.contextMenu = nil
			m.state = stateDefault
			if action != "" {
				return m.executeContextAction(action)
			}
			return m, nil
		}
		return m, nil
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.newInstance = nil
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.newInstance
		if instance == nil {
			// stateNew without a pending instance — shouldn't happen, return to default
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, nil
		}
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Set loading status and transition to default state immediately
			instance.SetStatus(session.Loading)
			m.state = stateDefault
			m.newInstance = nil
			m.menu.SetState(ui.StateDefault)

			// Handle prompt-after-name flow
			if m.promptAfterName {
				m.state = statePrompt
				m.menu.SetState(ui.StatePrompt)
				m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt", "")
				m.textInputOverlay.SetSize(50, 5)
				m.promptAfterName = false
			}

			// Start instance asynchronously
			startCmd := func() tea.Msg {
				return instanceStartedMsg{instance: instance, err: instance.Start(true)}
			}

			return m, tea.Batch(tea.WindowSize(), startCmd)
		case tea.KeyRunes:
			if runewidth.StringWidth(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			runes := []rune(instance.Title)
			if len(runes) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(string(runes[:len(runes)-1])); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.newInstance = nil
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if m.textInputOverlay.IsSubmitted() {
				if err := selected.SendPrompt(m.textInputOverlay.GetValue()); err != nil {
					// TODO: we probably end up in a bad state here.
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					m.showHelpScreen(helpStart(selected), nil)
					return nil
				},
			)
		}

		return m, nil
	}

	// Handle PR title input state
	if m.state == statePRTitle {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				prTitle := m.textInputOverlay.GetValue()
				selected := m.list.GetSelectedInstance()
				if selected != nil && prTitle != "" {
					m.pendingPRTitle = prTitle
					m.textInputOverlay = nil

					// Generate a PR body from git data
					generatedBody := ""
					worktree, err := selected.GetGitWorktree()
					if err == nil {
						body, genErr := worktree.GeneratePRBody()
						if genErr == nil {
							generatedBody = body
						}
					}

					// Transition to PR body editing state
					m.state = statePRBody
					m.textInputOverlay = overlay.NewTextInputOverlay("PR description (edit or submit)", generatedBody)
					m.textInputOverlay.SetSize(80, 20)
					return m, nil
				}
			}
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle PR body input state
	if m.state == statePRBody {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				prBody := m.textInputOverlay.GetValue()
				prTitle := m.pendingPRTitle
				selected := m.list.GetSelectedInstance()
				if selected != nil && prTitle != "" {
					m.textInputOverlay = nil
					m.pendingPRTitle = ""
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.pendingPRToastID = m.toastManager.Loading("Creating PR...")
					prToastID := m.pendingPRToastID
					return m, tea.Batch(tea.WindowSize(), func() tea.Msg {
						commitMsg := fmt.Sprintf("[klique] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
						worktree, err := selected.GetGitWorktree()
						if err != nil {
							return prErrorMsg{id: prToastID, err: err}
						}
						if err := worktree.CreatePR(prTitle, prBody, commitMsg); err != nil {
							return prErrorMsg{id: prToastID, err: err}
						}
						return prCreatedMsg{}
					}, m.toastTickCmd())
				}
			}
			m.textInputOverlay = nil
			m.pendingPRTitle = ""
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle instance rename state
	if m.state == stateRenameInstance {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				newName := m.textInputOverlay.GetValue()
				selected := m.list.GetSelectedInstance()
				if selected != nil && newName != "" {
					selected.Title = newName
					m.saveAllInstances()
				}
			}
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle focus mode — forward keys directly to the agent's or lazygit's PTY
	if m.state == stateFocusAgent {
		// Ctrl+Space exits focus mode
		if msg.Type == tea.KeyCtrlAt {
			m.exitFocusMode()
			return m, tea.WindowSize()
		}

		// F1/F2/F3: exit focus mode and switch to specific tab
		if targetTab, ok := fkeyToTab(msg.String()); ok {
			wasGitTab := m.tabbedWindow.IsInGitTab()
			m.exitFocusMode()
			m.tabbedWindow.SetActiveTab(targetTab)
			m.menu.SetInDiffTab(targetTab == ui.DiffTab)
			if wasGitTab && targetTab != ui.GitTab {
				m.killGitTab()
			}
			if targetTab == ui.GitTab && !wasGitTab {
				cmd := m.spawnGitTab()
				return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), cmd)
			}
			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		}

		// Git tab focus: forward to lazygit
		if m.tabbedWindow.IsInGitTab() {
			gitPane := m.tabbedWindow.GetGitPane()
			if gitPane == nil || !gitPane.IsRunning() {
				m.exitFocusMode()
				return m, nil
			}
			data := keyToBytes(msg)
			if data == nil {
				return m, nil
			}
			if err := gitPane.SendKey(data); err != nil {
				m.exitFocusMode()
				return m, m.handleError(err)
			}
			return m, nil
		}

		// Preview tab focus: forward to embedded terminal
		if m.embeddedTerminal == nil {
			m.exitFocusMode()
			return m, nil
		}
		data := keyToBytes(msg)
		if data == nil {
			return m, nil
		}
		if err := m.embeddedTerminal.SendKey(data); err != nil {
			return m, m.handleError(err)
		}
		return m, nil
	}

	// Handle send prompt state
	if m.state == stateSendPrompt {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				value := m.textInputOverlay.GetValue()
				selected := m.list.GetSelectedInstance()
				if selected != nil && value != "" {
					if err := selected.SendPrompt(value); err != nil {
						m.textInputOverlay = nil
						m.state = stateDefault
						m.menu.SetState(ui.StateDefault)
						return m, m.handleError(err)
					}
					selected.SetStatus(session.Running)
				}
			}
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		switch msg.String() {
		case m.confirmationOverlay.ConfirmKey:
			action := m.pendingConfirmAction
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			// Return the action as a tea.Cmd so bubbletea runs it asynchronously.
			// This prevents blocking the UI during I/O (git push, etc.).
			return m, action
		case m.confirmationOverlay.CancelKey, "esc":
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			return m, nil
		default:
			return m, nil
		}
	}

	// Handle new plan name state
	if m.state == stateNewPlanName {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				m.pendingPlanName = m.textInputOverlay.GetValue()
				if m.pendingPlanName == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.textInputOverlay = nil
					return m, m.handleError(fmt.Errorf("plan name cannot be empty"))
				}
				m.textInputOverlay = overlay.NewTextInputOverlay("Plan description (optional)", "")
				m.textInputOverlay.SetSize(60, 3)
				m.state = stateNewPlanDescription
				return m, nil
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pendingPlanName = ""
			m.textInputOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle new plan description state
	if m.state == stateNewPlanDescription {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				m.pendingPlanDesc = m.textInputOverlay.GetValue()
			}
			m.textInputOverlay = nil
			// Show topic picker
			topicNames := m.getTopicNames()
			topicNames = append([]string{"(No topic)"}, topicNames...)
			m.pickerOverlay = overlay.NewPickerOverlay("Assign to topic (optional)", topicNames)
			m.state = stateNewPlanTopic
			return m, nil
		}
		return m, nil
	}

	// Handle new plan topic picker state
	if m.state == stateNewPlanTopic {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.pickerOverlay.HandleKeyPress(msg)
		if shouldClose {
			topic := ""
			if m.pickerOverlay.IsSubmitted() {
				picked := m.pickerOverlay.Value()
				if picked != "(No topic)" {
					topic = picked
				}
			}
			if m.pendingPlanName != "" {
				if err := m.createPlanEntry(m.pendingPlanName, m.pendingPlanDesc, topic); err != nil {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.pickerOverlay = nil
					m.pendingPlanName = ""
					m.pendingPlanDesc = ""
					return m, m.handleError(err)
				}
				m.loadPlanState()
				m.updateSidebarPlans()
				m.updateSidebarItems()
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pickerOverlay = nil
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle move-to-plan state (assign instance to plan)
	if m.state == stateMoveTo {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.pickerOverlay.HandleKeyPress(msg)
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			if selected != nil && m.pickerOverlay.IsSubmitted() {
				picked := m.pickerOverlay.Value()
				selected.PlanFile = m.planPickerMap[picked]
				m.updateSidebarItems()
				if err := m.saveAllInstances(); err != nil {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.pickerOverlay = nil
					return m, m.handleError(err)
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pickerOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle repo switch state (picker overlay)
	if m.state == stateRepoSwitch {
		shouldClose := m.pickerOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.pickerOverlay.IsSubmitted() {
				selected := m.pickerOverlay.Value()
				if selected != "" {
					if selected == "Open folder..." {
						m.state = stateDefault
						m.menu.SetState(ui.StateDefault)
						m.pickerOverlay = nil
						return m, m.openFolderPicker()
					}
					m.switchToRepo(selected)
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pickerOverlay = nil
		}
		return m, nil
	}

	// Handle search state — allows typing to filter AND arrow keys to navigate topics
	if m.state == stateSearch {
		switch {
		case msg.String() == "esc":
			m.sidebar.DeactivateSearch()
			m.sidebar.UpdateMatchCounts(nil, 0)
			m.state = stateDefault
			m.filterInstancesByTopic()
			return m, nil
		case msg.String() == "enter":
			m.sidebar.DeactivateSearch()
			m.sidebar.UpdateMatchCounts(nil, 0)
			m.state = stateDefault
			return m, nil
		case msg.String() == "up":
			m.sidebar.Up()
			m.filterSearchWithTopic()
			return m, m.instanceChanged()
		case msg.String() == "down":
			m.sidebar.Down()
			m.filterSearchWithTopic()
			return m, m.instanceChanged()
		case msg.Type == tea.KeyBackspace:
			q := m.sidebar.GetSearchQuery()
			if len(q) > 0 {
				runes := []rune(q)
				m.sidebar.SetSearchQuery(string(runes[:len(runes)-1]))
			}
			m.filterBySearch()
			return m, nil
		case msg.Type == tea.KeySpace:
			m.sidebar.SetSearchQuery(m.sidebar.GetSearchQuery() + " ")
			m.filterBySearch()
			return m, nil
		case msg.Type == tea.KeyRunes:
			m.sidebar.SetSearchQuery(m.sidebar.GetSearchQuery() + string(msg.Runes))
			m.filterBySearch()
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// Exit document mode (plan viewer) on Esc
		if m.tabbedWindow.IsDocumentMode() {
			m.tabbedWindow.ClearDocumentMode()
			return m, m.instanceChanged()
		}
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt:
		if m.list.TotalInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    m.activeRepoPath,
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNew:
		if m.list.TotalInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    m.activeRepoPath,
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyNewSkipPermissions:
		if m.list.TotalInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:           "",
			Path:            m.activeRepoPath,
			Program:         m.program,
			SkipPermissions: true,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.tabbedWindow.ClearDocumentMode()
		if m.focusedPanel == 0 {
			m.sidebar.Up()
			m.filterInstancesByTopic()
		} else {
			m.list.Up()
		}
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.tabbedWindow.ClearDocumentMode()
		if m.focusedPanel == 0 {
			m.sidebar.Down()
			m.filterInstancesByTopic()
		} else {
			m.list.Down()
		}
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, m.instanceChanged()
	case keys.KeyTab:
		wasGitTab := m.tabbedWindow.IsInGitTab()
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		// Kill lazygit when leaving git tab
		if wasGitTab && !m.tabbedWindow.IsInGitTab() {
			m.killGitTab()
		}
		// Spawn lazygit when entering git tab
		if m.tabbedWindow.IsInGitTab() {
			cmd := m.spawnGitTab()
			return m, tea.Batch(m.instanceChanged(), cmd)
		}
		return m, m.instanceChanged()
	case keys.KeyFilterAll:
		m.list.SetStatusFilter(ui.StatusFilterAll)
		return m, m.instanceChanged()
	case keys.KeyFilterActive:
		m.list.SetStatusFilter(ui.StatusFilterActive)
		return m, m.instanceChanged()
	case keys.KeyCycleSort:
		m.list.CycleSortMode()
		return m, m.instanceChanged()
	case keys.KeySpace:
		if m.focusedPanel == 0 && m.sidebar.ToggleSelectedExpand() {
			return m, nil
		}
		return m.openContextMenu()
	case keys.KeyGitTab:
		// Jump directly to git tab
		if m.tabbedWindow.IsInGitTab() {
			return m, nil
		}
		m.tabbedWindow.SetActiveTab(ui.GitTab)
		m.menu.SetInDiffTab(false)
		cmd := m.spawnGitTab()
		return m, tea.Batch(m.instanceChanged(), cmd)
	case keys.KeyTabAgent, keys.KeyTabDiff, keys.KeyTabGit:
		return m.switchToTab(name)
	case keys.KeySendPrompt:
		if m.tabbedWindow.IsInGitTab() {
			return m, m.enterGitFocusMode()
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		return m, m.enterFocusMode()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Pre-kill checks run async; model mutations happen in Update via killInstanceMsg.
		title := selected.Title
		killAction := func() tea.Msg {
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}
			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", selected.Title)
			}
			return killInstanceMsg{title: title}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[klique] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCreatePR:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("PR title", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout{}, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		// If the sidebar is focused, handle tree-mode interactions.
		if m.focusedPanel == 0 {
			// Stage row: trigger the stage action
			if planFile, stage, ok := m.sidebar.GetSelectedPlanStage(); ok {
				return m.triggerPlanStage(planFile, stage)
			}
			// Plan header: open plan context menu
			if m.sidebar.IsSelectedPlanHeader() {
				return m.openPlanContextMenu()
			}
			// Topic header: open topic context menu
			if m.sidebar.IsSelectedTopicHeader() {
				return m.openTopicContextMenu()
			}
			// Plan file selected: show context menu with start/view/push/pr options
			if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
				return m.openPlanContextMenu()
			}
		}
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			ch, err := m.list.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	case keys.KeyFocusSidebar:
		if m.sidebarHidden {
			// Show and focus in one motion
			m.sidebarHidden = false
			m.setFocus(0)
			return m, tea.WindowSize()
		}
		// s key always jumps directly to the sidebar regardless of current panel.
		m.setFocus(0)
		return m, nil
	case keys.KeyViewPlan:
		return m.viewSelectedPlan()
	case keys.KeyToggleSidebar:
		if m.sidebarHidden {
			// Show sidebar, keep current focus
			m.sidebarHidden = false
		} else {
			// Hide sidebar
			m.sidebarHidden = true
			// If sidebar was focused, move focus to tabbed view
			if m.focusedPanel == 0 {
				m.setFocus(1)
			}
		}
		return m, tea.WindowSize()
	case keys.KeyLeft:
		if m.focusedPanel == 0 {
			// Already on sidebar: hide it and move focus to center
			if !m.sidebar.IsSearchActive() {
				m.sidebarHidden = true
				m.setFocus(1)
				return m, tea.WindowSize()
			}
			return m, nil
		}
		if m.focusedPanel == 1 && m.sidebarHidden {
			// Show sidebar and focus it in one motion
			m.sidebarHidden = false
			m.setFocus(0)
			return m, tea.WindowSize()
		}
		// Normal cycle left: list(2) → preview(1) → sidebar(0)
		if m.focusedPanel > 0 {
			m.setFocus(m.focusedPanel - 1)
		}
		return m, nil
	case keys.KeyRight:
		// Cycle right: sidebar(0) → preview(1) → list(2) → enter focus mode.
		if m.focusedPanel == 2 {
			// Already on instance list → enter focus mode on the active tab's pane
			if m.tabbedWindow.IsInGitTab() {
				return m, m.enterGitFocusMode()
			}
			selected := m.list.GetSelectedInstance()
			if selected != nil && selected.Started() && !selected.Paused() {
				return m, m.enterFocusMode()
			}
		} else {
			m.setFocus(m.focusedPanel + 1)
		}
		return m, nil
	case keys.KeyNewPlan:
		m.state = stateNewPlanName
		m.textInputOverlay = overlay.NewTextInputOverlay("Plan name", "")
		m.textInputOverlay.SetSize(60, 3)
		return m, nil
	case keys.KeyRepoSwitch:
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	case keys.KeySearch:
		m.sidebar.ActivateSearch()
		m.sidebar.SelectFirst() // Reset to "All" when starting search
		m.state = stateSearch
		m.setFocus(0)
		m.list.SetFilter("") // Show all instances
		return m, nil
	case keys.KeyMoveTo:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = stateMoveTo
		items, mapping := m.getAssignablePlanNames()
		m.planPickerMap = mapping
		m.pickerOverlay = overlay.NewPickerOverlay("Assign to plan", items)
		return m, nil
	default:
		return m, nil
	}
}

// keyToBytes translates a Bubble Tea key message to raw bytes for PTY forwarding.
func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyEnter:
		return []byte{0x0D}
	case tea.KeyBackspace:
		return []byte{0x7F}
	case tea.KeyTab:
		return []byte{0x09}
	case tea.KeySpace:
		return []byte{0x20}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyCtrlC:
		return []byte{0x03}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyCtrlA:
		return []byte{0x01}
	case tea.KeyCtrlE:
		return []byte{0x05}
	case tea.KeyCtrlL:
		return []byte{0x0C}
	case tea.KeyCtrlU:
		return []byte{0x15}
	case tea.KeyCtrlK:
		return []byte{0x0B}
	case tea.KeyCtrlW:
		return []byte{0x17}
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyShiftTab:
		return []byte("\x1b[Z")
	default:
		// Forward any ctrl+letter key as its raw control character byte.
		// bubbletea KeyCtrlA..KeyCtrlZ have sequential values 0x01..0x1A.
		if msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ {
			return []byte{byte(msg.Type)}
		}
		return nil
	}
}

func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.toastManager.Error(err.Error())
	return m.toastTickCmd()
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm.
// The action is a tea.Cmd that will be returned from Update() to run asynchronously —
// never called synchronously, which would block the UI during I/O operations.
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.pendingConfirmAction = action

	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	m.confirmationOverlay.SetWidth(50)

	return nil
}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}
