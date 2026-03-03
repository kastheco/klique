# Async Metadata Tick — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move all subprocess I/O out of the bubbletea `Update()` loop so that metadata polling never blocks the UI, fixing 1-10 second input/render latency.

**Architecture:** The current `tickUpdateMetadataMessage` handler runs 4-6 subprocess calls per instance (tmux capture-pane, git diff, pgrep, ps) synchronously inside `Update()`. With 5 instances this blocks the event loop for 250ms+, starving queued messages (key presses, plan renders, etc.). The fix: collect all per-instance data in a `tea.Cmd` goroutine, return a `metadataResultMsg` with the results, and apply them to the model in `Update()` with zero I/O. The same pattern applies to the 50ms `previewTickMsg` which also shells out to tmux.

**Tech Stack:** Go, bubbletea tea.Cmd, tmux, git

**Important — Current Codebase State:**

1. **`tickUpdateMetadataMessage`** runs every 500ms in `Update()` (`app/app.go:405-450`). For each started, non-paused instance it calls: `HasUpdated()` (tmux capture-pane), `GetPaneContent()` (tmux capture-pane again — redundant), `ParseActivity()`, `UpdateDiffStats()` (git add + git diff), `UpdateResourceUsage()` (tmux display-message + pgrep + ps). It also calls `loadPlanState()` (disk read), `checkReviewerCompletion()`, `updateSidebarPlans()`, `updateSidebarItems()`, and `checkPlanCompletion()`.
2. **`previewTickMsg`** runs every 50ms in `Update()` (`app/app.go:354-367`). It calls `instanceChanged()` which calls `UpdatePreview()` → `instance.Preview()` → tmux capture-pane.
3. **`HasUpdated()`** in `session/tmux/tmux_io.go:38` already captures pane content and hashes it. `GetPaneContent()` re-captures the same content — this is a redundant subprocess call that should be eliminated.
4. **`planRefreshMsg`** and **`killInstanceMsg`** patterns already exist in `app.go` as examples of the "do I/O in cmd, mutate model in Update" pattern.

---

### Task 1: Define the metadata result types

**Files:**
- Modify: `app/app.go`

**Step 1: Add the result message types after the existing message types (~line 639)**

Add these types after `planRefreshMsg`:

```go
// instanceMetadata holds the results of polling a single instance's subprocess data.
// Collected in a goroutine, applied to the model in Update.
type instanceMetadata struct {
	Title       string
	Content     string // tmux capture-pane output (reused for preview, activity, hash)
	Updated     bool
	HasPrompt   bool
	DiffStats   *git.DiffStats
	CPUPercent  float64
	MemMB       float64
}

// metadataResultMsg carries all per-instance metadata collected by the async tick.
type metadataResultMsg struct {
	Results []instanceMetadata
}
```

This requires adding `"github.com/kastheco/kasmos/session/git"` to the imports in `app.go`.

**Step 2: Run tests**

Run: `go build ./app/...`
Expected: PASS — these are just type definitions.

---

### Task 2: Add a method to collect metadata from a single instance without model mutation

**Files:**
- Modify: `session/instance_session.go`

The current code calls `HasUpdated()` (which internally calls `CapturePaneContent()`) and then `GetPaneContent()` (which calls `CapturePaneContent()` again). That's two subprocess calls for the same data. We need a single method that captures once, returns everything the metadata tick needs.

**Step 1: Add `CollectMetadata` to Instance**

Add to `session/instance_session.go`:

```go
// InstanceMetadata holds the results of polling a single instance.
// Collected in a goroutine — all fields are values, no pointers into the model.
type InstanceMetadata struct {
	Content    string // raw tmux capture-pane output
	Updated    bool
	HasPrompt  bool
	DiffStats  *git.DiffStats
	CPUPercent float64
	MemMB      float64
}

// CollectMetadata gathers all per-tick data for this instance via subprocess calls.
// Safe to call from a goroutine — reads only, no model mutations.
// Combines HasUpdated + GetPaneContent + UpdateDiffStats + UpdateResourceUsage
// into a single method, eliminating the redundant second capture-pane call.
func (i *Instance) CollectMetadata() InstanceMetadata {
	var m InstanceMetadata

	if !i.started || i.Status == Paused {
		return m
	}

	// Single capture-pane call — reused for hash check, activity parsing, and preview.
	m.Updated, m.HasPrompt, m.Content = i.tmuxSession.HasUpdatedWithContent()

	// Git diff stats
	if i.gitWorktree != nil {
		stats := i.gitWorktree.Diff()
		if stats.Error != nil {
			if !strings.Contains(stats.Error.Error(), "base commit SHA not set") &&
				!strings.Contains(stats.Error.Error(), "worktree path gone") {
				log.WarningLog.Printf("diff stats error: %v", stats.Error)
			}
			// On error, return nil stats (caller keeps previous)
		} else {
			m.DiffStats = stats
		}
	}

	// Resource usage (pgrep + ps)
	m.CPUPercent, m.MemMB = i.collectResourceUsage()

	return m
}
```

**Step 2: Extract resource usage collection into a pure function**

Refactor `UpdateResourceUsage` in `session/instance_session.go` to separate the collection (subprocess calls) from the mutation (writing to `i.CPUPercent`, `i.MemMB`):

```go
// collectResourceUsage queries CPU and memory usage via subprocess calls.
// Returns (cpu%, memMB). Safe to call from a goroutine.
func (i *Instance) collectResourceUsage() (float64, float64) {
	if !i.started || i.tmuxSession == nil {
		return 0, 0
	}

	pid, err := i.tmuxSession.GetPanePID()
	if err != nil {
		return i.CPUPercent, i.MemMB // keep previous on error
	}

	targetPid := strconv.Itoa(pid)
	childCmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	if childOutput, err := childCmd.Output(); err == nil {
		if children := strings.Fields(strings.TrimSpace(string(childOutput))); len(children) > 0 {
			targetPid = children[0]
		}
	}

	psCmd := exec.Command("ps", "-o", "%cpu=,rss=", "-p", targetPid)
	output, err := psCmd.Output()
	if err != nil {
		return i.CPUPercent, i.MemMB
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return i.CPUPercent, i.MemMB
	}

	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return i.CPUPercent, i.MemMB
	}
	rssKB, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return i.CPUPercent, i.MemMB
	}
	return cpu, rssKB / 1024
}

// UpdateResourceUsage queries the process tree for CPU and memory usage.
// Kept for backward compat but now delegates to collectResourceUsage.
func (i *Instance) UpdateResourceUsage() {
	i.CPUPercent, i.MemMB = i.collectResourceUsage()
}
```

**Step 3: Run tests**

Run: `go test ./session/... && go build ./...`
Expected: PASS

---

### Task 3: Add `HasUpdatedWithContent` to TmuxSession

**Files:**
- Modify: `session/tmux/tmux_io.go`

The current `HasUpdated()` calls `CapturePaneContent()` internally, hashes the result, then throws away the content. The caller then calls `GetPaneContent()` → `Preview()` → `CapturePaneContent()` again. We need a variant that returns the captured content alongside the update/prompt flags.

**Step 1: Add `HasUpdatedWithContent` method**

Add to `session/tmux/tmux_io.go`, right after `HasUpdated()`:

```go
// HasUpdatedWithContent is like HasUpdated but also returns the raw captured
// pane content, eliminating the need for a separate CapturePaneContent call.
func (t *TmuxSession) HasUpdatedWithContent() (updated bool, hasPrompt bool, content string) {
	raw, err := t.CapturePaneContent()
	if err != nil {
		t.monitor.captureFailures++
		if t.monitor.captureFailures == 1 || t.monitor.captureFailures%30 == 0 {
			log.ErrorLog.Printf("error capturing pane content in status monitor (failure #%d): %v",
				t.monitor.captureFailures, err)
		}
		return false, false, ""
	}
	t.monitor.captureFailures = 0

	content = raw

	switch {
	case isClaudeProgram(t.program):
		hasPrompt = strings.Contains(content, "No, and tell Claude what to do differently")
	case isAiderProgram(t.program):
		hasPrompt = strings.Contains(content, "(Y)es/(N)o/(D)on't ask again")
	case isGeminiProgram(t.program):
		hasPrompt = strings.Contains(content, "Yes, allow once")
	case isOpenCodeProgram(t.program):
		hasPrompt = strings.Contains(content, "Ask anything")
	}

	newHash := t.monitor.hash(content)
	if !bytes.Equal(newHash, t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = newHash
		t.monitor.unchangedTicks = 0
		return true, hasPrompt, content
	}

	t.monitor.unchangedTicks++
	if t.monitor.unchangedTicks < 6 {
		return true, hasPrompt, content
	}
	return false, hasPrompt, content
}
```

**Step 2: Run tests**

Run: `go test ./session/... && go build ./...`
Expected: PASS

---

### Task 4: Refactor `tickUpdateMetadataMessage` to async collection

**Files:**
- Modify: `app/app.go`

This is the core change. The `tickUpdateMetadataMessage` handler currently does all subprocess I/O inline. We split it into:
1. A `tea.Cmd` goroutine that calls `CollectMetadata()` on each instance and reads plan state from disk
2. A `metadataResultMsg` handler in `Update()` that applies the results to the model

**Step 1: Replace the `tickUpdateMetadataMessage` handler**

Replace the entire `case tickUpdateMetadataMessage:` block (lines ~405-450) with:

```go
	case tickUpdateMetadataMessage:
		// Snapshot the instance list for the goroutine. The slice header is
		// copied but the pointers are shared — CollectMetadata only reads
		// instance fields that don't change between ticks (started, Status,
		// tmuxSession, gitWorktree, Program).
		instances := m.list.GetInstances()
		snapshots := make([]*session.Instance, len(instances))
		copy(snapshots, instances)

		planDir := m.planStateDir

		return m, func() tea.Msg {
			results := make([]instanceMetadata, 0, len(snapshots))
			for _, inst := range snapshots {
				if !inst.Started() || inst.Paused() {
					continue
				}
				md := inst.CollectMetadata()
				results = append(results, instanceMetadata{
					Title:     inst.Title,
					Content:   md.Content,
					Updated:   md.Updated,
					HasPrompt: md.HasPrompt,
					DiffStats: md.DiffStats,
					CPUPercent: md.CPUPercent,
					MemMB:     md.MemMB,
				})
			}
			time.Sleep(500 * time.Millisecond)
			return metadataResultMsg{Results: results, PlanDir: planDir}
		}
```

Update the `metadataResultMsg` type to include `PlanDir`:

```go
type metadataResultMsg struct {
	Results []instanceMetadata
	PlanDir string
}
```

**Step 2: Add the `metadataResultMsg` handler**

Add a new case in `Update()`:

```go
	case metadataResultMsg:
		// Apply collected metadata to instances — zero I/O, just field writes.
		instanceMap := make(map[string]*session.Instance)
		for _, inst := range m.list.GetInstances() {
			instanceMap[inst.Title] = inst
		}

		for _, md := range msg.Results {
			inst, ok := instanceMap[md.Title]
			if !ok {
				continue
			}

			if md.Updated {
				inst.SetStatus(session.Running)
				if md.Content != "" {
					inst.LastActivity = session.ParseActivity(md.Content, inst.Program)
				}
			} else {
				if md.HasPrompt {
					inst.PromptDetected = true
					inst.TapEnter()
				} else {
					inst.SetStatus(session.Ready)
				}
				if inst.Status != session.Running {
					inst.LastActivity = nil
				}
			}

			// Deliver queued prompt
			if inst.QueuedPrompt != "" && (inst.Status == session.Ready || inst.PromptDetected) {
				if err := inst.SendPrompt(inst.QueuedPrompt); err != nil {
					log.WarningLog.Printf("could not send queued prompt to %q: %v", inst.Title, err)
				}
				inst.QueuedPrompt = ""
			}

			if md.DiffStats != nil {
				inst.SetDiffStats(md.DiffStats)
			}
			inst.CPUPercent = md.CPUPercent
			inst.MemMB = md.MemMB
		}

		// Clear activity for non-started / paused instances
		for _, inst := range m.list.GetInstances() {
			if !inst.Started() || inst.Paused() {
				inst.LastActivity = nil
			}
		}

		// Refresh plan state and sidebar (these are cheap — JSON parse + in-memory rebuild)
		m.loadPlanState()
		m.checkReviewerCompletion()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		completionCmd := m.checkPlanCompletion()
		return m, tea.Batch(tickUpdateMetadataCmd, completionCmd)
```

**Step 3: Add `SetDiffStats` to Instance**

Add to `session/instance_session.go`:

```go
// SetDiffStats sets the diff stats from externally collected data.
func (i *Instance) SetDiffStats(stats *git.DiffStats) {
	i.diffStats = stats
}
```

**Step 4: Run tests**

Run: `go test ./... && go build ./...`
Expected: PASS

**Step 5: Commit**

```
feat(app): move metadata tick subprocess I/O to async tea.Cmd

The tickUpdateMetadataMessage handler was running 4-6 subprocess calls
per instance (tmux capture-pane ×2, git diff, pgrep, ps) synchronously
inside Update(), blocking the event loop for 250ms+ with 5 instances.

Refactored to collect all per-instance data in a tea.Cmd goroutine
and apply results via metadataResultMsg in Update() with zero I/O.
Also eliminated redundant second capture-pane call by adding
HasUpdatedWithContent() which returns content alongside update flags.
```

---

### Task 5: Optimize the preview tick

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_state.go`

The `previewTickMsg` fires every 50ms and calls `instanceChanged()` which calls `UpdatePreview()` → `instance.Preview()` → tmux capture-pane. This is another synchronous subprocess call in `Update()`.

After Task 4, the metadata tick already captures pane content every 500ms. We can cache the last captured content on the instance and have the preview tick read from cache instead of shelling out to tmux.

**Step 1: Add a content cache field to Instance**

Add to `session/instance.go`, in the Instance struct:

```go
	// CachedContent is the last tmux capture-pane output, set by CollectMetadata.
	// Used by the preview tick to avoid redundant subprocess calls.
	CachedContent string
```

**Step 2: Update `CollectMetadata` to populate the cache**

In `session/instance_session.go`, at the end of `CollectMetadata()`, before the return:

```go
	// Cache the content for the preview tick to read without shelling out.
	i.CachedContent = m.Content
```

**Step 3: Add `PreviewCached` method**

Add to `session/instance_session.go`:

```go
// PreviewCached returns the last captured pane content without a subprocess call.
// Falls back to live capture if the cache is empty (first tick).
func (i *Instance) PreviewCached() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	if i.CachedContent != "" {
		return i.CachedContent, nil
	}
	return i.Preview()
}
```

**Step 4: Update `UpdateContent` in preview pane to use cached content**

In `ui/preview.go`, change the normal-mode preview call (line ~198):

```go
	} else if !p.isScrolling {
		content, err = instance.PreviewCached()
```

**Step 5: Run tests**

Run: `go test ./... && go build ./...`
Expected: PASS

**Step 6: Commit**

```
perf(app): use cached pane content for preview tick

The 50ms preview tick was shelling out to tmux capture-pane on every
tick. Now reads from CachedContent populated by the async metadata
tick, eliminating ~20 subprocess calls/second from the event loop.
Falls back to live capture on the first tick before metadata runs.
```

---

### Task 6: Verify and clean up

**Files:**
- Modify: `session/instance_session.go` — remove now-unused `GetPaneContent()` if nothing else calls it
- Modify: `app/app_test.go` — update confirmation tests to use `pendingConfirmAction` pattern instead of `OnConfirm` callbacks

**Step 1: Check for unused methods**

Search for callers of `GetPaneContent()` — it was only called from the metadata tick's `content, err := instance.GetPaneContent()` line which is now removed. If no other callers exist, delete it.

Search for callers of `HasUpdated()` on Instance — the metadata tick now uses `CollectMetadata()` which calls `HasUpdatedWithContent()`. If nothing else calls `HasUpdated()`, it can stay (the tmux method is still used internally) but the Instance wrapper can be removed.

**Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: All PASS

**Step 3: Run with multiple instances to verify**

Launch klique with 3+ active instances. Press `V` on a plan — it should render in <100ms, not 10 seconds. The UI should stay responsive during metadata ticks.

**Step 4: Commit**

```
refactor(session): remove unused GetPaneContent wrapper

No longer needed — metadata tick uses CollectMetadata which calls
HasUpdatedWithContent directly. Preview tick uses PreviewCached.
```
