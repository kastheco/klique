package app

import (
	"fmt"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/mattn/go-runewidth"
)

func (m *home) handleMenuHighlighting(msg tea.KeyPressMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm || m.state == stateNewPlan || m.state == stateNewPlanDeriving || m.state == stateNewPlanTopic || m.state == stateSpawnAgent || m.state == stateSearch || m.state == stateContextMenu || m.state == statePRTitle || m.state == statePRBody || m.state == stateRenameInstance || m.state == stateRenameTask || m.state == stateSendPrompt || m.state == stateFocusAgent || m.state == stateChangeTopic || m.state == stateSetStatus || m.state == stateClickUpSearch || m.state == stateClickUpPicker || m.state == stateClickUpFetching || m.state == stateClickUpWorkspacePicker || m.state == statePermission || m.state == stateTmuxBrowser || m.state == stateChatAboutTask {
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

// handleMouseWheel processes mouse wheel events for scrolling.
func (m *home) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	selected := m.nav.GetSelectedInstance()
	if selected != nil && selected.Status != session.Paused {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.tabbedWindow.ContentScrollUp()
		case tea.MouseWheelDown:
			m.tabbedWindow.ContentScrollDown()
		}
	}
	return m, nil
}

// handleMouseClick processes mouse click events for left/right click interactions.
func (m *home) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Dismiss overlays on click-outside
	if m.state == stateContextMenu && msg.Button == tea.MouseLeft {
		m.overlays.Dismiss()
		m.state = stateDefault
		return m, nil
	}
	if m.state == stateTmuxBrowser && msg.Button == tea.MouseLeft {
		m.overlays.Dismiss()
		m.state = stateDefault
		return m, nil
	}
	if m.state != stateDefault {
		return m, nil
	}

	// Right-click: show context menu
	if msg.Button == tea.MouseRight {
		return m.handleRightClick(msg)
	}

	// Only handle left clicks from here
	if msg.Button != tea.MouseLeft {
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
func (m *home) handleRightClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Right-click in nav panel: select the clicked row then show context menu
	if zone.Get(ui.ZoneNavPanel).InBounds(msg) {
		for i := range m.nav.RowCount() {
			if zone.Get(ui.NavRowZoneID(i)).InBounds(msg) {
				m.nav.ClickItem(i)
				break
			}
		}
		if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openTaskContextMenu()
		}
		return m, nil
	}
	return m, nil
}

func (m *home) handleKeyPress(msg tea.KeyPressMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateContextMenu {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			m.state = stateDefault
			if result.Action != "" {
				return m.executeContextAction(result.Action)
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
				tea.RequestWindowSize,
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
		switch msg.Code {
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
				tio := overlay.NewTextInputOverlay("enter prompt", "")
				tio.SetSize(50, 5)
				m.overlays.Show(tio)
				m.promptAfterName = false
			}

			// Start instance asynchronously
			startCmd := func() tea.Msg {
				return instanceStartedMsg{instance: instance, err: instance.Start(true)}
			}

			return m, tea.Batch(tea.RequestWindowSize, startCmd)
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
		case tea.KeyEscape:
			m.nav.Kill()
			m.state = stateDefault
			m.newInstance = nil
			m.instanceChanged()

			return m, tea.Sequence(
				tea.RequestWindowSize,
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
			if len(msg.Text) > 0 {
				if runewidth.StringWidth(instance.Title) >= 32 {
					return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
				}
				if err := instance.SetTitle(instance.Title + msg.Text); err != nil {
					return m, m.handleError(err)
				}
			}
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		result := m.overlays.HandleKey(msg)

		// Check if the form was submitted or canceled
		if result.Dismissed {
			selected := m.nav.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if result.Submitted {
				promptText := result.Value
				if err := selected.SendPrompt(promptText); err != nil {
					// TODO: we probably end up in a bad state here.
					return m, m.handleError(err)
				}
				// Emit audit event for prompt sent (truncate to 200 chars).
				msg := promptText
				if len(msg) > 200 {
					msg = msg[:200]
				}
				m.audit(auditlog.EventPromptSent, msg,
					auditlog.WithInstance(selected.Title),
				)
			}

			// Close the overlay and reset state
			m.state = stateDefault
			return m, tea.Sequence(
				tea.RequestWindowSize,
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
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				prTitle := result.Value
				selected := m.nav.GetSelectedInstance()
				if selected != nil && prTitle != "" {
					m.pendingPRTitle = prTitle

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
					tio := overlay.NewTextInputOverlay("pr description (edit or submit)", generatedBody)
					tio.SetSize(80, 20)
					m.overlays.Show(tio)
					return m, nil
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle PR body input state
	if m.state == statePRBody {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				prBody := result.Value
				prTitle := m.pendingPRTitle
				selected := m.nav.GetSelectedInstance()
				if selected != nil && prTitle != "" {
					m.pendingPRTitle = ""
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.pendingPRToastID = m.toastManager.Loading("Creating PR...")
					prToastID := m.pendingPRToastID
					capturedTitle := selected.Title
					capturedPRTitle := prTitle
					return m, tea.Batch(tea.RequestWindowSize, func() tea.Msg {
						commitMsg := fmt.Sprintf("[kas] update from '%s' on %s", capturedTitle, time.Now().Format(time.RFC822))
						worktree, err := selected.GetGitWorktree()
						if err != nil {
							return prErrorMsg{id: prToastID, err: err}
						}
						if err := worktree.CreatePR(capturedPRTitle, prBody, commitMsg); err != nil {
							return prErrorMsg{id: prToastID, err: err}
						}
						return prCreatedMsg{instanceTitle: capturedTitle, prTitle: capturedPRTitle}
					}, m.toastTickCmd())
				}
			}
			m.pendingPRTitle = ""
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle instance rename state
	if m.state == stateRenameInstance {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				newName := result.Value
				selected := m.nav.GetSelectedInstance()
				if selected != nil && newName != "" {
					selected.Title = newName
					m.saveAllInstances()
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle plan rename state
	if m.state == stateRenameTask {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				newName := strings.TrimSpace(result.Value)
				planFile := m.nav.GetSelectedPlanFile()
				if planFile != "" && newName != "" && m.taskState != nil {
					oldFile := planFile
					newFile, err := m.taskState.Rename(oldFile, newName)
					if err != nil {
						m.state = stateDefault
						m.menu.SetState(ui.StateDefault)
						return m, m.handleError(err)
					}
					// Update any instances that referenced the old plan file.
					for _, inst := range m.nav.GetInstances() {
						if inst.TaskFile == oldFile {
							inst.TaskFile = newFile
						}
					}
					for _, inst := range m.allInstances {
						if inst.TaskFile == oldFile {
							inst.TaskFile = newFile
						}
					}
					_ = m.saveAllInstances()
					m.updateSidebarTasks()
					m.nav.SelectByID(ui.SidebarPlanPrefix + newFile)
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle chat-about-plan question input
	if m.state == stateChatAboutTask {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				question := result.Value
				planFile := m.pendingChatAboutTask
				m.pendingChatAboutTask = ""
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				if planFile != "" && question != "" {
					return m.spawnChatAboutTask(planFile, question)
				}
				return m, tea.RequestWindowSize
			}
			m.pendingChatAboutTask = ""
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle focus mode — forward keys directly to the agent's PTY
	if m.state == stateFocusAgent {
		// Ctrl+Space exits focus mode
		if msg.String() == "ctrl+space" {
			m.exitFocusMode()
			return m, tea.RequestWindowSize
		}

		// Ctrl+Up/Down: cycle through active instances (wrapping) while staying in focus mode
		if msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModCtrl) || msg.Code == tea.KeyDown && msg.Mod.Contains(tea.ModCtrl) {
			if msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModCtrl) {
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
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				value := result.Value
				selected := m.nav.GetSelectedInstance()
				if selected != nil && value != "" {
					if err := selected.SendPrompt(value); err != nil {
						m.state = stateDefault
						m.menu.SetState(ui.StateDefault)
						return m, m.handleError(err)
					}
					selected.SetStatus(session.Running)
					// Emit audit event for prompt sent (truncate to 200 chars).
					auditMsg := value
					if len(auditMsg) > 200 {
						auditMsg = auditMsg[:200]
					}
					m.audit(auditlog.EventPromptSent, auditMsg,
						auditlog.WithInstance(selected.Title),
					)
				}
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		// Pre-intercept 'a' (abort) before delegating to the overlay.
		if msg.String() == "a" && m.pendingWaveAbortAction != nil {
			abortAction := m.pendingWaveAbortAction
			m.overlays.Dismiss()
			m.state = stateDefault
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			m.pendingWaveConfirmTaskFile = ""
			return m, abortAction
		}
		// Pre-intercept 'enter' as an alias for the confirm key.
		// ConfirmationOverlay.HandleKey only handles ConfirmKey ("y"/"r"), not "enter".
		effectiveMsg := msg
		if msg.Code == tea.KeyEnter {
			if co, ok := m.overlays.Current().(*overlay.ConfirmationOverlay); ok {
				effectiveMsg = tea.KeyPressMsg{Text: co.ConfirmKey, Code: rune(co.ConfirmKey[0])}
			}
		}
		result := m.overlays.HandleKey(effectiveMsg)
		if result.Dismissed {
			if result.Submitted {
				// Confirmed (ConfirmKey pressed).
				action := m.pendingConfirmAction
				m.state = stateDefault
				m.pendingConfirmAction = nil
				m.pendingWaveAbortAction = nil
				m.pendingWaveNextAction = nil
				m.pendingWaveConfirmTaskFile = ""
				// Return the action as a tea.Cmd so bubbletea runs it asynchronously.
				// This prevents blocking the UI during I/O (git push, etc.).
				return m, action
			}
			// Cancelled (CancelKey or Esc).
			cancelKey := result.Action // Action holds the key that triggered cancel
			if cancelKey == "" {
				// Esc dismiss — preserve everything, allow re-prompt on next tick (after cooldown).
				if m.pendingWaveConfirmTaskFile != "" {
					if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmTaskFile]; ok {
						orch.ResetConfirm()
					}
					m.pendingWaveConfirmTaskFile = ""
					m.waveConfirmDismissedAt = time.Now()
				}
				// Planner signal esc: same as cancel — signal is consumed, can't re-trigger.
				if m.pendingPlannerTaskFile != "" {
					m.plannerPrompted[m.pendingPlannerTaskFile] = true
					m.killExistingPlanAgent(m.pendingPlannerTaskFile, session.AgentTypePlanner)
					_ = m.saveAllInstances()
					m.updateNavPanelStatus()
					m.pendingPlannerInstanceTitle = ""
					m.pendingPlannerTaskFile = ""
				}
				m.state = stateDefault
				m.pendingConfirmAction = nil
				m.pendingWaveAbortAction = nil
				m.pendingWaveNextAction = nil
				return m, nil
			}
			// CancelKey pressed — check if this is the failed-wave dialog where CancelKey="n" fires advance.
			if m.pendingWaveNextAction != nil {
				nextAction := m.pendingWaveNextAction
				m.state = stateDefault
				m.pendingConfirmAction = nil
				m.pendingWaveAbortAction = nil
				m.pendingWaveNextAction = nil
				m.pendingWaveConfirmTaskFile = ""
				return m, nextAction
			}
			// "No" — user explicitly declined.
			// Reset the orchestrator confirm latch when the user cancels a wave prompt,
			// so the prompt can reappear on the next metadata tick (fixes deadlock).
			if m.pendingWaveConfirmTaskFile != "" {
				if orch, ok := m.waveOrchestrators[m.pendingWaveConfirmTaskFile]; ok {
					orch.ResetConfirm()
				}
				m.pendingWaveConfirmTaskFile = ""
			}
			// Planner signal "no": kill planner instance, mark prompted, leave plan ready.
			if m.pendingPlannerTaskFile != "" {
				m.plannerPrompted[m.pendingPlannerTaskFile] = true
				m.killExistingPlanAgent(m.pendingPlannerTaskFile, session.AgentTypePlanner)
				_ = m.saveAllInstances()
				m.updateNavPanelStatus()
				m.pendingPlannerInstanceTitle = ""
				m.pendingPlannerTaskFile = ""
			}
			m.state = stateDefault
			m.pendingConfirmAction = nil
			m.pendingWaveAbortAction = nil
			m.pendingWaveNextAction = nil
			return m, nil
		}
		return m, nil
	}

	// Handle permission prompt state
	if m.state == statePermission {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				// Read the pattern/description captured at detection time.
				cacheKey := config.CacheKey(m.pendingPermissionPattern, m.pendingPermissionDesc)
				inst := m.pendingPermissionInstance

				// Map action string to PermissionChoice.
				var choice overlay.PermissionChoice
				switch result.Action {
				case "allow_always":
					choice = overlay.PermissionAllowAlways
				case "reject":
					choice = overlay.PermissionReject
				default:
					choice = overlay.PermissionAllowOnce
				}

				// Cache "allow always" decisions
				if choice == overlay.PermissionAllowAlways && cacheKey != "" && m.permissionStore != nil {
					m.permissionStore.Remember(m.activeProject(), cacheKey)
				}

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
					// Emit audit event for permission answered.
					choiceStr := "allow once"
					switch choice {
					case overlay.PermissionAllowAlways:
						choiceStr = "allow always"
					case overlay.PermissionReject:
						choiceStr = "reject"
					}
					m.audit(auditlog.EventPermissionAnswered, choiceStr,
						auditlog.WithInstance(inst.Title),
					)
					m.pendingPermissionInstance = nil
					m.pendingPermissionPattern = ""
					m.pendingPermissionDesc = ""
					return m, func() tea.Msg {
						capturedInst.SendPermissionResponse(capturedChoice)
						return nil
					}
				}
			}
			// Esc dismiss — also guard so the same prompt doesn't re-open.
			if m.pendingPermissionInstance != nil {
				guardKey := m.pendingPermissionPattern
				if guardKey == "" {
					guardKey = "__handled__"
				}
				m.permissionHandled[m.pendingPermissionInstance] = guardKey
			}
			m.pendingPermissionInstance = nil
			m.pendingPermissionPattern = ""
			m.pendingPermissionDesc = ""
			m.state = stateDefault
			return m, nil
		}
		return m, nil
	}

	// Handle new plan description state
	if m.state == stateNewPlan {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				description := strings.TrimSpace(result.Value)
				if description == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					return m, m.handleError(fmt.Errorf("description cannot be empty"))
				}
				// Set heuristic title as fallback; AI title will replace it when it arrives
				m.pendingPlanName = heuristicPlanTitle(description)
				m.pendingPlanDesc = description

				// If the first line is already a viable slug, skip AI derivation
				if firstLineIsViableSlug(description) {
					topicNames := m.getTopicNames()
					topicNames = append([]string{"(No topic)"}, topicNames...)
					pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
					po := overlay.NewPickerOverlay(pickerTitle, topicNames)
					po.SetAllowCustom(true)
					m.overlays.Show(po)
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
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle deriving state — waiting for AI title before showing topic picker
	if m.state == stateNewPlanDeriving {
		if msg.Code == tea.KeyEscape {
			m.state = stateDefault
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, nil
		}
		return m, nil
	}

	// Handle new plan topic picker state
	if m.state == stateNewPlanTopic {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			topic := ""
			if result.Submitted {
				picked := result.Value
				if picked != "(No topic)" {
					topic = picked
				}
			}
			if err := m.createTaskEntry(m.pendingPlanName, m.pendingPlanDesc, topic); err != nil {
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				m.pendingPlanName = ""
				m.pendingPlanDesc = ""
				return m, m.handleError(err)
			}
			m.loadTaskState()
			m.updateSidebarTasks()
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.pendingPlanName = ""
			m.pendingPlanDesc = ""
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle spawn agent form state
	if m.state == stateSpawnAgent {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		// Type-assert before HandleKey to access Name/Branch/WorkPath after dismiss.
		fo, _ := m.overlays.Current().(*overlay.FormOverlay)
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted && fo != nil {
				name := fo.Name()
				branch := fo.Branch()
				workPath := fo.WorkPath()

				if name == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					return m, m.handleError(fmt.Errorf("name cannot be empty"))
				}

				return m.spawnAdHocAgent(name, branch, workPath)
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle change-topic picker for existing plans
	if m.state == stateChangeTopic {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			m.pendingChangeTopicTask = ""
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted && m.taskState != nil && m.pendingChangeTopicTask != "" {
				picked := result.Value
				newTopic := ""
				if picked != "(No topic)" {
					newTopic = picked
				}
				if err := m.taskState.SetTopic(m.pendingChangeTopicTask, newTopic); err != nil {
					m.state = stateDefault
					m.pendingChangeTopicTask = ""
					return m, m.handleError(err)
				}
				m.updateSidebarTasks()
			}
			m.state = stateDefault
			m.pendingChangeTopicTask = ""
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle set-status picker for force-overriding a plan's status
	if m.state == stateSetStatus {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			m.pendingSetStatusTask = ""
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted && m.taskState != nil && m.pendingSetStatusTask != "" {
				picked := result.Value
				if picked != "" {
					if err := m.taskState.ForceSetStatus(m.pendingSetStatusTask, taskstate.Status(picked)); err != nil {
						m.state = stateDefault
						m.pendingSetStatusTask = ""
						return m, m.handleError(err)
					}
					m.audit(auditlog.EventPlanTransition, "manual override → "+picked,
						auditlog.WithPlan(m.pendingSetStatusTask),
						auditlog.WithDetail("manual override"))
					m.loadTaskState()
					m.updateSidebarTasks()
					m.toastManager.Success(fmt.Sprintf("status → %s", picked))
					m.state = stateDefault
					m.pendingSetStatusTask = ""
					return m, tea.Batch(tea.RequestWindowSize, m.toastTickCmd())
				}
			}
			m.state = stateDefault
			m.pendingSetStatusTask = ""
			return m, tea.RequestWindowSize
		}
		return m, nil
	}

	// Handle ClickUp search input state
	if m.state == stateClickUpSearch {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				query := strings.TrimSpace(result.Value)
				if query != "" {
					m.state = stateClickUpFetching
					m.toastManager.Info("searching clickup...")
					return m, tea.Batch(m.searchClickUp(query), m.toastTickCmd())
				}
			}
			m.state = stateDefault
		}
		return m, nil
	}

	// Handle ClickUp task picker state
	if m.state == stateClickUpPicker {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				selected := result.Value
				if selected != "" {
					for _, r := range m.clickUpResults {
						label := r.ID + " · " + r.Name
						if strings.HasPrefix(selected, label) {
							m.state = stateClickUpFetching
							m.toastManager.Info("fetching task details...")
							return m, tea.Batch(m.fetchClickUpTaskWithTimeout(r.ID), m.toastTickCmd())
						}
					}
				}
			}
			m.state = stateDefault
		}
		return m, nil
	}

	if m.state == stateClickUpFetching {
		return m, nil
	}

	// Handle ClickUp workspace picker state
	if m.state == stateClickUpWorkspacePicker {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			if result.Submitted {
				selected := result.Value
				if selected != "" && m.clickUpImporter != nil {
					// Resolve label ("name (id)") back to bare workspace ID.
					wsID := selected
					if id, ok := m.clickUpWorkspaceMap[selected]; ok {
						wsID = id
					}
					m.clickUpImporter.SetWorkspaceID(wsID)
					// Persist choice so user isn't prompted again for this project.
					if err := clickup.SaveProjectConfig(m.activeRepoPath, &clickup.ProjectConfig{
						WorkspaceID: wsID,
					}); err != nil {
						log.WarningLog.Printf("failed to save clickup workspace config: %v", err)
					}
					query := m.clickUpPendingQuery
					m.clickUpPendingQuery = ""
					m.clickUpWorkspaceMap = nil
					m.state = stateClickUpFetching
					m.toastManager.Info("searching clickup...")
					return m, tea.Batch(m.searchClickUp(query), m.toastTickCmd())
				}
			}
			m.state = stateDefault
			m.clickUpPendingQuery = ""
			m.clickUpWorkspaceMap = nil
		}
		return m, nil
	}

	if m.state == stateTmuxBrowser {
		if !m.overlays.IsActive() {
			m.state = stateDefault
			return m, nil
		}
		browser, _ := m.overlays.Current().(*overlay.TmuxBrowserOverlay)
		result := m.overlays.HandleKey(msg)
		if result.Dismissed {
			m.state = stateDefault
			return m.handleTmuxBrowserAction(browser, result.Action)
		}
		// Handle non-dismissed actions (e.g. "kill" keeps the browser open for
		// multi-kill workflow — the action handler decides whether to dismiss).
		if result.Action != "" {
			return m.handleTmuxBrowserAction(browser, result.Action)
		}
		return m, nil
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
		case msg.Code == tea.KeyBackspace:
			q := m.nav.GetSearchQuery()
			if len(q) > 0 {
				runes := []rune(q)
				m.nav.SetSearchQuery(string(runes[:len(runes)-1]))
			}
			return m, nil
		case msg.Code == tea.KeySpace:
			m.nav.SetSearchQuery(m.nav.GetSearchQuery() + " ")
			return m, nil
		case len(msg.Text) > 0:
			m.nav.SetSearchQuery(m.nav.GetSearchQuery() + msg.Text)
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Code == tea.KeyEscape {
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
		if msg.String() != "shift+up" && msg.String() != "shift+down" {
			if m.tabbedWindow.ViewportHandlesKey(msg) {
				return m, cmd
			}
		}

		if cmd != nil {
			return m, cmd
		}
	}

	// Ctrl+Up/Down: cycle through active instances (wrapping)
	if msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModCtrl) || msg.Code == tea.KeyDown && msg.Mod.Contains(tea.ModCtrl) {
		if msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModCtrl) {
			m.nav.CyclePrevActive()
		} else {
			m.nav.CycleNextActive()
		}
		return m, m.instanceChanged()
	}

	// Ctrl+U/D: half-page scroll in agent session preview
	if msg.Code == 'u' && msg.Mod.Contains(tea.ModCtrl) || msg.Code == 'd' && msg.Mod.Contains(tea.ModCtrl) {
		if msg.Code == 'u' && msg.Mod.Contains(tea.ModCtrl) {
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
	if msg.String() == "shift+tab" {
		m.prevFocusSlot()
		return m, nil
	}

	// Delete key: dismiss a finished (non-running) instance from the list.
	if msg.Code == tea.KeyDelete || msg.Code == tea.KeyBackspace {
		selected := m.nav.GetSelectedInstance()
		if selected != nil && (selected.Exited || (selected.Status != session.Running && selected.Status != session.Loading)) {
			title := selected.Title
			m.nav.Remove()
			m.removeFromAllInstances(title)
			_ = m.saveAllInstances()
			m.updateNavPanelStatus()
			return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged())
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
			tio := overlay.NewTextInputOverlay("enter clickup id or url", "")
			tio.SetSize(50, 1)
			m.overlays.Show(tio)
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
		if selected == nil || !selected.Started() || selected.Paused() || selected.Exited {
			return m, nil
		}
		m.audit(auditlog.EventAgentKilled, "killed instance",
			auditlog.WithInstance(selected.Title),
			auditlog.WithAgent(selected.AgentType),
			auditlog.WithPlan(selected.TaskFile),
		)
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
		message := fmt.Sprintf("stop session '%s'? branch will be preserved.", selected.Title)
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
		tio := overlay.NewTextInputOverlay("pr title", selected.Title)
		tio.SetSize(60, 3)
		m.overlays.Show(tio)
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
		return m, tea.RequestWindowSize
	case keys.KeyEnter:
		// Sidebar always has focus: handle plan/instance interactions first.
		if m.nav.GetSelectedID() == ui.SidebarImportClickUp {
			m.state = stateClickUpSearch
			tio := overlay.NewTextInputOverlay("enter clickup id or url", "")
			tio.SetSize(50, 1)
			m.overlays.Show(tio)
			return m, nil
		}
		// Plan header or plan file: open plan context menu
		if m.nav.IsSelectedPlanHeader() {
			return m.openTaskContextMenu()
		}
		if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openTaskContextMenu()
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
		return m, tea.RequestWindowSize
	case keys.KeyAuditToggle:
		if m.auditPane != nil {
			m.auditPane.ToggleVisible()
		}
		return m, tea.RequestWindowSize
	case keys.KeyArrowLeft:
		// Sidebar always has focus — no-op.
		return m, nil
	case keys.KeyArrowRight:
		// Toggle expand/collapse on the selected sidebar item (same as space's expand behavior).
		if m.nav.GetSelectedID() == ui.SidebarImportClickUp {
			m.state = stateClickUpSearch
			tio := overlay.NewTextInputOverlay("enter clickup id or url", "")
			tio.SetSize(50, 1)
			m.overlays.Show(tio)
			return m, nil
		}
		// Right on an instance while in the info tab: jump to the agent tab.
		if m.nav.GetSelectedInstance() != nil && m.tabbedWindow.IsInInfoTab() {
			m.tabbedWindow.SetActiveTab(ui.PreviewTab)
			return m, nil
		}
		m.nav.ToggleSelectedExpand()
		return m, nil
	case keys.KeyNewPlan:
		m.state = stateNewPlan
		tio := overlay.NewTextInputOverlay("new plan", "")
		tio.SetMultiline(true)
		tio.SetPlaceholder("describe what you want to work on...")
		tio.SetSize(70, 8)
		m.overlays.Show(tio)
		return m, nil
	case keys.KeySpawnAgent:
		if m.tmuxSessionCount >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances (%d tmux sessions active)", GlobalInstanceLimit, m.tmuxSessionCount))
		}
		m.state = stateSpawnAgent
		m.overlays.Show(overlay.NewSpawnFormOverlay("spawn agent", 60))
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
func keyToBytes(msg tea.KeyPressMsg) []byte {
	// Handle modifier combinations first.
	if msg.Mod.Contains(tea.ModCtrl) {
		// Ctrl+letter → raw control character byte (0x01..0x1A).
		if msg.Code >= 'a' && msg.Code <= 'z' {
			return []byte{byte(msg.Code - 'a' + 1)}
		}
	}
	if msg.Mod.Contains(tea.ModShift) {
		switch msg.Code {
		case tea.KeyTab:
			return []byte("\x1b[Z")
		case tea.KeyUp:
			return []byte("\x1b[1;2A")
		case tea.KeyDown:
			return []byte("\x1b[1;2B")
		case tea.KeyRight:
			return []byte("\x1b[1;2C")
		case tea.KeyLeft:
			return []byte("\x1b[1;2D")
		case tea.KeyHome:
			return []byte("\x1b[1;2H")
		case tea.KeyEnd:
			return []byte("\x1b[1;2F")
		}
	}
	if msg.Mod.Contains(tea.ModAlt) && len(msg.Text) > 0 {
		return append([]byte{0x1b}, []byte(msg.Text)...)
	}

	// Printable text with no modifiers.
	if len(msg.Text) > 0 {
		return []byte(msg.Text)
	}

	// Special keys (no modifiers).
	switch msg.Code {
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
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyEscape:
		return []byte{0x1b}
	default:
		return nil
	}
}

func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.toastManager.Error(err.Error())
	m.audit(auditlog.EventError, err.Error(), auditlog.WithLevel("error"))
	return m.toastTickCmd()
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm.
// The action is a tea.Cmd that will be returned from Update() to run asynchronously —
// never called synchronously, which would block the UI during I/O operations.
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.pendingConfirmAction = action

	co := overlay.NewConfirmationOverlay(message)
	m.overlays.Show(co)

	return nil
}

// waveStandardConfirmAction shows the wave-advance confirmation for a wave with no failures.
// Stores the plan file so the cancel path can reset the orchestrator confirm latch.
func (m *home) waveStandardConfirmAction(message, planFile string, entry taskstate.TaskEntry) {
	m.pendingWaveConfirmTaskFile = planFile
	capturedPlanFile := planFile
	capturedEntry := entry
	m.confirmAction(message, func() tea.Msg {
		return waveAdvanceMsg{planFile: capturedPlanFile, entry: capturedEntry}
	})
}

// waveFailedConfirmAction shows a three-choice dialog for a wave that has failed tasks.
// Keys: r=retry, n=next wave/advance, a=abort. The abort action is stored separately so the
// stateConfirm key handler can dispatch it on 'a'.
func (m *home) waveFailedConfirmAction(message, planFile string, entry taskstate.TaskEntry) {
	m.pendingWaveConfirmTaskFile = planFile
	capturedPlanFile := planFile
	capturedEntry := entry

	m.state = stateConfirm
	co := overlay.NewConfirmationOverlay(message)
	co.ConfirmKey = "r"
	co.CancelKey = "n"
	co.SetSize(60, 0)
	m.overlays.Show(co)

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
