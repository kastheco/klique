package app

import (
	"fmt"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"strings"
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
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm || m.state == stateNewPlan || m.state == stateNewPlanDeriving || m.state == stateNewPlanTopic || m.state == stateSpawnAgent || m.state == stateSearch || m.state == stateContextMenu || m.state == statePRTitle || m.state == statePRBody || m.state == stateRenameInstance || m.state == stateRenamePlan || m.state == stateSendPrompt || m.state == stateFocusAgent || m.state == stateRepoSwitch || m.state == stateChangeTopic || m.state == stateSetStatus || m.state == stateClickUpSearch || m.state == stateClickUpPicker || m.state == stateClickUpFetching || m.state == statePermission || m.state == stateTmuxBrowser || m.state == stateChatAboutPlan {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if m.nav.GetSelectedInstance() != nil && m.nav.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	// (no special-cased keys to skip here)

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
	repoHovered := zone.Get(ui.ZoneNavRepo).InBounds(msg)
	m.nav.SetRepoHovered(repoHovered)

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// Handle scroll wheel — always scrolls content (never navigates files)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		selected := m.nav.GetSelectedInstance()
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
	if m.state == stateTmuxBrowser && msg.Button == tea.MouseButtonLeft {
		m.tmuxBrowser = nil
		m.state = stateDefault
		return m, nil
	}
	if m.state != stateDefault {
		return m, nil
	}

	// Right-click: show context menu
	if msg.Button == tea.MouseButtonRight {
		return m.handleRightClick(msg)
	}

	// Only handle left clicks from here
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Zone-based click: repo switch button
	if zone.Get(ui.ZoneNavRepo).InBounds(msg) {
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	}

	// Zone-based click: search box
	if zone.Get(ui.ZoneNavSearch).InBounds(msg) {
		m.setFocusSlot(slotNav)
		m.nav.ActivateSearch()
		m.state = stateSearch
		return m, nil
	}

	// Zone-based click: tab headers — switch visible tab without stealing sidebar focus.
	for i, zoneID := range ui.TabZoneIDs {
		if zone.Get(zoneID).InBounds(msg) {
			m.tabbedWindow.SetActiveTab(i)
			m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
			return m, nil
		}
	}

	// Zone-based click: "view plan doc" button in info tab
	if zone.Get(ui.ZoneViewPlan).InBounds(msg) {
		return m.viewSelectedPlan()
	}

	// Zone-based click: nav panel rows
	if zone.Get(ui.ZoneNavPanel).InBounds(msg) {
		m.setFocusSlot(slotNav)
		for i := range m.nav.RowCount() {
			if zone.Get(ui.NavRowZoneID(i)).InBounds(msg) {
				m.tabbedWindow.ClearDocumentMode()
				m.nav.ClickItem(i)
				return m, m.instanceChanged()
			}
		}
		return m, nil
	}

	// Click in tabbed window area — sidebar retains focus.
	return m, nil
}

// handleRightClick builds and shows a context menu based on what was right-clicked.
func (m *home) handleRightClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Right-click in nav panel: select the clicked row then show context menu
	if zone.Get(ui.ZoneNavPanel).InBounds(msg) {
		for i := range m.nav.RowCount() {
			if zone.Get(ui.NavRowZoneID(i)).InBounds(msg) {
				m.nav.ClickItem(i)
				break
			}
		}
		if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openPlanContextMenu()
		}
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
			m.nav.Kill()
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
				m.textInputOverlay = overlay.NewTextInputOverlay("enter prompt", "")
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
			m.nav.Kill()
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
			selected := m.nav.GetSelectedInstance()
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
				selected := m.nav.GetSelectedInstance()
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
					m.textInputOverlay = overlay.NewTextInputOverlay("pr description (edit or submit)", generatedBody)
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
				selected := m.nav.GetSelectedInstance()
				if selected != nil && prTitle != "" {
					m.textInputOverlay = nil
					m.pendingPRTitle = ""
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.pendingPRToastID = m.toastManager.Loading("Creating PR...")
					prToastID := m.pendingPRToastID
					return m, tea.Batch(tea.WindowSize(), func() tea.Msg {
						commitMsg := fmt.Sprintf("[kas] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
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
				selected := m.nav.GetSelectedInstance()
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

	// Handle plan rename state
	if m.state == stateRenamePlan {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				newName := strings.TrimSpace(m.textInputOverlay.GetValue())
				planFile := m.nav.GetSelectedPlanFile()
				if planFile != "" && newName != "" && m.planState != nil {
					oldFile := planFile
					newFile, err := m.planState.Rename(oldFile, newName)
					if err != nil {
						m.textInputOverlay = nil
						m.state = stateDefault
						m.menu.SetState(ui.StateDefault)
						return m, m.handleError(err)
					}
					// Update any instances that referenced the old plan file.
					for _, inst := range m.nav.GetInstances() {
						if inst.PlanFile == oldFile {
							inst.PlanFile = newFile
						}
					}
					for _, inst := range m.allInstances {
						if inst.PlanFile == oldFile {
							inst.PlanFile = newFile
						}
					}
					_ = m.saveAllInstances()
					m.updateSidebarPlans()
					m.nav.SelectByID(ui.SidebarPlanPrefix + newFile)
				}
			}
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle chat-about-plan question input
	if m.state == stateChatAboutPlan {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				question := m.textInputOverlay.GetValue()
				planFile := m.pendingChatAboutPlan
				m.textInputOverlay = nil
				m.pendingChatAboutPlan = ""
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				if planFile != "" && question != "" {
					return m.spawnChatAboutPlan(planFile, question)
				}
				return m, tea.WindowSize()
			}
			m.textInputOverlay = nil
			m.pendingChatAboutPlan = ""
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle focus mode — forward keys directly to the agent's PTY
	if m.state == stateFocusAgent {
		// Ctrl+Space exits focus mode
		if msg.Type == tea.KeyCtrlAt {
			m.exitFocusMode()
			return m, tea.WindowSize()
		}

		// !/@ /#: exit focus mode and switch to the corresponding center tab.
		// Sidebar retains focus (slotNav) — only the visible tab changes.
		var jumpTab int
		var doJump bool
		switch msg.String() {
		case "!":
			jumpTab, doJump = ui.PreviewTab, true
		case "@":
			jumpTab, doJump = ui.DiffTab, true
		case "#":
			jumpTab, doJump = ui.InfoTab, true
		}
		if doJump {
			m.exitFocusMode()
			m.tabbedWindow.SetActiveTab(jumpTab)
			return m, tea.WindowSize()
		}

		// Ctrl+Up/Down: cycle through active instances (wrapping) while staying in focus mode
		if msg.Type == tea.KeyCtrlUp || msg.Type == tea.KeyCtrlDown {
			if msg.Type == tea.KeyCtrlUp {
				m.nav.CyclePrevActive()
			} else {
				m.nav.CycleNextActive()
			}
			cmd := m.instanceChanged()
			// Re-enter focus mode for the newly selected instance
			focusCmd := m.enterFocusMode()
			return m, tea.Batch(cmd, focusCmd)
		}

		// Preview tab focus: forward to embedded terminal
		if m.previewTerminal == nil {
			m.exitFocusMode()
			return m, nil
		}
		data := keyToBytes(msg)
		if data == nil {
			return m, nil
		}
		if err := m.previewTerminal.SendKey(data); err != nil {
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
				selected := m.nav.GetSelectedInstance()
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
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			m.pendingWaveConfirmPlanFile = ""
			// Return the action as a tea.Cmd so bubbletea runs it asynchronously.
			// This prevents blocking the UI during I/O (git push, etc.).
			return m, action
		case "a":
			// 'a' = abort, used by the failed-wave decision dialog.
			abortAction := m.pendingWaveAbortAction
			if abortAction == nil {
				return m, nil // 'a' not bound for this confirm dialog
			}
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			m.pendingWaveConfirmPlanFile = ""
			return m, abortAction
		case m.confirmationOverlay.CancelKey:
			// If this is the failed-wave dialog and 'n' (next wave) is the cancel key,
			// fire the advance action instead of the normal cancel/re-prompt logic.
			if m.pendingWaveNextAction != nil {
				nextAction := m.pendingWaveNextAction
				m.state = stateDefault
				m.confirmationOverlay = nil
				m.pendingConfirmAction = nil
				m.pendingWaveAbortAction = nil
				m.pendingWaveNextAction = nil
				m.pendingWaveConfirmPlanFile = ""
				return m, nextAction
			}
			// "No" — user explicitly declined.
			// Reset the orchestrator confirm latch when the user cancels a wave prompt,
			// so the prompt can reappear on the next metadata tick (fixes deadlock).
			if m.pendingWaveConfirmPlanFile != "" {
				if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmPlanFile]; ok {
					orch.ResetConfirm()
				}
				m.pendingWaveConfirmPlanFile = ""
			}
			// Planner signal "no": kill planner instance, mark prompted, leave plan ready.
			if m.pendingPlannerPlanFile != "" {
				m.plannerPrompted[m.pendingPlannerPlanFile] = true
				m.killExistingPlanAgent(m.pendingPlannerPlanFile, session.AgentTypePlanner)
				_ = m.saveAllInstances()
				m.updateNavPanelStatus()
				m.pendingPlannerInstanceTitle = ""
				m.pendingPlannerPlanFile = ""
			}
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			return m, nil
		case "esc":
			// Esc — preserve everything, allow re-prompt on next tick (after cooldown).
			if m.pendingWaveConfirmPlanFile != "" {
				if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmPlanFile]; ok {
					orch.ResetConfirm()
				}
				m.pendingWaveConfirmPlanFile = ""
				m.waveConfirmDismissedAt = time.Now()
			}
			// Planner signal esc: same as cancel — signal is consumed, can't re-trigger.
			if m.pendingPlannerPlanFile != "" {
				m.plannerPrompted[m.pendingPlannerPlanFile] = true
				m.killExistingPlanAgent(m.pendingPlannerPlanFile, session.AgentTypePlanner)
				_ = m.saveAllInstances()
				m.updateNavPanelStatus()
				m.pendingPlannerInstanceTitle = ""
				m.pendingPlannerPlanFile = ""
			}
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			return m, nil
		default:
			return m, nil
		}
	}

	// Handle permission prompt state
	if m.state == statePermission {
		if m.permissionOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.permissionOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.permissionOverlay.IsConfirmed() {
				choice := m.permissionOverlay.Choice()
				// Read the pattern/description from the overlay (captured at detection
				// time) rather than re-parsing CachedContent, which may have changed.
				cacheKey := config.CacheKey(m.permissionOverlay.Pattern(), m.permissionOverlay.Description())
				inst := m.pendingPermissionInstance

				// Cache "allow always" decisions
				if choice == overlay.PermissionAllowAlways && cacheKey != "" && m.permissionCache != nil {
					m.permissionCache.Remember(cacheKey)
					_ = m.permissionCache.Save()
				}

				m.permissionOverlay = nil
				m.state = stateDefault

				// Guard against re-trigger: the pane still shows the permission
				// prompt for a few ticks while the key sequence propagates.
				// Without this, the next metadata tick re-opens the modal.
				if inst != nil {
					guardKey := cacheKey
					if guardKey == "" {
						guardKey = "__handled__"
					}
					m.permissionHandled[inst] = guardKey
				}

				if inst != nil {
					// overlay.PermissionChoice and tmux.PermissionChoice share the same
					// iota ordering, so a direct cast is safe.
					capturedInst := inst
					capturedChoice := tmux.PermissionChoice(choice)
					m.pendingPermissionInstance = nil
					return m, func() tea.Msg {
						capturedInst.SendPermissionResponse(capturedChoice)
						return nil
					}
				}
			}
			// Esc dismiss — also guard so the same prompt doesn't re-open.
			if m.pendingPermissionInstance != nil {
				guardKey := m.permissionOverlay.Pattern()
				if guardKey == "" {
					guardKey = "__handled__"
				}
				m.permissionHandled[m.pendingPermissionInstance] = guardKey
			}
			m.permissionOverlay = nil
			m.pendingPermissionInstance = nil
			m.state = stateDefault
			return m, nil
		}
		return m, nil
	}

	// Handle new plan description state
	if m.state == stateNewPlan {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				description := strings.TrimSpace(m.textInputOverlay.GetValue())
				if description == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.textInputOverlay = nil
					return m, m.handleError(fmt.Errorf("description cannot be empty"))
				}
				// Set heuristic title as fallback; AI title will replace it when it arrives
				m.pendingPlanName = heuristicPlanTitle(description)
				m.pendingPlanDesc = description
				m.textInputOverlay = nil

				// If the first line is already a viable slug, skip AI derivation
				if firstLineIsViableSlug(description) {
					topicNames := m.getTopicNames()
					topicNames = append([]string{"(No topic)"}, topicNames...)
					pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
					m.pickerOverlay = overlay.NewPickerOverlay(pickerTitle, topicNames)
					m.pickerOverlay.SetAllowCustom(true)
					m.state = stateNewPlanTopic
					return m, nil
				}

				m.state = stateNewPlanDeriving
				if m.toastManager != nil {
					m.toastManager.Info("deriving title...")
					return m, tea.Batch(aiDerivePlanTitleCmd(description), m.toastTickCmd())
				}
				return m, aiDerivePlanTitleCmd(description)
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.textInputOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle deriving state — waiting for AI title before showing topic picker
	if m.state == stateNewPlanDeriving {
		if msg.Type == tea.KeyEscape {
			m.state = stateDefault
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, nil
		}
		return m, nil
	}

	// Handle new plan topic picker state
	if m.state == stateNewPlanTopic {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
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
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pickerOverlay = nil
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle spawn agent form state
	if m.state == stateSpawnAgent {
		if m.formOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.formOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.formOverlay.IsSubmitted() {
				name := m.formOverlay.Name()
				branch := m.formOverlay.Branch()
				workPath := m.formOverlay.WorkPath()
				m.formOverlay = nil

				if name == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					return m, m.handleError(fmt.Errorf("name cannot be empty"))
				}

				return m.spawnAdHocAgent(name, branch, workPath)
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.formOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle change-topic picker for existing plans
	if m.state == stateChangeTopic {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			m.pendingChangeTopicPlan = ""
			return m, nil
		}
		shouldClose := m.pickerOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.pickerOverlay.IsSubmitted() && m.planState != nil && m.pendingChangeTopicPlan != "" {
				picked := m.pickerOverlay.Value()
				newTopic := ""
				if picked != "(No topic)" {
					newTopic = picked
				}
				if entry, ok := m.planState.Plans[m.pendingChangeTopicPlan]; ok {
					entry.Topic = newTopic
					m.planState.Plans[m.pendingChangeTopicPlan] = entry
					if err := m.planState.Save(); err != nil {
						m.state = stateDefault
						m.pickerOverlay = nil
						m.pendingChangeTopicPlan = ""
						return m, m.handleError(err)
					}
					m.updateSidebarPlans()
				}
			}
			m.state = stateDefault
			m.pickerOverlay = nil
			m.pendingChangeTopicPlan = ""
			return m, tea.WindowSize()
		}
		return m, nil
	}

	// Handle set-status picker for force-overriding a plan's status
	if m.state == stateSetStatus {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			m.pendingSetStatusPlan = ""
			return m, nil
		}
		shouldClose := m.pickerOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.pickerOverlay.IsSubmitted() && m.planState != nil && m.pendingSetStatusPlan != "" {
				picked := m.pickerOverlay.Value()
				if picked != "" {
					if err := m.planState.ForceSetStatus(m.pendingSetStatusPlan, planstate.Status(picked)); err != nil {
						m.state = stateDefault
						m.pickerOverlay = nil
						m.pendingSetStatusPlan = ""
						return m, m.handleError(err)
					}
					m.loadPlanState()
					m.updateSidebarPlans()
					m.toastManager.Success(fmt.Sprintf("status → %s", picked))
					m.state = stateDefault
					m.pickerOverlay = nil
					m.pendingSetStatusPlan = ""
					return m, tea.Batch(tea.WindowSize(), m.toastTickCmd())
				}
			}
			m.state = stateDefault
			m.pickerOverlay = nil
			m.pendingSetStatusPlan = ""
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

	// Handle ClickUp search input state
	if m.state == stateClickUpSearch {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}

		closed := m.textInputOverlay.HandleKeyPress(msg)
		if closed {
			if m.textInputOverlay.IsSubmitted() {
				query := strings.TrimSpace(m.textInputOverlay.GetValue())
				if query != "" {
					m.state = stateClickUpFetching
					m.textInputOverlay = nil
					m.toastManager.Info("searching clickup...")
					return m, tea.Batch(m.searchClickUp(query), m.toastTickCmd())
				}
			}
			m.state = stateDefault
			m.textInputOverlay = nil
		}
		return m, nil
	}

	// Handle ClickUp task picker state
	if m.state == stateClickUpPicker {
		if m.pickerOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		closed := m.pickerOverlay.HandleKeyPress(msg)
		if closed {
			if m.pickerOverlay.IsSubmitted() {
				selected := m.pickerOverlay.Value()
				if selected != "" {
					for _, r := range m.clickUpResults {
						label := r.ID + " · " + r.Name
						if strings.HasPrefix(selected, label) {
							m.state = stateClickUpFetching
							m.pickerOverlay = nil
							m.toastManager.Info("fetching task details...")
							return m, tea.Batch(m.fetchClickUpTaskWithTimeout(r.ID), m.toastTickCmd())
						}
					}
				}
			}
			m.state = stateDefault
			m.pickerOverlay = nil
		}
		return m, nil
	}

	if m.state == stateClickUpFetching {
		return m, nil
	}

	if m.state == stateTmuxBrowser {
		if m.tmuxBrowser == nil {
			m.state = stateDefault
			return m, nil
		}
		action := m.tmuxBrowser.HandleKeyPress(msg)
		return m.handleTmuxBrowserAction(action)
	}

	// Handle search state — allows typing to filter AND arrow keys to navigate
	if m.state == stateSearch {
		switch {
		case msg.String() == "esc":
			m.nav.DeactivateSearch()
			m.state = stateDefault
			return m, nil
		case msg.String() == "enter":
			m.nav.DeactivateSearch()
			m.state = stateDefault
			return m, nil
		case msg.String() == "up":
			m.nav.Up()
			return m, m.instanceChanged()
		case msg.String() == "down":
			m.nav.Down()
			return m, m.instanceChanged()
		case msg.Type == tea.KeyBackspace:
			q := m.nav.GetSearchQuery()
			if len(q) > 0 {
				runes := []rune(q)
				m.nav.SetSearchQuery(string(runes[:len(runes)-1]))
			}
			return m, nil
		case msg.Type == tea.KeySpace:
			m.nav.SetSearchQuery(m.nav.GetSearchQuery() + " ")
			return m, nil
		case msg.Type == tea.KeyRunes:
			m.nav.SetSearchQuery(m.nav.GetSearchQuery() + string(msg.Runes))
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
			selected := m.nav.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Forward key events to the viewport when in document or scroll mode.
	// This enables viewport native keys like PgUp/PgDn and arrow keys.
	if (m.tabbedWindow.IsDocumentMode() || m.tabbedWindow.IsPreviewInScrollMode()) &&
		m.tabbedWindow.GetActiveTab() == ui.PreviewTab {
		cmd := m.tabbedWindow.ViewportUpdate(msg)

		// Keep existing shift+up/down behavior as fallback handlers.
		if msg.Type != tea.KeyShiftUp && msg.Type != tea.KeyShiftDown {
			if m.tabbedWindow.ViewportHandlesKey(msg) {
				return m, cmd
			}
		}

		if cmd != nil {
			return m, cmd
		}
	}

	// Ctrl+Up/Down: cycle through active instances (wrapping)
	if msg.Type == tea.KeyCtrlUp || msg.Type == tea.KeyCtrlDown {
		if msg.Type == tea.KeyCtrlUp {
			m.nav.CyclePrevActive()
		} else {
			m.nav.CycleNextActive()
		}
		return m, m.instanceChanged()
	}

	// Ctrl+U/D: half-page scroll in agent session preview
	if msg.Type == tea.KeyCtrlU || msg.Type == tea.KeyCtrlD {
		if msg.Type == tea.KeyCtrlU {
			m.tabbedWindow.HalfPageUp()
		} else {
			m.tabbedWindow.HalfPageDown()
		}
		return m, nil
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	// Shift+Tab: reverse focus ring cycle (don't call instanceChanged — it auto-switches tabs)
	if msg.Type == tea.KeyShiftTab {
		m.prevFocusSlot()
		return m, nil
	}

	// Delete key: dismiss a finished (non-running) instance from the list.
	if msg.Type == tea.KeyDelete || msg.Type == tea.KeyBackspace {
		selected := m.nav.GetSelectedInstance()
		if selected != nil && selected.Status != session.Running && selected.Status != session.Loading {
			title := selected.Title
			m.nav.Remove()
			m.removeFromAllInstances(title)
			_ = m.saveAllInstances()
			m.updateNavPanelStatus()
			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		}
		return m, nil
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt:
		if m.tmuxSessionCount >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances (%d tmux sessions active)", GlobalInstanceLimit, m.tmuxSessionCount))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    m.activeRepoPath,
			Program: m.programForAgent(""),
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.addInstanceFinalizer(instance, m.nav.AddInstance(instance))
		m.newInstance = instance
		m.nav.SetSelectedInstance(m.nav.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNewSkipPermissions:
		if m.tmuxSessionCount >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances (%d tmux sessions active)", GlobalInstanceLimit, m.tmuxSessionCount))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:           "",
			Path:            m.activeRepoPath,
			Program:         m.programForAgent(""),
			SkipPermissions: true,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.addInstanceFinalizer(instance, m.nav.AddInstance(instance))
		m.newInstance = instance
		m.nav.SetSelectedInstance(m.nav.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.tabbedWindow.ClearDocumentMode()
		if m.focusSlot != slotNav {
			m.setFocusSlot(slotNav)
		}
		m.nav.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.tabbedWindow.ClearDocumentMode()
		if m.focusSlot != slotNav {
			m.setFocusSlot(slotNav)
		}
		m.nav.Down()
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.nextFocusSlot()
		return m, nil
	case keys.KeySpace:
		if m.focusSlot == slotNav && m.nav.GetSelectedID() == ui.SidebarImportClickUp {
			m.state = stateClickUpSearch
			m.textInputOverlay = overlay.NewTextInputOverlay("enter clickup id or url", "")
			m.textInputOverlay.SetSize(50, 1)
			return m, nil
		}
		if m.focusSlot == slotNav && m.nav.ToggleSelectedExpand() {
			return m, nil
		}
		return m.openContextMenu()
	case keys.KeyInfoTab:
		// Switch visible tab to info without stealing sidebar focus.
		if m.tabbedWindow.IsInInfoTab() {
			return m, nil
		}
		m.tabbedWindow.SetActiveTab(ui.InfoTab)
		return m, nil
	case keys.KeyTabAgent, keys.KeyTabDiff, keys.KeyTabInfo:
		return m.switchToTab(name)
	case keys.KeySendPrompt:
		// Ensure the agent tab is visible when entering focus mode.
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
		selected := m.nav.GetSelectedInstance()
		// When a plan header is selected (no instance), find the best instance for that plan.
		if selected == nil {
			if pf := m.nav.GetSelectedPlanFile(); pf != "" {
				if best := m.nav.FindPlanInstance(pf); best != nil {
					m.nav.SelectInstance(best)
					selected = best
				}
			}
		}
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		return m, m.enterFocusMode()
	case keys.KeySendYes:
		selected := m.nav.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() || !selected.PromptDetected {
			return m, nil
		}
		selected.QueuedPrompt = "yes"
		selected.AwaitingWork = true
		return m, nil
	case keys.KeyKill:
		// Soft kill: terminate tmux session only, keep instance in list.
		selected := m.nav.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		inst := selected
		return m, func() tea.Msg {
			inst.StopTmux()
			inst.SetStatus(session.Ready)
			return instanceChangedMsg{}
		}
	case keys.KeyAbort:
		// Full abort: kill tmux, remove worktree, remove from list + persistence.
		selected := m.nav.GetSelectedInstance()
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
		message := fmt.Sprintf("[!] abort session '%s'? this removes the worktree.", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[kas] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
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
		message := fmt.Sprintf("[!] push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCreatePR:
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("pr title", selected.Title)
		m.textInputOverlay.SetSize(60, 3)
		return m, nil
	case keys.KeyCheckout:
		selected := m.nav.GetSelectedInstance()
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
		selected := m.nav.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		// Sidebar always has focus: handle plan/instance interactions first.
		if m.nav.GetSelectedID() == ui.SidebarImportClickUp {
			m.state = stateClickUpSearch
			m.textInputOverlay = overlay.NewTextInputOverlay("enter clickup id or url", "")
			m.textInputOverlay.SetSize(50, 1)
			return m, nil
		}
		// Plan header or plan file: open plan context menu
		if m.nav.IsSelectedPlanHeader() {
			return m.openPlanContextMenu()
		}
		if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openPlanContextMenu()
		}
		if m.nav.NumInstances() == 0 {
			return m, nil
		}
		selected := m.nav.GetSelectedInstance()
		if selected == nil || !selected.Started() || selected.Paused() {
			return m, nil
		}
		if !selected.TmuxAlive() {
			m.toastManager.Error(fmt.Sprintf("session for '%s' is not running", selected.Title))
			return m, m.toastTickCmd()
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			ch, err := m.nav.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	case keys.KeyFocusList:
		// t key always jumps directly to the instance list — no-op when list is hidden.
		if m.nav.TotalInstances() > 0 {
			m.setFocusSlot(slotNav)
		}
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
			// If sidebar was focused, move focus to agent tab
			if m.focusSlot == slotNav {
				m.setFocusSlot(slotAgent)
			}
		}
		return m, tea.WindowSize()
	case keys.KeyArrowLeft:
		// Sidebar always has focus — no-op.
		return m, nil
	case keys.KeyArrowRight:
		// Toggle expand/collapse on the selected sidebar item (same as space's expand behavior).
		if m.nav.GetSelectedID() == ui.SidebarImportClickUp {
			m.state = stateClickUpSearch
			m.textInputOverlay = overlay.NewTextInputOverlay("enter clickup id or url", "")
			m.textInputOverlay.SetSize(50, 1)
			return m, nil
		}
		m.nav.ToggleSelectedExpand()
		return m, nil
	case keys.KeyNewPlan:
		m.state = stateNewPlan
		m.textInputOverlay = overlay.NewTextInputOverlay("new plan", "")
		m.textInputOverlay.SetMultiline(true)
		m.textInputOverlay.SetPlaceholder("describe what you want to work on...")
		m.textInputOverlay.SetSize(70, 8)
		return m, nil
	case keys.KeySpawnAgent:
		if m.tmuxSessionCount >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances (%d tmux sessions active)", GlobalInstanceLimit, m.tmuxSessionCount))
		}
		m.state = stateSpawnAgent
		m.formOverlay = overlay.NewSpawnFormOverlay("spawn agent", 60)
		return m, nil
	case keys.KeyRepoSwitch:
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	case keys.KeyTmuxBrowser:
		return m, m.discoverTmuxSessions()
	case keys.KeySearch:
		m.nav.ActivateSearch()
		m.nav.SelectFirst()
		m.state = stateSearch
		m.setFocusSlot(slotNav)
		return m, nil
	default:
		return m, nil
	}
}

// keyToBytes translates a Bubble Tea key message to raw bytes for PTY forwarding.
func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		if msg.Alt {
			return append([]byte{0x1b}, []byte(string(msg.Runes))...)
		}
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
	case tea.KeyShiftUp:
		return []byte("\x1b[1;2A")
	case tea.KeyShiftDown:
		return []byte("\x1b[1;2B")
	case tea.KeyShiftRight:
		return []byte("\x1b[1;2C")
	case tea.KeyShiftLeft:
		return []byte("\x1b[1;2D")
	case tea.KeyShiftHome:
		return []byte("\x1b[1;2H")
	case tea.KeyShiftEnd:
		return []byte("\x1b[1;2F")
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

// waveStandardConfirmAction shows the wave-advance confirmation for a wave with no failures.
// Stores the plan file so the cancel path can reset the orchestrator confirm latch.
func (m *home) waveStandardConfirmAction(message, planFile string, entry planstate.PlanEntry) {
	m.pendingWaveConfirmPlanFile = planFile
	capturedPlanFile := planFile
	capturedEntry := entry
	m.confirmAction(message, func() tea.Msg {
		return waveAdvanceMsg{planFile: capturedPlanFile, entry: capturedEntry}
	})
}

// waveFailedConfirmAction shows a three-choice dialog for a wave that has failed tasks.
// Keys: r=retry, n=next wave/advance, a=abort. The abort action is stored separately so the
// stateConfirm key handler can dispatch it on 'a'.
func (m *home) waveFailedConfirmAction(message, planFile string, entry planstate.PlanEntry) {
	m.pendingWaveConfirmPlanFile = planFile
	capturedPlanFile := planFile
	capturedEntry := entry

	m.state = stateConfirm
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	m.confirmationOverlay.ConfirmKey = "r"
	m.confirmationOverlay.CancelKey = "n"
	m.confirmationOverlay.SetWidth(60)

	m.pendingConfirmAction = func() tea.Msg {
		return waveRetryMsg{planFile: capturedPlanFile, entry: capturedEntry}
	}
	m.pendingWaveNextAction = func() tea.Msg {
		return waveAdvanceMsg{planFile: capturedPlanFile, entry: capturedEntry}
	}
	m.pendingWaveAbortAction = func() tea.Msg {
		return waveAbortMsg{planFile: capturedPlanFile}
	}
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
