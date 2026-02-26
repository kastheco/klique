# Review Approval Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** After automated review completes (reviewer writes `review-approved` signal or its tmux dies), block auto-transition to `done` and require manual user approval via popups before the plan can be marked done.

**Architecture:** Two trigger paths with distinct popups. Path A (sentinel): show "review approved" confirmation → focus on reviewer → merge/PR choice. Path B (reviewer death): show "approve?" with y/n/esc — y enters Path A, n respawns reviewer, esc dismisses. `pendingApprovals map[string]bool` tracks plans awaiting merge/PR. Dedicated `pendingApprovalPRAction` avoids state collision with wave orchestration. Context menu offers merge/PR as alternate path for `reviewing` plans.

**Tech Stack:** Go 1.24+, bubbletea v1.3.x, lipgloss v1.1.x, testify

---

## Wave 1: State Infrastructure

### Task 1: Add `pendingApprovals` and `pendingApprovalPRAction` fields to home struct

**Files:**
- Modify: `app/app.go` (add fields after `pendingReviewFeedback`, initialize in `newHome()`)

**Step 1: Add the fields to the home struct**

In `app/app.go`, after line 235 (`pendingReviewFeedback map[string]string`), add:

```go
	// pendingApprovals tracks plans whose automated review approved but the user
	// hasn't yet confirmed merge/PR. Keyed by plan filename. In-memory only —
	// on restart, reviewer-death fallback re-triggers the approval popup.
	pendingApprovals map[string]bool

	// pendingApprovalPRAction stores the "create PR" action for the post-review
	// merge/PR popup (Popup 2). Isolated from pendingWaveNextAction to avoid
	// state collision with wave orchestration dialogs.
	pendingApprovalPRAction tea.Cmd
```

**Step 2: Initialize in `newHome()`**

In `app/app.go`, after line 275 (`pendingReviewFeedback: make(map[string]string),`), add:

```go
		pendingApprovals: make(map[string]bool),
```

(`pendingApprovalPRAction` is nil by default, no init needed.)

**Step 3: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat(approval): add pendingApprovals and pendingApprovalPRAction fields"
```

### Task 2: Add `pendingApprovals` to test helpers

**Files:**
- Modify: `app/app_plan_completion_test.go` (all `&home{...}` literals)

**Step 1: Add field to all test `home` struct literals**

Each test helper that constructs a `&home{...}` literal needs `pendingApprovals: make(map[string]bool)`. Add it next to the existing `pendingReviewFeedback` line in each. There are three locations — find `pendingReviewFeedback: make(map[string]string),` in each:

1. `TestMetadataResultMsg_SignalDoesNotClobberFreshPlanState` (~line 283)
2. `TestImplementFinishedSignal_SpawnsReviewer` (~line 359)
3. `TestReviewChangesSignal_RespawnsCoder` (~line 440)

**Step 2: Verify tests compile**

Run: `go test ./app/... -run TestMetadata -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add app/app_plan_completion_test.go
git commit -m "fix(test): add pendingApprovals to test home struct literals"
```

### Task 3: Define new msg types

**Files:**
- Modify: `app/app.go` (add near other msg types around line 1206)

**Step 1: Add msg types**

```go
// reviewApprovalFocusMsg is sent when the user confirms "read review" in the
// review-approved popup. Selects the reviewer instance and enters focus mode.
type reviewApprovalFocusMsg struct {
	planFile string
}

// reviewMergeMsg is sent when the user chooses "merge" in the post-review popup.
type reviewMergeMsg struct {
	planFile string
}

// reviewCreatePRMsg is sent when the user chooses "create PR" in the post-review popup.
type reviewCreatePRMsg struct {
	planFile string
}

// reviewerRedoMsg is sent when the user rejects a reviewer-death approval (presses 'n').
// Respawns the reviewer for the given plan.
type reviewerRedoMsg struct {
	planFile string
}
```

**Step 2: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add app/app.go
git commit -m "feat(approval): define review approval msg types"
```

## Wave 2: Intercept ReviewApproved — Path A (Sentinel)

### Task 4: Write tests for sentinel-path approval interception

**Files:**
- Modify: `app/app_plan_completion_test.go` (append new tests)

**Step 1: Write test for signal-path interception**

Append to `app/app_plan_completion_test.go`:

```go
// TestReviewApprovedSignal_SetsPendingApproval verifies that when a
// review-approved sentinel is processed, the plan does NOT transition to done.
// Instead, pendingApprovals is set and the confirmation overlay appears.
func TestReviewApprovedSignal_SetsPendingApproval(t *testing.T) {
	const planFile = "2026-02-23-feature.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "feature", "plan/feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusReviewing)

	// Create a reviewer instance bound to this plan.
	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "feature-review",
		Path:      dir,
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeReviewer,
	})
	require.NoError(t, err)
	reviewerInst.IsReviewer = true

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(reviewerInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		list:                  list,
		menu:                  ui.NewMenu(),
		sidebar:               ui.NewSidebar(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:          overlay.NewToastManager(&sp),
		planState:             ps,
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		pendingReviewFeedback: make(map[string]string),
		pendingApprovals:      make(map[string]bool),
		plannerPrompted:       make(map[string]bool),
		activeRepoPath:        dir,
		program:               "claude",
	}

	signal := planfsm.Signal{
		Event:    planfsm.ReviewApproved,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	_, _ = h.Update(msg)

	// pendingApprovals must be set.
	assert.True(t, h.pendingApprovals[planFile],
		"review-approved signal must set pendingApprovals instead of transitioning to done")

	// Plan status must still be "reviewing" — NOT "done".
	reloaded, _ := planstate.Load(plansDir)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReviewing, entry.Status,
		"plan must stay in reviewing until user manually approves")

	// Confirmation overlay must be shown.
	assert.Equal(t, stateConfirm, h.state,
		"confirmation overlay must be shown for review approval")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/... -run TestReviewApprovedSignal_SetsPendingApproval -count=1 -v`
Expected: FAIL — the signal currently calls `fsm.Transition` and transitions to `done`.

**Step 3: Commit failing test**

```bash
git add app/app_plan_completion_test.go
git commit -m "test(approval): add failing test for review-approved interception"
```

### Task 5: Intercept `ReviewApproved` signal in signal handler

**Files:**
- Modify: `app/app.go` (~line 546, the signal processing loop in `metadataResultMsg` handler)

**Step 1: Add `ReviewApproved` intercept before `fsm.Transition`**

In `app/app.go`, inside the signal loop (`for _, sig := range msg.Signals {`), add a `ReviewApproved` interception *before* the `fsm.Transition` call at line 548. Insert immediately after the `for` line:

```go
			// Intercept ReviewApproved: block FSM transition, show approval popup instead.
			if sig.Event == planfsm.ReviewApproved {
				planfsm.ConsumeSignal(sig)
				m.pendingApprovals[sig.PlanFile] = true
				if m.state != stateConfirm {
					planName := planstate.DisplayName(sig.PlanFile)
					capturedPlanFile := sig.PlanFile
					m.confirmAction(
						fmt.Sprintf("review approved — %s\n\n[y] read review  [esc] dismiss", planName),
						func() tea.Msg {
							return reviewApprovalFocusMsg{planFile: capturedPlanFile}
						},
					)
				}
				continue
			}
```

**Step 2: Handle `reviewApprovalFocusMsg` in Update**

Add a case in the `Update` function's type switch (near other msg handlers):

```go
	case reviewApprovalFocusMsg:
		// Select the reviewer instance for this plan and enter focus mode.
		for _, inst := range m.list.GetInstances() {
			if inst.PlanFile == msg.planFile && inst.IsReviewer {
				m.list.SelectInstance(inst)
				return m, m.enterFocusMode()
			}
		}
		// Reviewer instance not found — show toast and leave pending for context menu path.
		m.toastManager.Info(fmt.Sprintf("reviewer session not found for %s — use context menu to merge or create pr", planstate.DisplayName(msg.planFile)))
		return m, m.toastTickCmd()
```

**Step 3: Run the test**

Run: `go test ./app/... -run TestReviewApprovedSignal_SetsPendingApproval -count=1 -v`
Expected: PASS

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat(approval): intercept review-approved signal with approval popup"
```

## Wave 3: Intercept Reviewer Death — Path B

### Task 6: Write test for reviewer-death approval popup

**Files:**
- Modify: `app/app_plan_completion_test.go` (append new test)

**Step 1: Write test**

Append to `app/app_plan_completion_test.go`:

```go
// TestReviewerDeath_ShowsApprovalPopup verifies that when a reviewer's tmux
// session dies while the plan is in reviewing state, a y/n/esc approval popup
// is shown instead of auto-transitioning to done.
func TestReviewerDeath_ShowsApprovalPopup(t *testing.T) {
	const planFile = "2026-02-23-feature.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "feature", "plan/feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusReviewing)

	// Create a started reviewer instance (tmux will report dead).
	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "feature-review",
		Path:      dir,
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeReviewer,
	})
	require.NoError(t, err)
	reviewerInst.IsReviewer = true
	reviewerInst.SetStatus(session.Running) // mark as started

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(reviewerInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		list:                  list,
		menu:                  ui.NewMenu(),
		sidebar:               ui.NewSidebar(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:          overlay.NewToastManager(&sp),
		planState:             ps,
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		pendingReviewFeedback: make(map[string]string),
		pendingApprovals:      make(map[string]bool),
		plannerPrompted:       make(map[string]bool),
		activeRepoPath:        dir,
		program:               "claude",
	}

	// Simulate metadata tick where reviewer's tmux is dead.
	msg := metadataResultMsg{
		PlanState: ps,
		Results: []metadataResult{
			{Title: "feature-review", TmuxAlive: false, ContentCaptured: true},
		},
	}

	_, _ = h.Update(msg)

	// Plan status must still be "reviewing" — NOT "done".
	reloaded, _ := planstate.Load(plansDir)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReviewing, entry.Status,
		"plan must stay in reviewing after reviewer death until user approves")

	// Confirmation overlay must be shown (y/n/esc).
	assert.Equal(t, stateConfirm, h.state,
		"confirmation overlay must be shown for reviewer death")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/... -run TestReviewerDeath_ShowsApprovalPopup -count=1 -v`
Expected: FAIL — reviewer death currently calls `fsm.Transition(ReviewApproved)`.

**Step 3: Commit failing test**

```bash
git add app/app_plan_completion_test.go
git commit -m "test(approval): add failing test for reviewer-death approval popup"
```

### Task 7: Replace reviewer-death auto-approve with y/n/esc popup

**Files:**
- Modify: `app/app.go` (~line 688, the reviewer death block)

**Step 1: Replace the reviewer-death auto-approve**

In `app/app.go`, replace lines 688-691:

```go
			// Reviewer death → ReviewApproved: one-shot FSM transition, rare event.
			if err := m.fsm.Transition(inst.PlanFile, planfsm.ReviewApproved); err != nil {
				log.WarningLog.Printf("could not mark plan %q completed: %v", inst.PlanFile, err)
			}
```

With:

```go
			// Reviewer death → approval popup (manual gate).
			// Don't auto-transition to done — show y/n/esc popup so the user
			// can approve, reject (redo), or dismiss.
			if !m.pendingApprovals[inst.PlanFile] {
				planName := planstate.DisplayName(inst.PlanFile)
				if m.state != stateConfirm {
					capturedPlanFile := inst.PlanFile
					m.state = stateConfirm
					m.confirmationOverlay = overlay.NewConfirmationOverlay(
						fmt.Sprintf("reviewer exited — approve '%s'?\n\n[y] approve  [n] reject (redo review)  [esc] dismiss", planName),
					)
					m.confirmationOverlay.SetWidth(60)
					m.pendingConfirmAction = func() tea.Msg {
						return reviewApprovalFocusMsg{planFile: capturedPlanFile}
					}
					m.pendingWaveNextAction = nil
					m.pendingWaveAbortAction = nil
					// Store reject action: 'n' key triggers CancelKey handler.
					m.confirmationOverlay.CancelKey = "n"
					// We need a dedicated field for the reject action.
					// Reuse pendingConfirmAction for 'y' and handle 'n' as a reviewerRedoMsg.
					capturedPlanFileForReject := inst.PlanFile
					m.pendingApprovalPRAction = func() tea.Msg {
						return reviewerRedoMsg{planFile: capturedPlanFileForReject}
					}
				}
			}
```

**Step 2: Handle `reviewerRedoMsg` in Update**

Add a case in the `Update` type switch:

```go
	case reviewerRedoMsg:
		// User rejected the reviewer-death approval — respawn the reviewer.
		if cmd := m.spawnReviewer(msg.planFile); cmd != nil {
			m.toastManager.Info(fmt.Sprintf("restarting review for %s", planstate.DisplayName(msg.planFile)))
			return m, tea.Batch(m.toastTickCmd(), cmd)
		}
		return m, nil
```

**Step 3: Update `stateConfirm` CancelKey handler for reviewer-death popup**

In `app/app_input.go`, in the `case m.confirmationOverlay.CancelKey:` block (~line 589), after the existing `pendingWaveNextAction` check, add a check for the reviewer-death reject:

```go
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
			// If this is the reviewer-death approval dialog, 'n' triggers reject (redo).
			if m.pendingApprovalPRAction != nil {
				rejectAction := m.pendingApprovalPRAction
				m.state = stateDefault
				m.confirmationOverlay = nil
				m.pendingConfirmAction = nil
				m.pendingApprovalPRAction = nil
				return m, rejectAction
			}
```

The rest of the CancelKey handler remains unchanged.

**Step 4: Run the test**

Run: `go test ./app/... -run TestReviewerDeath_ShowsApprovalPopup -count=1 -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./app/... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add app/app.go app/app_input.go app/app_plan_completion_test.go
git commit -m "feat(approval): replace reviewer-death auto-approve with y/n/esc popup"
```

## Wave 4: Popup 2 — Post-Focus Merge/PR Choice

### Task 8: Add `reviewApprovalConfirmAction` helper

**Files:**
- Modify: `app/app_input.go` (add helper after `waveFailedConfirmAction` ~line 1370)

**Step 1: Add helper function**

```go
// reviewApprovalConfirmAction shows a three-choice dialog for a plan with
// a pending review approval. Keys: m=merge to main, p=create PR, esc=dismiss.
func (m *home) reviewApprovalConfirmAction(planFile string) {
	planName := planstate.DisplayName(planFile)

	m.state = stateConfirm
	m.confirmationOverlay = overlay.NewConfirmationOverlay(
		fmt.Sprintf("merge to main or create pr for '%s'?\n\n[m] merge  [p] create pr  [esc] dismiss", planName),
	)
	m.confirmationOverlay.ConfirmKey = "m"
	m.confirmationOverlay.CancelKey = "p"
	m.confirmationOverlay.SetWidth(60)

	capturedPlanFile := planFile
	m.pendingConfirmAction = func() tea.Msg {
		return reviewMergeMsg{planFile: capturedPlanFile}
	}
	// Use dedicated field to avoid collision with wave orchestration.
	m.pendingApprovalPRAction = func() tea.Msg {
		return reviewCreatePRMsg{planFile: capturedPlanFile}
	}
	m.pendingWaveNextAction = nil
	m.pendingWaveAbortAction = nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "feat(approval): add review approval confirm helper"
```

### Task 9: Show Popup 2 when exiting focus mode from a reviewer with pending approval

**Files:**
- Modify: `app/app_input.go` (~line 479, Ctrl+Space exit path; ~line 495, jump-slot exit path)

**Step 1: Modify Ctrl+Space focus mode exit**

Replace lines 479-481:

```go
		if msg.Type == tea.KeyCtrlAt {
			m.exitFocusMode()
			return m, tea.WindowSize()
		}
```

With:

```go
		if msg.Type == tea.KeyCtrlAt {
			selected := m.list.GetSelectedInstance()
			m.exitFocusMode()
			if selected != nil && selected.IsReviewer && selected.PlanFile != "" && m.pendingApprovals[selected.PlanFile] {
				m.reviewApprovalConfirmAction(selected.PlanFile)
				return m, nil
			}
			return m, tea.WindowSize()
		}
```

**Step 2: Modify jump-slot exit path**

Replace lines 495-498:

```go
		if doJump {
			m.exitFocusMode()
			m.setFocusSlot(jumpSlot)
			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		}
```

With:

```go
		if doJump {
			selected := m.list.GetSelectedInstance()
			m.exitFocusMode()
			if selected != nil && selected.IsReviewer && selected.PlanFile != "" && m.pendingApprovals[selected.PlanFile] {
				m.reviewApprovalConfirmAction(selected.PlanFile)
				return m, nil
			}
			m.setFocusSlot(jumpSlot)
			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		}
```

**Step 3: Update CancelKey handler in stateConfirm for Popup 2**

The CancelKey `p` in Popup 2 needs to fire `pendingApprovalPRAction`. In `app/app_input.go`, the CancelKey handler (line 589) already checks `pendingWaveNextAction` first. The `pendingApprovalPRAction` check added in Task 7 will also handle this — when CancelKey is `p` and `pendingApprovalPRAction` is set, it fires the PR action. This is correct for both reviewer-death (Task 7) and Popup 2 (this task).

**Step 4: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 5: Commit**

```bash
git add app/app_input.go
git commit -m "feat(approval): show merge/pr popup when exiting focus mode from approved reviewer"
```

### Task 10: Handle `reviewMergeMsg` and `reviewCreatePRMsg` in Update

**Files:**
- Modify: `app/app.go` (add cases to `Update` type switch)

**Step 1: Handle `reviewMergeMsg`**

Add to the type switch in `Update()`:

```go
	case reviewMergeMsg:
		planFile := msg.planFile
		delete(m.pendingApprovals, planFile)
		if m.planState == nil {
			return m, m.handleError(fmt.Errorf("no plan state loaded"))
		}
		entry, ok := m.planState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
		}
		branch := entry.Branch
		if branch == "" {
			branch = gitpkg.PlanBranchFromFile(planFile)
		}
		planName := planstate.DisplayName(planFile)
		m.toastManager.Loading(fmt.Sprintf("merging '%s' to main...", planName))
		capturedPlanFile := planFile
		capturedBranch := branch
		return m, tea.Batch(m.toastTickCmd(), func() tea.Msg {
			// Kill all instances bound to this plan.
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].PlanFile == capturedPlanFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.MergePlanBranch(m.activeRepoPath, capturedBranch); err != nil {
				return err
			}
			if err := m.fsm.Transition(capturedPlanFile, planfsm.ReviewApproved); err != nil {
				return err
			}
			_ = m.saveAllInstances()
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateSidebarItems()
			return planRefreshMsg{}
		})
```

**Step 2: Handle `reviewCreatePRMsg`**

```go
	case reviewCreatePRMsg:
		planFile := msg.planFile
		delete(m.pendingApprovals, planFile)
		m.pendingApprovalPRAction = nil
		// Find an instance for this plan so the PR flow can use it.
		var planInst *session.Instance
		for _, inst := range m.list.GetInstances() {
			if inst.PlanFile == planFile {
				planInst = inst
				break
			}
		}
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for plan %s", planFile))
		}
		// Select the instance and enter the PR title flow.
		m.list.SelectInstance(planInst)
		planName := planstate.DisplayName(planFile)
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("pr title", planName)
		m.textInputOverlay.SetSize(60, 3)
		// Mark the plan as done via FSM so it moves to history after PR creation.
		if err := m.fsm.Transition(planFile, planfsm.ReviewApproved); err != nil {
			log.WarningLog.Printf("could not mark plan %q done after PR: %v", planFile, err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, nil
```

**Step 3: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 4: Run full test suite**

Run: `go test ./app/... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app.go
git commit -m "feat(approval): handle merge and create-pr messages from approval popup"
```

## Wave 5: Context Menu Alternate Path

### Task 11: Add merge/PR options to reviewing plan context menu

**Files:**
- Modify: `app/app_actions.go` (~line 552, plan context menu for `StatusReviewing`)

**Step 1: Update the reviewing context menu**

Replace lines 552-556:

```go
		case planstate.StatusReviewing:
			items = append(items,
				overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				overlay.ContextMenuItem{Label: "mark finished", Action: "mark_plan_done"},
			)
```

With:

```go
		case planstate.StatusReviewing:
			items = append(items,
				overlay.ContextMenuItem{Label: "start review", Action: "start_review"},
				overlay.ContextMenuItem{Label: "review & merge", Action: "review_merge"},
				overlay.ContextMenuItem{Label: "create pr & push", Action: "review_create_pr"},
				overlay.ContextMenuItem{Label: "mark finished", Action: "mark_plan_done"},
			)
```

**Step 2: Add action handlers for `review_merge` and `review_create_pr`**

In `app/app_actions.go`, in the `executeContextAction` switch, add two new cases (near the existing `merge_plan` case around line 240):

```go
	case "review_merge":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		entry, ok := m.planState.Entry(planFile)
		if !ok {
			return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
		}
		branch := entry.Branch
		if branch == "" {
			branch = gitpkg.PlanBranchFromFile(planFile)
		}
		delete(m.pendingApprovals, planFile)
		planName := planstate.DisplayName(planFile)
		capturedPlanFile := planFile
		capturedBranch := branch
		mergeAction := func() tea.Msg {
			for i := len(m.allInstances) - 1; i >= 0; i-- {
				if m.allInstances[i].PlanFile == capturedPlanFile {
					_ = m.allInstances[i].Kill()
					m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
				}
			}
			if err := gitpkg.MergePlanBranch(m.activeRepoPath, capturedBranch); err != nil {
				return err
			}
			if err := m.fsm.Transition(capturedPlanFile, planfsm.ReviewApproved); err != nil {
				return err
			}
			_ = m.saveAllInstances()
			m.loadPlanState()
			m.updateSidebarPlans()
			m.updateSidebarItems()
			return planRefreshMsg{}
		}
		return m, m.confirmAction(fmt.Sprintf("merge '%s' branch into main?", planName), mergeAction)

	case "review_create_pr":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		delete(m.pendingApprovals, planFile)
		// Find an instance for this plan so the PR flow can find it.
		var planInst *session.Instance
		for _, inst := range m.list.GetInstances() {
			if inst.PlanFile == planFile {
				planInst = inst
				break
			}
		}
		if planInst == nil {
			return m, m.handleError(fmt.Errorf("no active session for this plan"))
		}
		m.list.SelectInstance(planInst)
		planName := planstate.DisplayName(planFile)
		m.state = statePRTitle
		m.textInputOverlay = overlay.NewTextInputOverlay("pr title", planName)
		m.textInputOverlay.SetSize(60, 3)
		// Transition plan to done.
		if err := m.fsm.Transition(planFile, planfsm.ReviewApproved); err != nil {
			log.WarningLog.Printf("could not mark plan %q done: %v", planFile, err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, nil
```

**Step 3: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 4: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app_actions.go
git commit -m "feat(approval): add review & merge / create pr context menu options"
```

## Wave 6: Edge Cases and Cleanup

### Task 12: Clear pending approval on plan cancel/start-over/mark-done

**Files:**
- Modify: `app/app_actions.go` (cancel_plan, start_over_plan, mark_plan_done handlers)

**Step 1: Add cleanup to `mark_plan_done` handler**

In the `mark_plan_done` case (~line 275), add `delete(m.pendingApprovals, planFile)` after getting the planFile:

```go
	case "mark_plan_done":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		delete(m.pendingApprovals, planFile)
		// Solo agents finish at "implementing" — advance through reviewing in one step.
```

**Step 2: Add cleanup to `cancel_plan` handler**

In the `cancel_plan` case (~line 307), add `delete(m.pendingApprovals, planFile)` after getting the planFile:

```go
	case "cancel_plan":
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile == "" || m.planState == nil {
			return m, nil
		}
		delete(m.pendingApprovals, planFile)
		planName := planstate.DisplayName(planFile)
```

**Step 3: Add cleanup to `start_over_plan` handler**

In the `start_over_plan` case (~line 337), add `delete(m.pendingApprovals, planFile)` after getting the planFile.

**Step 4: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add app/app_actions.go
git commit -m "fix(approval): clear pending approvals on cancel/start-over/mark-done"
```

### Task 13: Clear `pendingApprovalPRAction` in esc handler

**Files:**
- Modify: `app/app_input.go` (~line 641, esc handler in stateConfirm)

**Step 1: Add cleanup**

In the esc handler block, after line 644 (`m.pendingWaveAbortAction = nil`), add:

```go
			m.pendingApprovalPRAction = nil
```

Also add it in the ConfirmKey handler cleanup (~line 570) and the `a` abort handler cleanup (~line 585).

**Step 2: Verify it compiles**

Run: `go build ./app/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "fix(approval): clear pendingApprovalPRAction in all stateConfirm exit paths"
```

### Task 14: Final verification

**Step 1: Build the full binary**

Run: `go build ./cmd/...`
Expected: SUCCESS

**Step 2: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 3: Run typos check**

Run: `typos app/`
Expected: No new typos

**Step 4: Manual smoke test (if running locally)**

1. Start kasmos
2. Pick a plan in `reviewing` state
3. Verify context menu shows "review & merge" and "create pr & push"
4. If a reviewer is running: wait for it to die
5. Verify popup appears: "reviewer exited — approve '{plan}'?" with y/n/esc
6. Press `y` → should enter focus mode on reviewer
7. Exit focus mode (Ctrl+Space) → Popup 2 should appear with m/p/esc choices
8. Press `m` → merge completes, plan moves to done
9. For sentinel path: wait for reviewer to write `review-approved`
10. Verify popup: "review approved — {plan}" with y/esc
11. Press `y` → focus mode → exit → merge/PR popup
