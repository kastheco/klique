# Safe Kill Implementation Plan

**Goal:** Make kill safe by preserving the git branch and changing all user-facing kill actions (TUI key, context menu, CLI) to pause the instance instead of destroying it, preventing accidental data loss.

**Architecture:** Currently `Kill()` calls `Cleanup()` which deletes both the worktree directory and the git branch. The fix has two parts: (1) change `Kill()` to use `Remove()` + `Prune()` instead of `Cleanup()` so the branch is always preserved even for internal flows (merge, wave abort, failure cleanup), and (2) change all user-facing kill entry points (TUI `K` key, context menu "kill", CLI `kas instance kill`) to call `Pause()` instead, keeping the instance in the list as resumable. The existing `Pause()` method already auto-commits dirty changes, removes the worktree, and preserves the branch — it's exactly the safe behavior we want.

**Tech Stack:** Go, bubbletea TUI framework, cobra CLI, git worktrees

**Size:** Small (estimated ~1.5 hours, 2 tasks, 1 wave)

---

## Wave 1: Safe Kill

### Task 1: Preserve Branch in Kill() Method

**Files:**
- Modify: `session/instance_lifecycle.go`
- Test: `session/instance_lifecycle_test.go`

**Step 1: write the failing test**

Add a test that verifies `Kill()` preserves the git branch. Create a real git repo + worktree, call `Kill()`, then assert the branch still exists:

```go
func TestKill_PreservesBranch(t *testing.T) {
    repoPath := setupGitRepo(t)
    inst, err := NewInstance(InstanceOptions{
        Title:   "test-safe-kill",
        Path:    repoPath,
        Program: "opencode",
    })
    require.NoError(t, err)

    cmdExec := cmd_test.MockCmdExec{
        RunFunc:    func(cmd *exec.Cmd) error { return nil },
        OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
    }
    inst.tmuxSession = tmux.NewTmuxSessionWithDeps(inst.Title, inst.Program, false, &testPtyFactory{}, cmdExec)

    err = inst.StartOnBranch("safe-kill-branch")
    require.NoError(t, err)

    branchName := inst.Branch
    require.NotEmpty(t, branchName)

    err = inst.Kill()
    require.NoError(t, err)

    // Branch must still exist after kill
    out, gitErr := exec.Command("git", "-C", repoPath, "branch", "--list", branchName).CombinedOutput()
    require.NoError(t, gitErr)
    assert.Contains(t, string(out), branchName, "branch should be preserved after Kill()")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/... -run TestKill_PreservesBranch -v
```

expected: FAIL — `Kill()` calls `Cleanup()` which deletes the branch.

**Step 3: write minimal implementation**

In `session/instance_lifecycle.go`, change `Kill()` to replace `gitWorktree.Cleanup()` with `gitWorktree.Remove()` + `gitWorktree.Prune()`:

```go
// In Kill(), replace:
//   if err := i.gitWorktree.Cleanup(); err != nil {
//       errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
//   }
// With:
if err := i.gitWorktree.Remove(); err != nil {
    errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
}
if err := i.gitWorktree.Prune(); err != nil {
    errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
}
```

Also update the Kill() doc comment to reflect the new behavior: branch is preserved.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run TestKill_PreservesBranch -v
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_lifecycle.go session/instance_lifecycle_test.go
git commit -m "fix: preserve git branch when killing an instance"
```

### Task 2: TUI and CLI Kill Actions Become Pause

**Files:**
- Modify: `app/app_actions.go`
- Modify: `app/app_input.go`
- Modify: `app/app.go`
- Modify: `app/help.go`
- Modify: `cmd/instance.go`
- Test: `cmd/instance_test.go`

**Step 1: write the failing test**

Add a CLI test that verifies `kas instance kill` now sets the instance to paused instead of removing it from state:

```go
func TestKillCmd_SetsInstanceToPaused(t *testing.T) {
    rec := fullInstanceRecord("kill-target")
    rec.Status = instanceRunning
    other := fullInstanceRecord("other")
    state := newTestStateFromRecords(t, []instanceRecord{rec, other})

    // Simulate what the new kill logic should do: update to paused, not remove.
    err := updateInstanceInState(state, "kill-target", func(r *instanceRecord) error {
        r.Status = instancePaused
        r.Worktree.WorktreePath = ""
        return nil
    })
    require.NoError(t, err)

    records, err := loadInstanceRecords(state)
    require.NoError(t, err)
    require.Len(t, records, 2, "kill should NOT remove instance from state")

    var target instanceRecord
    for _, r := range records {
        if r.Title == "kill-target" {
            target = r
        }
    }
    assert.Equal(t, instancePaused, target.Status, "killed instance should be paused")
    assert.Empty(t, target.Worktree.WorktreePath, "worktree path should be cleared")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestKillCmd_SetsInstanceToPaused -v
```

expected: PASS immediately (this tests the state update helper, which already works). The real behavioral test is that the kill command handler uses this path instead of `removeInstanceFromState`.

**Step 3: write minimal implementation**

**3a. `cmd/instance.go` — CLI kill becomes pause:**

Replace the kill command's `RunE` body. Instead of removing the instance from state, auto-commit dirty changes, kill tmux, remove worktree (not branch), and set status to paused:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    title := args[0]
    state := config.LoadState()
    records, err := loadInstanceRecords(state)
    if err != nil { return err }
    rec, err := findInstanceData(records, title)
    if err != nil { return err }
    if err := validateStatusForAction(rec, "kill"); err != nil { return err }
    // Auto-commit dirty changes before removing worktree.
    if rec.Worktree.WorktreePath != "" {
        commitMsg := fmt.Sprintf("[kas] auto-save from '%s' on %s (killed)",
            rec.Title, time.Now().Format(time.RFC822))
        _ = exec.Command("git", "-C", rec.Worktree.WorktreePath, "add", "-A").Run()
        _ = exec.Command("git", "-C", rec.Worktree.WorktreePath,
            "commit", "-m", commitMsg, "--allow-empty").Run()
    }
    // Kill tmux session (best-effort).
    _ = exec.Command("tmux", "kill-session", "-t", kasTmuxName(rec.Title)).Run()
    // Remove worktree but preserve branch.
    if rec.Worktree.WorktreePath != "" && rec.Worktree.RepoPath != "" {
        _ = exec.Command("git", "-C", rec.Worktree.RepoPath,
            "worktree", "remove", "--force", rec.Worktree.WorktreePath).Run()
        _ = exec.Command("git", "-C", rec.Worktree.RepoPath,
            "worktree", "prune").Run()
    }
    // Set to paused instead of removing from state.
    if err := updateInstanceInState(state, rec.Title, func(r *instanceRecord) error {
        r.Status = instancePaused
        r.Worktree.WorktreePath = ""
        return nil
    }); err != nil { return err }
    fmt.Printf("paused: %s (branch preserved)\n", rec.Title)
    return nil
},
```

**3b. `app/app_actions.go` — TUI context menu "kill" becomes pause:**

Replace the `kill_instance` case to call `selected.Pause()` instead of `nav.Kill()` + `removeFromAllInstances()`:

```go
case "kill_instance":
    selected := m.nav.GetSelectedInstance()
    if selected != nil {
        if err := selected.Pause(); err != nil {
            return m, m.handleError(err)
        }
        m.audit(auditlog.EventAgentKilled, "agent stopped (branch preserved)",
            auditlog.WithInstance(selected.Title),
            auditlog.WithAgent(selected.AgentType),
            auditlog.WithPlan(selected.TaskFile),
        )
        m.saveAllInstances()
        m.updateNavPanelStatus()
    }
    return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged())
```

**3c. `app/app_input.go` — Update K key confirmation message:**

Change:
```go
message := fmt.Sprintf("[!] abort session '%s'? this removes the worktree.", selected.Title)
```
To:
```go
message := fmt.Sprintf("stop session '%s'? branch will be preserved.", selected.Title)
```

**3d. `app/app.go` — `killInstanceMsg` handler becomes pause:**

Replace the handler to pause the instance instead of destroying it:

```go
case killInstanceMsg:
    for _, inst := range m.allInstances {
        if inst.Title == msg.title {
            if err := inst.Pause(); err != nil {
                return m, m.handleError(err)
            }
            break
        }
    }
    m.saveAllInstances()
    m.updateNavPanelStatus()
    return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged())
```

**3e. `app/help.go` — Update help text (2 occurrences):**

Change K key descriptions from:
```
K — abort session (removes worktree)
```
To:
```
K — stop session (branch preserved)
```

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v
go test ./app/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_actions.go app/app_input.go app/app.go app/help.go cmd/instance.go cmd/instance_test.go
git commit -m "feat: safe kill — kill now pauses instance and preserves branch"
```
