# Spawn Agent Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restore `s` keybind for spawning ad-hoc agent sessions outside the plan lifecycle, with optional branch/path overrides via a form overlay.

**Architecture:** New `stateSpawnAgent` state with a `huh`-backed form overlay (name + optional branch + optional path). On submit, creates an `Instance` with no `PlanFile`/`AgentType`, using standard or overridden worktree setup. All lifecycle opt-out is automatic via existing `PlanFile == ""` guards.

**Tech Stack:** Go, bubbletea, huh (form framework), lipgloss

---

## Wave 1: Form Overlay and Key Constants

### Task 1: Add spawn form overlay constructor

**Files:**
- Modify: `ui/overlay/formOverlay.go`
- Test: `ui/overlay/formOverlay_test.go`

**Step 1: Write failing tests for the spawn form overlay**

Add these tests to `ui/overlay/formOverlay_test.go`:

```go
func TestSpawnFormOverlay_SubmitWithNameOnly(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "my-task" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "my-task", f.Name())
	assert.Equal(t, "", f.Branch())
	assert.Equal(t, "", f.WorkPath())
}

func TestSpawnFormOverlay_SubmitWithAllFields(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "task" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "feature/login" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "/tmp/worktree" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "task", f.Name())
	assert.Equal(t, "feature/login", f.Branch())
	assert.Equal(t, "/tmp/worktree", f.WorkPath())
}

func TestSpawnFormOverlay_EmptyNameDoesNotSubmit(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, closed)
	assert.False(t, f.IsSubmitted())
}

func TestSpawnFormOverlay_TabCyclesThreeFields(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "n" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to branch
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "b" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to path
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "p" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab wraps to name
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "!" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, closed)
	assert.Equal(t, "n!", f.Name())
	assert.Equal(t, "b", f.Branch())
	assert.Equal(t, "p", f.WorkPath())
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/overlay/ -run TestSpawnFormOverlay -v`
Expected: FAIL — `NewSpawnFormOverlay` undefined, `Branch`/`WorkPath` methods missing.

**Step 3: Implement NewSpawnFormOverlay**

In `ui/overlay/formOverlay.go`, add fields and constructor:

```go
// Add fields to FormOverlay struct (after descVal):
//   branchVal string
//   pathVal   string

// NewSpawnFormOverlay creates a form overlay with name, branch (optional), and path (optional) inputs.
func NewSpawnFormOverlay(title string, width int) *FormOverlay {
	f := &FormOverlay{
		title: title,
		width: width,
	}

	formWidth := width - 6
	if formWidth < 34 {
		formWidth = 34
	}

	f.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("name").
				Value(&f.nameVal),
			huh.NewInput().
				Key("branch").
				Title("branch (optional)").
				Value(&f.branchVal),
			huh.NewInput().
				Key("path").
				Title("path (optional)").
				Value(&f.pathVal),
		),
	).
		WithTheme(ThemeRosePine()).
		WithWidth(formWidth).
		WithShowHelp(false).
		WithShowErrors(false)

	_ = f.form.Init()
	return f
}

// Branch returns the branch field value.
func (f *FormOverlay) Branch() string {
	return strings.TrimSpace(f.branchVal)
}

// WorkPath returns the path field value.
func (f *FormOverlay) WorkPath() string {
	return strings.TrimSpace(f.pathVal)
}
```

The `HandleKeyPress` method needs updating: the existing tab/shift-tab wrapping logic hardcodes two fields (`name` ↔ `desc`). For the spawn overlay with three fields, the navigation must wrap correctly. The fix: change the wrap logic to use `huh.NextField()`/`huh.PrevField()` with wrap detection based on the **current focused key** and the **field count**.

Replace the tab/shift-tab cases in `HandleKeyPress`:

```go
case tea.KeyTab, tea.KeyDown:
	focused := f.focusedKey()
	// Determine the last field key for wrap detection
	lastKey := "desc" // default for plan form
	if f.pathVal != "" || f.branchVal != "" || f.form.GetFocusedField() != nil {
		// Check if this is a 3-field form by looking for "path" key
		// Simple approach: try to detect the form shape
	}
	// Actually, the cleanest approach: always use huh's NextField.
	// If we're on the last field, wrap to first. Detect by checking
	// if focused is the last field.
	switch focused {
	case "path":
		// 3-field form: wrap from path → name
		f.updateForm(huh.PrevField())
		f.updateForm(huh.PrevField())
	case "desc":
		// 2-field form: wrap from desc → name
		f.updateForm(huh.PrevField())
	default:
		f.updateForm(huh.NextField())
	}
	return false
```

Actually, this gets messy. A cleaner approach: **add a `fieldKeys []string` slice** to `FormOverlay` set by each constructor. Use it for wrap detection:

```go
type FormOverlay struct {
	form      *huh.Form
	nameVal   string
	descVal   string
	branchVal string
	pathVal   string
	title     string
	submitted bool
	canceled  bool
	width     int
	fieldKeys []string // ordered list of field keys for tab wrap
}
```

In `NewFormOverlay`: `f.fieldKeys = []string{"name", "desc"}`
In `NewSpawnFormOverlay`: `f.fieldKeys = []string{"name", "branch", "path"}`

Then in `HandleKeyPress`:

```go
case tea.KeyTab, tea.KeyDown:
	focused := f.focusedKey()
	if focused == f.fieldKeys[len(f.fieldKeys)-1] {
		// Wrap: go back to first field
		for i := 0; i < len(f.fieldKeys)-1; i++ {
			f.updateForm(huh.PrevField())
		}
	} else {
		f.updateForm(huh.NextField())
	}
	return false

case tea.KeyShiftTab, tea.KeyUp:
	focused := f.focusedKey()
	if focused == f.fieldKeys[0] {
		// Wrap: go forward to last field
		for i := 0; i < len(f.fieldKeys)-1; i++ {
			f.updateForm(huh.NextField())
		}
	} else {
		f.updateForm(huh.PrevField())
	}
	return false
```

This generalizes the wrap for any number of fields without breaking the existing 2-field plan form.

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/overlay/ -run TestSpawnFormOverlay -v`
Expected: PASS

Also run existing tests to ensure no regressions:
Run: `go test ./ui/overlay/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add ui/overlay/formOverlay.go ui/overlay/formOverlay_test.go
git commit -m "feat: add spawn form overlay with name/branch/path fields"
```

### Task 2: Update key constants and mapping

**Files:**
- Modify: `keys/keys.go`
- Test: `keys/keys_test.go`

**Step 1: Write failing test**

Add to `keys/keys_test.go`:

```go
func TestSpawnAgentKeyInGlobalMap(t *testing.T) {
	name, ok := GlobalKeyStringsMap["s"]
	assert.True(t, ok, "'s' must be in GlobalKeyStringsMap")
	assert.Equal(t, KeySpawnAgent, name)
}

func TestFocusSidebarRemoved(t *testing.T) {
	_, ok := GlobalKeyStringsMap["s"]
	// Should map to KeySpawnAgent, NOT KeyFocusSidebar
	assert.True(t, ok)
	assert.NotEqual(t, KeyFocusSidebar, GlobalKeyStringsMap["s"])
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./keys/ -run TestSpawnAgent -v`
Expected: FAIL — `KeySpawnAgent` undefined, `"s"` not in map.

**Step 3: Update key constants**

In `keys/keys.go`:

1. Replace `KeyFocusSidebar` constant with `KeySpawnAgent`:
   ```go
   KeySpawnAgent // s — spawn ad-hoc agent session
   ```

2. Add `"s"` to `GlobalKeyStringsMap`:
   ```go
   "s": KeySpawnAgent,
   ```

3. Update `GlobalkeyBindings` — replace the `KeyFocusSidebar` entry:
   ```go
   KeySpawnAgent: key.NewBinding(
       key.WithKeys("s"),
       key.WithHelp("s", "spawn agent"),
   ),
   ```

4. Remove `KeyNew` from `GlobalkeyBindings` (dead code — no entry in map, handler unreachable).

**Step 4: Fix compile errors**

The constant rename `KeyFocusSidebar` → `KeySpawnAgent` will break references in:
- `app/app_input.go` (line 1243): the `case keys.KeyFocusSidebar:` handler — **remove this entire case block** (dead code, sidebar focus is handled by arrow keys)
- `app/app.go` (line 300, 387): `slotSidebar` assignments — these use `slotSidebar` not `KeyFocusSidebar`, so no change needed there.

Verify: `rg 'KeyFocusSidebar' --type go` should return zero results after changes.

**Step 5: Run tests to verify they pass**

Run: `go test ./keys/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add keys/keys.go keys/keys_test.go
git commit -m "feat: replace KeyFocusSidebar with KeySpawnAgent, map 's' to spawn"
```

## Wave 2: App State and Handler

### Task 3: Add stateSpawnAgent and form handler

**Files:**
- Modify: `app/app.go` (add state constant, ensure `stateSpawnAgent` skips menu highlighting)
- Modify: `app/app_input.go` (add form handler, add key handler, remove dead code)
- Modify: `app/help.go` (update help text)
- Test: `app/app_test.go`

**Step 1: Write failing tests**

Add to `app/app_test.go`:

```go
func TestSpawnAgent_KeyOpensFormOverlay(t *testing.T) {
	h := newTestHome()
	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	updated := model.(*home)
	require.Equal(t, stateSpawnAgent, updated.state)
	require.NotNil(t, updated.formOverlay, "form overlay must be set")
}

func TestSpawnAgent_EscCancels(t *testing.T) {
	h := newTestHome()
	h.state = stateSpawnAgent
	h.formOverlay = overlay.NewSpawnFormOverlay("spawn agent", 60)

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	updated := model.(*home)
	assert.Equal(t, stateDefault, updated.state)
	assert.Nil(t, updated.formOverlay)
}

func TestSpawnAgent_SubmitCreatesInstance(t *testing.T) {
	h := newTestHome()
	h.state = stateSpawnAgent
	h.formOverlay = overlay.NewSpawnFormOverlay("spawn agent", 60)

	// Type a name
	for _, r := range "test-agent" {
		h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(*home)
	assert.Equal(t, stateDefault, updated.state)
	assert.Nil(t, updated.formOverlay)
	assert.NotNil(t, cmd, "should return start command")

	// Verify instance was added
	instances := updated.list.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "test-agent", last.Title)
	assert.Equal(t, "", last.PlanFile, "ad-hoc instance must have no PlanFile")
	assert.Equal(t, "", last.AgentType, "ad-hoc instance must have no AgentType")
	assert.Equal(t, session.Loading, last.Status)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/ -run TestSpawnAgent -v`
Expected: FAIL — `stateSpawnAgent` undefined, no handler.

**Step 3: Add stateSpawnAgent**

In `app/app.go`, add to the state constants:

```go
// stateSpawnAgent is the state when the user is spawning an ad-hoc agent session.
stateSpawnAgent
```

Add `stateSpawnAgent` to the menu-highlighting skip list in `app/app_input.go` `handleMenuHighlighting` (line 26):

```go
if m.state == statePrompt || m.state == stateHelp || ... || m.state == stateSpawnAgent {
```

**Step 4: Add the form handler in app_input.go**

After the `stateNewPlanTopic` handler block (around line 730), add:

```go
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
```

**Step 5: Add the key handler**

In the main switch on `name` (around line 1310, near `KeyNewPlan`), add:

```go
case keys.KeySpawnAgent:
	if m.list.TotalInstances() >= GlobalInstanceLimit {
		return m, m.handleError(
			fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
	}
	m.state = stateSpawnAgent
	m.formOverlay = overlay.NewSpawnFormOverlay("spawn agent", 60)
	return m, nil
```

**Step 6: Remove dead code**

Remove the `case keys.KeyFocusSidebar:` block (lines 1243-1252) — replaced by `KeySpawnAgent`.

Remove the `case keys.KeyNew:` block (lines 988-1008) — dead code, `KeyNew` is not in `GlobalKeyStringsMap`.

**Step 7: Update help text**

In `app/help.go`, add to the sessions section (after the `↵/o` line):

```go
keyStyle.Render("s")+descStyle.Render("             - spawn agent"),
```

**Step 8: Run tests**

Run: `go test ./app/ -run TestSpawnAgent -v`
Expected: PASS

**Step 9: Commit**

```bash
git add app/app.go app/app_input.go app/help.go app/app_test.go
git commit -m "feat: add stateSpawnAgent with form overlay and 's' key handler"
```

### Task 4: Implement spawnAdHocAgent with branch/path overrides

**Files:**
- Modify: `app/app_state.go` (add `spawnAdHocAgent` method)
- Test: `app/app_test.go`

**Step 1: Write failing tests**

```go
func TestSpawnAdHocAgent_DefaultCreatesWorktree(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "", "")
	updated := model.(*home)
	instances := updated.list.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.Equal(t, session.Loading, last.Status)
	assert.NotNil(t, cmd, "should return async start command")
}

func TestSpawnAdHocAgent_BranchOverride(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "feature/login", "")
	updated := model.(*home)
	instances := updated.list.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.NotNil(t, cmd)
}

func TestSpawnAdHocAgent_PathOverride(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "", "/tmp/custom-path")
	updated := model.(*home)
	instances := updated.list.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.NotNil(t, cmd)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/ -run TestSpawnAdHocAgent -v`
Expected: FAIL — `spawnAdHocAgent` undefined.

**Step 3: Implement spawnAdHocAgent**

Add to `app/app_state.go`:

```go
// spawnAdHocAgent creates and starts an ad-hoc agent session (no plan, no lifecycle).
// branch and workPath are optional overrides — empty strings use defaults.
func (m *home) spawnAdHocAgent(name, branch, workPath string) (tea.Model, tea.Cmd) {
	path := m.activeRepoPath
	if workPath != "" {
		path = workPath
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   name,
		Path:    path,
		Program: m.program,
	})
	if err != nil {
		return m, m.handleError(err)
	}

	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 8
	inst.LoadingMessage = "preparing session..."

	m.state = stateDefault
	m.menu.SetState(ui.StateDefault)

	var startCmd tea.Cmd
	switch {
	case workPath != "" && branch == "":
		// Path override only — run in-place on main branch (no worktree)
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
		}

	case branch != "":
		// Branch override — create worktree on specified branch
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.StartOnBranch(branch)}
		}

	default:
		// No overrides — standard worktree + auto-generated branch
		startCmd = func() tea.Msg {
			return instanceStartedMsg{instance: inst, err: inst.Start(true)}
		}
	}

	m.addInstanceFinalizer(inst, m.list.AddInstance(inst))
	m.list.SelectInstance(inst)
	return m, tea.Batch(tea.WindowSize(), startCmd)
}
```

**Step 4: Run tests**

Run: `go test ./app/ -run TestSpawnAdHocAgent -v`
Expected: PASS (or fail on `StartOnBranch` — handled in Task 5)

**Step 5: Commit**

```bash
git add app/app_state.go app/app_test.go
git commit -m "feat: implement spawnAdHocAgent with branch/path overrides"
```

### Task 5: Add StartOnBranch to instance lifecycle

**Files:**
- Modify: `session/instance_lifecycle.go`
- Test: `session/instance_lifecycle_test.go` (or `session/instance_test.go`)

**Step 1: Write failing test**

```go
func TestStartOnBranch_SetsFields(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:   "test-branch",
		Path:    "/tmp/test-repo",
		Program: "claude",
	})
	require.NoError(t, err)
	assert.Equal(t, "", inst.Branch)
	// We can't fully test Start without a real git repo,
	// but we can verify the method exists and sets the branch field.
}
```

**Step 2: Implement StartOnBranch**

Add to `session/instance_lifecycle.go`:

```go
// StartOnBranch starts the instance in a worktree checked out to the specified branch.
// If the branch exists, it reuses it. If not, it creates a new branch from HEAD.
// Used for ad-hoc agent sessions with a branch override.
func (i *Instance) StartOnBranch(branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 8
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(3+stage, desc)
	}
	i.tmuxSession = tmuxSession
	i.transferPromptToCli()

	i.setLoadingProgress(2, "Creating git worktree...")
	worktree, branchName, err := git.NewGitWorktreeOnBranch(i.Path, i.Title, branch)
	if err != nil {
		return fmt.Errorf("failed to create git worktree on branch %s: %w", branch, err)
	}
	i.gitWorktree = worktree
	i.Branch = branchName

	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	i.setLoadingProgress(3, "Setting up git worktree...")
	if err := i.gitWorktree.Setup(); err != nil {
		setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
		return setupErr
	}

	i.setLoadingProgress(4, "Starting tmux session...")
	if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
		if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
		}
		setupErr = fmt.Errorf("failed to start new session: %w", err)
		return setupErr
	}

	i.SetStatus(Running)
	return nil
}
```

**Step 3: Add NewGitWorktreeOnBranch**

Add to `session/git/worktree.go`:

```go
// NewGitWorktreeOnBranch creates a GitWorktree targeting a specific branch name.
// Unlike NewGitWorktree which auto-generates a branch, this uses the exact branch provided.
// Setup() will handle whether the branch already exists or needs creating.
func NewGitWorktreeOnBranch(repoPath, sessionName, branch string) (*GitWorktree, string, error) {
	branch = sanitizeBranchName(branch)

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	repoPath, err = findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory(repoPath)
	if err != nil {
		return nil, "", err
	}

	worktreePath := filepath.Join(worktreeDir, branch)
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branch,
		worktreePath: worktreePath,
	}, branch, nil
}
```

**Step 4: Run tests**

Run: `go test ./session/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/instance_lifecycle.go session/git/worktree.go
git commit -m "feat: add StartOnBranch and NewGitWorktreeOnBranch for branch overrides"
```

## Wave 3: Render Updates and Polish

### Task 6: Update overlay rendering for spawn form

**Files:**
- Modify: `app/app.go` (render spawn overlay in View)

**Step 1: Check existing overlay rendering**

The `View()` method in `app/app.go` already renders `m.formOverlay` for `stateNewPlan`. Find the render block and add `stateSpawnAgent`:

```go
case m.state == stateNewPlan && m.formOverlay != nil:
	// existing code
case m.state == stateSpawnAgent && m.formOverlay != nil:
	overlayContent = m.formOverlay.Render()
```

Or if it's a single condition checking `m.formOverlay != nil`, just add `stateSpawnAgent` to it.

**Step 2: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: All PASS

**Step 3: Commit**

```bash
git add app/app.go
git commit -m "feat: render spawn agent form overlay in View"
```

### Task 7: Clean up dead code and final polish

**Files:**
- Modify: `keys/keys.go` (remove `KeyNew` from `GlobalkeyBindings` if still present)
- Modify: `app/app_input.go` (verify dead `KeyNew` handler removed)
- Modify: `app/app.go` (remove `stateNew` if now unused — check first)

**Step 1: Verify no remaining references to dead code**

Run: `rg 'KeyNew[^P]' --type go` — should only show `KeyNew` in the binding map (not handler).
Run: `rg 'KeyFocusSidebar' --type go` — should return zero results.

**Step 2: Check if stateNew is still used**

`stateNew` is still used by `N` (KeyPrompt) and `S` (KeyNewSkipPermissions) for the old-style name input flow. Keep it — those flows still work.

**Step 3: Run full test suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: All PASS

**Step 4: Run the app manually to verify**

Run: `go build -o kasmos . && ./kasmos`
- Press `s` — spawn agent form should appear
- Type a name, press enter — instance should start
- Press `s`, fill name + branch, press enter — instance should use that branch
- Press `?` — help should show `s — spawn agent`

**Step 5: Commit**

```bash
git add -A
git commit -m "chore: clean up dead KeyNew/KeyFocusSidebar code"
```
