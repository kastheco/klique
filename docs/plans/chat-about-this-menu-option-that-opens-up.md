# Chat About This — Plan Context Menu Action

**Goal:** Add a "chat about this" option to the plan context menu that spawns a custodian agent pre-loaded with plan context and a user-provided question, enabling quick Q&A about any plan.

**Architecture:** A new `stateChatAboutPlan` state captures the user's question via `TextInputOverlay`. On submit, `spawnChatAboutPlan()` builds a prompt containing the plan's metadata (name, status, description, branch, wave progress) and the plan file content, then spawns a custodian agent on the plan's branch (or main if no branch). The custodian runs in the plan's shared worktree so it can read the actual implementation files. The context menu action `"chat_about_plan"` is added to `openPlanContextMenu()` and handled in `executeContextAction()`.

**Tech Stack:** Go, bubbletea, overlay components, session/instance lifecycle

**Size:** Small (estimated ~1 hour, 2 tasks, 1 wave)

---

## Wave 1: Chat About This

### Task 1: Add State, Overlay, and Prompt Builder

**Files:**
- Modify: `app/app.go` (add `stateChatAboutPlan` state constant, render overlay)
- Modify: `app/app_state.go` (add `buildChatAboutPlanPrompt()` helper, add `spawnChatAboutPlan()`)
- Modify: `app/app_input.go` (add `stateChatAboutPlan` to menu-highlighting guard, handle key events for the new state)

**Step 1: write the failing test**

Add a test in `app/app_test.go`:

```go
func TestChatAboutPlan_ContextMenuAction(t *testing.T) {
	h := newTestHome()
	h.setupPlanState("test-plan.md", planstate.StatusImplementing, "test topic")

	// Select the plan in the nav panel
	h.nav.SelectByID(ui.SidebarPlanPrefix + "test-plan.md")

	// Execute the context action
	model, _ := h.executeContextAction("chat_about_plan")
	updated := model.(*home)

	require.Equal(t, stateChatAboutPlan, updated.state)
	require.NotNil(t, updated.textInputOverlay, "text input overlay must be set for question")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestChatAboutPlan_ContextMenuAction -v -count=1
```

expected: FAIL — `stateChatAboutPlan` undefined

**Step 3: write minimal implementation**

1. In `app/app.go`, add the new state constant after `stateTmuxBrowser`:
   ```go
   // stateChatAboutPlan is the state when the user is typing a question about a plan.
   stateChatAboutPlan
   ```

2. In `app/app.go` `View()`, add the overlay rendering case alongside other text input overlays:
   ```go
   case m.state == stateChatAboutPlan && m.textInputOverlay != nil:
       result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
   ```

3. In `app/app_input.go`, add `stateChatAboutPlan` to the menu-highlighting guard list (the long `if m.state == ...` condition).

4. In `app/app_input.go`, add the key handler block for `stateChatAboutPlan` (after the rename plan handler):
   ```go
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
   ```

5. In `app/app.go` `home` struct, add the pending field:
   ```go
   pendingChatAboutPlan string
   ```

6. In `app/app_state.go`, add the prompt builder:
   ```go
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
   ```

7. In `app/app_state.go`, add the spawn function:
   ```go
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
           Program:   m.programForAgent(session.AgentTypeCustodian),
           PlanFile:  planFile,
           AgentType: session.AgentTypeCustodian,
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

       m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
       m.nav.SelectInstance(inst)
       return m, tea.Batch(tea.WindowSize(), startCmd)
   }
   ```

8. In `app/app_actions.go` `executeContextAction()`, add the `"chat_about_plan"` case:
   ```go
   case "chat_about_plan":
       planFile := m.nav.GetSelectedPlanFile()
       if planFile == "" {
           return m, nil
       }
       m.pendingChatAboutPlan = planFile
       m.state = stateChatAboutPlan
       m.textInputOverlay = overlay.NewTextInputOverlay("ask about this plan", "")
       m.textInputOverlay.SetSize(60, 5)
       m.textInputOverlay.SetMultiline(true)
       m.textInputOverlay.SetPlaceholder("what would you like to know?")
       return m, nil
   ```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestChatAboutPlan -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_state.go app/app_input.go app/app_actions.go app/app_test.go
git commit -m "feat: add chat-about-this-plan state, overlay, prompt builder, and spawn logic"
```

---

### Task 2: Wire Context Menu Item and Integration Test

**Files:**
- Modify: `app/app_actions.go` (add `"chat about this"` to `openPlanContextMenu()`)
- Modify: `app/app_test.go` (integration test for full flow)

**Step 1: write the failing test**

```go
func TestChatAboutPlan_AppearsInContextMenu(t *testing.T) {
	h := newTestHome()
	h.setupPlanState("test-plan.md", planstate.StatusImplementing, "")

	h.focusSlot = slotNav
	h.nav.SelectByID(ui.SidebarPlanPrefix + "test-plan.md")

	model, _ := h.openPlanContextMenu()
	updated := model.(*home)

	require.Equal(t, stateContextMenu, updated.state)
	require.NotNil(t, updated.contextMenu)

	// Verify "chat about this" appears in the menu items
	found := false
	for _, item := range updated.contextMenu.Items() {
		if item.Action == "chat_about_plan" {
			found = true
			break
		}
	}
	require.True(t, found, "context menu must include 'chat about this' action")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestChatAboutPlan_AppearsInContextMenu -v -count=1
```

expected: FAIL — `Items()` method may not exist, or `"chat_about_plan"` not in menu

**Step 3: write minimal implementation**

1. In `app/app_actions.go` `openPlanContextMenu()`, add the "chat about this" item. Insert it near the top of the common items block (after the lifecycle actions, before "view plan"):
   ```go
   items = append(items,
       overlay.ContextMenuItem{Label: "chat about this", Action: "chat_about_plan"},
       overlay.ContextMenuItem{Label: "view plan", Action: "view_plan"},
       // ... rest of items
   )
   ```

2. If `ContextMenu.Items()` accessor doesn't exist, add it to `ui/overlay/contextMenu.go`:
   ```go
   func (c *ContextMenu) Items() []ContextMenuItem {
       return c.items
   }
   ```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestChatAboutPlan -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_actions.go app/app_test.go ui/overlay/contextMenu.go
git commit -m "feat: add 'chat about this' to plan context menu"
```
