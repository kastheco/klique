# Plan Lifecycle & Agent Sessions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire up the full plan lifecycle — from creation (stub file + branch) through agent session spawning (planner/coder/reviewer) to completion detection and push prompts.

**Architecture:** Plan creation commits stub to main and creates feature branch. AgentType field on Instance drives --agent flag injection. Sub-item actions in the sidebar dispatch the appropriate agent type on the correct branch. End-of-implementation detection triggers push confirmation.

**Tech Stack:** Go, bubbletea, tmux, git

**Important — Recent Codebase Changes (post-plan-authoring):**

1. **Sidebar toggle** — `sidebarHidden bool` on `home` struct, `KeyToggleSidebar` (ctrl+s), two-step reveal for `s` and `left` keys. Keep this intact — it's orthogonal to lifecycle work.
2. **Global background fill** — `ui.FillBackground()` in `View()`, `termWidth`/`termHeight` on `home`. All styles use `.Background(ColorBase)`. Keep this intact.
3. **`IsReviewer` field** — Plan 1 may not have removed `IsReviewer` from Instance yet (it depends on `AgentType` being added here). Check if `IsReviewer` still exists before removing it; if Plan 1 left it in place, this plan should replace it with `AgentType == "reviewer"`.

---

### Task 1: Plan State Registration Helpers

**Files:**
- Modify: `config/planstate/planstate.go`
- Modify: `config/planstate/planstate_test.go`

1. **Write the failing test**

Add these tests to `config/planstate/planstate_test.go`:

```go
func TestRegisterPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o644))

	ps, err := Load(dir)
	require.NoError(t, err)

	now := time.Date(2026, 2, 21, 15, 4, 5, 0, time.UTC)
	err = ps.Register("2026-02-21-auth-refactor.md", "refactor auth flow", "plan/auth-refactor", now)
	require.NoError(t, err)

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, StatusReady, entry.Status)
	assert.Equal(t, "refactor auth flow", entry.Description)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
	assert.Equal(t, now, entry.CreatedAt)
}

func TestRegisterPlan_RejectsDuplicate(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"2026-02-21-auth-refactor.md": {
				Status:      StatusReady,
				Description: "existing",
				Branch:      "plan/auth-refactor",
				CreatedAt:   time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	err := ps.Register(
		"2026-02-21-auth-refactor.md",
		"new description",
		"plan/auth-refactor",
		time.Now().UTC(),
	)
	assert.Error(t, err)
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./config/planstate -run 'TestRegisterPlan|TestRegisterPlan_RejectsDuplicate' -v
```

Expected: FAIL because `Register` and `Entry` do not exist yet.

3. **Implement registration helpers**

Update `config/planstate/planstate.go`:

```go
type PlanEntry struct {
	Status      Status    `json:"status"`
	Description string    `json:"description,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

func (ps *PlanState) Register(filename, description, branch string, createdAt time.Time) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	if _, exists := ps.Plans[filename]; exists {
		return fmt.Errorf("plan already exists: %s", filename)
	}
	ps.Plans[filename] = PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		CreatedAt:   createdAt.UTC(),
	}
	return ps.save()
}

func (ps *PlanState) Entry(filename string) (PlanEntry, bool) {
	entry, ok := ps.Plans[filename]
	return entry, ok
}
```

4. **Run tests**

Run:

```bash
go test ./config/planstate -v
```

Expected: PASS.

5. **Commit**

```bash
git add config/planstate/planstate.go config/planstate/planstate_test.go
git commit -m "feat(planstate): add plan registration metadata helpers"
```

---

### Task 2: Git Primitives for Plan Bootstrap and Reset

**Files:**
- Create: `session/git/plan_lifecycle.go`
- Create: `session/git/plan_lifecycle_test.go`

1. **Write the failing test**

Create `session/git/plan_lifecycle_test.go`:

```go
package git

import (
	"path/filepath"
	"testing"
)

func TestPlanBranchFromFile(t *testing.T) {
	got := PlanBranchFromFile("2026-02-21-auth-refactor.md")
	want := "plan/auth-refactor"
	if got != want {
		t.Fatalf("PlanBranchFromFile() = %q, want %q", got, want)
	}
}

func TestPlanWorktreePath(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	got := PlanWorktreePath(repo, branch)
	want := filepath.Join(repo, ".worktrees", "plan-auth-refactor")
	if got != want {
		t.Fatalf("PlanWorktreePath() = %q, want %q", got, want)
	}
}

func TestNewSharedPlanWorktree(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	gt := NewSharedPlanWorktree(repo, branch)

	if gt.GetRepoPath() != repo {
		t.Fatalf("repo = %q, want %q", gt.GetRepoPath(), repo)
	}
	if gt.GetWorktreePath() != filepath.Join(repo, ".worktrees", "plan-auth-refactor") {
		t.Fatalf("unexpected worktree path %q", gt.GetWorktreePath())
	}
	if gt.GetBranchName() != branch {
		t.Fatalf("branch = %q, want %q", gt.GetBranchName(), branch)
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./session/git -run 'TestPlanBranchFromFile|TestPlanWorktreePath|TestNewSharedPlanWorktree' -v
```

Expected: FAIL because the helper functions do not exist.

3. **Implement the plan git helpers**

Create `session/git/plan_lifecycle.go`:

```go
package git

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config/planstate"
)

func PlanBranchFromFile(planFile string) string {
	name := planstate.DisplayName(planFile)
	name = sanitizeBranchName(name)
	if name == "" {
		name = "plan"
	}
	return "plan/" + name
}

func PlanWorktreePath(repoPath, branch string) string {
	safe := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(repoPath, ".worktrees", safe)
}

func NewSharedPlanWorktree(repoPath, branch string) *GitWorktree {
	return NewGitWorktreeFromStorage(
		repoPath,
		PlanWorktreePath(repoPath, branch),
		"plan-shared",
		branch,
		"",
	)
}

func CommitPlanScaffoldOnMain(repoPath, planFile string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	if _, err := gt.runGitCommand(repoPath, "add", filepath.Join("docs", "plans", planFile), filepath.Join("docs", "plans", "plan-state.json")); err != nil {
		return fmt.Errorf("stage plan scaffold: %w", err)
	}
	if _, err := gt.runGitCommand(repoPath, "commit", "-m", "feat(plan): add "+planstate.DisplayName(planFile)+" scaffold"); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("commit plan scaffold: %w", err)
	}
	return nil
}

func EnsurePlanBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	if _, err := gt.runGitCommand(repoPath, "rev-parse", "--verify", branch); err == nil {
		return nil
	}
	if _, err := gt.runGitCommand(repoPath, "branch", branch); err != nil {
		return fmt.Errorf("create plan branch %s: %w", branch, err)
	}
	return nil
}

func ResetPlanBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	worktreePath := PlanWorktreePath(repoPath, branch)
	_, _ = gt.runGitCommand(repoPath, "worktree", "remove", "-f", worktreePath)
	_, _ = gt.runGitCommand(repoPath, "branch", "-D", branch)
	if _, err := gt.runGitCommand(repoPath, "branch", branch); err != nil {
		return fmt.Errorf("recreate plan branch %s: %w", branch, err)
	}
	if _, err := gt.runGitCommand(repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("prune worktrees: %w", err)
	}
	return nil
}
```

4. **Run tests**

Run:

```bash
go test ./session/git -v
```

Expected: PASS.

5. **Commit**

```bash
git add session/git/plan_lifecycle.go session/git/plan_lifecycle_test.go
git commit -m "feat(git): add plan branch and shared worktree helpers"
```

---

### Task 3: Replace `IsReviewer` with `AgentType` on Instance

**Files:**
- Modify: `session/instance.go`
- Modify: `session/storage.go`
- Modify: `session/instance_planfile_test.go`

1. **Write the failing test**

Append to `session/instance_planfile_test.go`:

```go
func TestNewInstance_SetsAgentType(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:     "planner-worker",
		Path:      ".",
		Program:   "opencode",
		PlanFile:  "2026-02-21-auth-refactor.md",
		AgentType: AgentTypePlanner,
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	if inst.AgentType != AgentTypePlanner {
		t.Fatalf("AgentType = %q, want %q", inst.AgentType, AgentTypePlanner)
	}
}

func TestInstanceData_RoundTripAgentType(t *testing.T) {
	data := InstanceData{
		Title:     "persisted",
		Path:      "/tmp/repo",
		Branch:    "plan/auth-refactor",
		Status:    Paused,
		Program:   "opencode",
		PlanFile:  "2026-02-21-auth-refactor.md",
		AgentType: AgentTypeReviewer,
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/repo/.worktrees/plan-auth-refactor",
			SessionName:   "persisted",
			BranchName:    "plan/auth-refactor",
			BaseCommitSHA: "abc123",
		},
	}

	inst, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if inst.AgentType != AgentTypeReviewer {
		t.Fatalf("instance AgentType = %q, want %q", inst.AgentType, AgentTypeReviewer)
	}

	roundTrip := inst.ToInstanceData()
	if roundTrip.AgentType != AgentTypeReviewer {
		t.Fatalf("ToInstanceData AgentType = %q, want %q", roundTrip.AgentType, AgentTypeReviewer)
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./session -run 'TestNewInstance_SetsAgentType|TestInstanceData_RoundTripAgentType' -v
```

Expected: FAIL because `AgentType` fields/constants are missing.

3. **Implement `AgentType` and persistence**

Update `session/instance.go`:

```go
const (
	AgentTypePlanner  = "planner"
	AgentTypeCoder    = "coder"
	AgentTypeReviewer = "reviewer"
)

type Instance struct {
	// ...existing fields...
	PlanFile string
	// AgentType is planner/coder/reviewer or empty for ad-hoc sessions.
	AgentType string
	QueuedPrompt string
	// ...
}

type InstanceOptions struct {
	Title     string
	Path      string
	Program   string
	AutoYes   bool
	SkipPermissions bool
	TopicName string
	PlanFile  string
	AgentType string
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	// ...existing setup...
	return &Instance{
		Title:           opts.Title,
		Status:          Ready,
		Path:            absPath,
		Program:         opts.Program,
		AutoYes:         opts.AutoYes,
		SkipPermissions: opts.SkipPermissions,
		TopicName:       opts.TopicName,
		PlanFile:        opts.PlanFile,
		AgentType:       opts.AgentType,
		CreatedAt:       t,
		UpdatedAt:       t,
	}, nil
}
```

Update `session/storage.go`:

```go
type InstanceData struct {
	Title           string    `json:"title"`
	Path            string    `json:"path"`
	Branch          string    `json:"branch"`
	Status          Status    `json:"status"`
	Height          int       `json:"height"`
	Width           int       `json:"width"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	AutoYes         bool      `json:"auto_yes"`
	SkipPermissions bool      `json:"skip_permissions"`
	TopicName       string    `json:"topic_name,omitempty"`
	PlanFile        string    `json:"plan_file,omitempty"`
	AgentType       string    `json:"agent_type,omitempty"`
	QueuedPrompt    string    `json:"queued_prompt,omitempty"`
	Program         string          `json:"program"`
	Worktree        GitWorktreeData `json:"worktree"`
	DiffStats       DiffStatsData   `json:"diff_stats"`
}
```

And in `ToInstanceData` / `FromInstanceData` (in `session/instance.go`) map `AgentType` in both directions.

4. **Run tests**

Run:

```bash
go test ./session -v
```

Expected: PASS.

5. **Commit**

```bash
git add session/instance.go session/storage.go session/instance_planfile_test.go
git commit -m "refactor(session): replace reviewer bool with agent type"
```

---

### Task 4: Inject `--agent` at tmux Startup

**Files:**
- Modify: `session/tmux/tmux.go`
- Modify: `session/instance_lifecycle.go`
- Modify: `session/tmux/tmux_test.go`

1. **Write the failing test**

Add to `session/tmux/tmux_test.go`:

```go
func TestStartTmuxSessionInjectsAgentFlag(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := newTmuxSession("agent-test", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s klique_agent-test -c %s opencode --agent planner", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./session/tmux -run TestStartTmuxSessionInjectsAgentFlag -v
```

Expected: FAIL because `SetAgentType` and flag injection are missing.

3. **Implement agent flag injection**

Update `session/tmux/tmux.go`:

```go
type TmuxSession struct {
	// ...existing fields...
	agentType string
	// ...
}

func (t *TmuxSession) SetAgentType(agentType string) {
	t.agentType = strings.TrimSpace(agentType)
}

func (t *TmuxSession) Start(workDir string) error {
	// ...existing setup...
	program := t.program
	if t.skipPermissions && isClaudeProgram(program) {
		program += " --dangerously-skip-permissions"
	}
	if t.agentType != "" && !strings.Contains(program, "--agent") {
		program += " --agent " + t.agentType
	}
	cmd := exec.Command("tmux", "new-session", "-d", "-s", t.sanitizedName, "-c", workDir, program)
	// ...rest unchanged...
}
```

Update `session/instance_lifecycle.go`:

```go
if i.tmuxSession != nil {
	tmuxSession = i.tmuxSession
} else {
	tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
}
tmuxSession.SetAgentType(i.AgentType)
```

Apply the same `SetAgentType(i.AgentType)` in `StartInSharedWorktree` after constructing/reusing the tmux session.

4. **Run tests**

Run:

```bash
go test ./session/tmux -v
go test ./session -v
```

Expected: PASS.

5. **Commit**

```bash
git add session/tmux/tmux.go session/instance_lifecycle.go session/tmux/tmux_test.go
git commit -m "feat(tmux): inject agent flag from instance agent type"
```

---

### Task 5: Finalize Plan Creation Flow (`p` wizard completion)

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Create: `app/app_plan_creation_test.go`

1. **Write the failing test**

Create `app/app_plan_creation_test.go`:

```go
package app

import (
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
)

func TestBuildPlanFilename(t *testing.T) {
	got := buildPlanFilename("Auth Refactor", time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	want := "2026-02-21-auth-refactor.md"
	if got != want {
		t.Fatalf("buildPlanFilename() = %q, want %q", got, want)
	}
}

func TestRenderPlanStub(t *testing.T) {
	stub := renderPlanStub("Auth Refactor", "Refactor JWT auth", "2026-02-21-auth-refactor.md")
	if !strings.Contains(stub, "# Auth Refactor") {
		t.Fatalf("stub missing title: %s", stub)
	}
	if !strings.Contains(stub, "Refactor JWT auth") {
		t.Fatalf("stub missing description")
	}
}

func TestCreatePlanRecord(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	h := &home{planStateDir: plansDir, planState: ps}

	planFile := "2026-02-21-auth-refactor.md"
	branch := "plan/auth-refactor"
	err = h.createPlanRecord(planFile, "Refactor JWT auth", branch, time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	entry, ok := h.planState.Entry(planFile)
	require.True(t, ok)
	if entry.Branch != branch {
		t.Fatalf("entry.Branch = %q, want %q", entry.Branch, branch)
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./app -run 'TestBuildPlanFilename|TestRenderPlanStub|TestCreatePlanRecord' -v
```

Expected: FAIL because helpers do not exist.

3. **Implement creation finalization**

In `app/app_state.go`, add:

```go
func buildPlanFilename(name string, now time.Time) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "plan"
	}
	return now.UTC().Format("2006-01-02") + "-" + slug + ".md"
}

func renderPlanStub(name, description, filename string) string {
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by klique lifecycle flow\n- Plan file: %s\n", name, description, filename)
}

func (m *home) createPlanRecord(planFile, description, branch string, now time.Time) error {
	if m.planState == nil {
		ps, err := planstate.Load(m.planStateDir)
		if err != nil {
			return err
		}
		m.planState = ps
	}
	return m.planState.Register(planFile, description, branch, now)
}

func (m *home) finalizePlanCreation(name, description string) error {
	now := time.Now().UTC()
	planFile := buildPlanFilename(name, now)
	branch := git.PlanBranchFromFile(planFile)
	planPath := filepath.Join(m.planStateDir, planFile)

	if err := os.MkdirAll(m.planStateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(planPath, []byte(renderPlanStub(name, description, planFile)), 0o644); err != nil {
		return err
	}
	if err := m.createPlanRecord(planFile, description, branch, now); err != nil {
		return err
	}
	if err := git.CommitPlanScaffoldOnMain(m.activeRepoPath, planFile); err != nil {
		return err
	}
	if err := git.EnsurePlanBranch(m.activeRepoPath, branch); err != nil {
		return err
	}

	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateSidebarItems()
	return nil
}
```

In `app/app_input.go`, in the existing Plan 1 “description submitted” branch, call:

```go
if err := m.finalizePlanCreation(planName, planDescription); err != nil {
	return m, m.handleError(err)
}
m.state = stateDefault
m.menu.SetState(ui.StateDefault)
return m, tea.WindowSize()
```

4. **Run tests**

Run:

```bash
go test ./app -v
```

Expected: PASS.

5. **Commit**

```bash
git add app/app.go app/app_input.go app/app_state.go app/app_plan_creation_test.go
git commit -m "feat(app): finalize plan creation with scaffold commit and branch"
```

---

### Task 6: Sub-Item Action Dispatch (`plan` / `implement` / `review`)

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Modify: `ui/sidebar.go` (only if Plan 1 does not already expose selected sub-item metadata)
- Create: `app/app_plan_actions_test.go`

1. **Write the failing test**

Create `app/app_plan_actions_test.go`:

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/session"
)

func TestBuildPlanPrompt(t *testing.T) {
	prompt := buildPlanPrompt("Auth Refactor", "Refactor JWT auth")
	if !strings.Contains(prompt, "Plan Auth Refactor") {
		t.Fatalf("prompt missing title")
	}
	if !strings.Contains(prompt, "Goal: Refactor JWT auth") {
		t.Fatalf("prompt missing goal")
	}
}

func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("2026-02-21-auth-refactor.md")
	if !strings.Contains(prompt, "Implement docs/plans/2026-02-21-auth-refactor.md") {
		t.Fatalf("prompt missing plan path")
	}
}

func TestAgentTypeForSubItem(t *testing.T) {
	tests := map[string]string{
		"plan":      session.AgentTypePlanner,
		"implement": session.AgentTypeCoder,
		"review":    session.AgentTypeReviewer,
	}
	for action, want := range tests {
		got, ok := agentTypeForSubItem(action)
		if !ok {
			t.Fatalf("agentTypeForSubItem(%q) returned ok=false", action)
		}
		if got != want {
			t.Fatalf("agentTypeForSubItem(%q) = %q, want %q", action, got, want)
		}
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./app -run 'TestBuildPlanPrompt|TestBuildImplementPrompt|TestAgentTypeForSubItem' -v
```

Expected: FAIL because helper functions do not exist.

3. **Implement sub-item dispatch and spawn helpers**

In `app/app_state.go`, add helpers:

```go
func buildPlanPrompt(planName, description string) string {
	return fmt.Sprintf("Plan %s. Goal: %s.", planName, description)
}

func buildImplementPrompt(planFile string) string {
	return fmt.Sprintf(
		"Implement docs/plans/%s using the executing-plans superpowers skill. Execute all tasks sequentially.",
		planFile,
	)
}

func buildModifyPlanPrompt(planFile string) string {
	return fmt.Sprintf("Modify existing plan at docs/plans/%s. Keep the same filename and update only what changed.", planFile)
}

func agentTypeForSubItem(action string) (string, bool) {
	switch action {
	case "plan":
		return session.AgentTypePlanner, true
	case "implement":
		return session.AgentTypeCoder, true
	case "review":
		return session.AgentTypeReviewer, true
	default:
		return "", false
	}
}

func (m *home) spawnPlanAgent(planFile, action, prompt string) (tea.Model, tea.Cmd) {
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
	}

	agentType, ok := agentTypeForSubItem(action)
	if !ok {
		return m, m.handleError(fmt.Errorf("unknown plan action: %s", action))
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     planstate.DisplayName(planFile) + "-" + action,
		Path:      m.activeRepoPath,
		Program:   m.program,
		PlanFile:  planFile,
		AgentType: agentType,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	inst.QueuedPrompt = prompt

	var startCmd tea.Cmd
	if action == "plan" {
		repoWorktree := git.NewGitWorktreeFromStorage(m.activeRepoPath, m.activeRepoPath, inst.Title, "main", "")
		startCmd = func() tea.Msg {
			err := inst.StartInSharedWorktree(repoWorktree, "main")
			return instanceStartedMsg{instance: inst, err: err}
		}
	} else {
		shared := git.NewSharedPlanWorktree(m.activeRepoPath, entry.Branch)
		if err := shared.Setup(); err != nil {
			return m, m.handleError(err)
		}
		startCmd = func() tea.Msg {
			err := inst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: inst, err: err}
		}
	}

	m.newInstanceFinalizer = m.list.AddInstance(inst)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)
	return m, tea.Batch(tea.WindowSize(), startCmd)
}
```

In `app/app_input.go`, in Enter handling for sidebar-focused plan sub-items:

```go
if m.focusedPanel == 0 {
	planFile, subItem, ok := m.sidebar.GetSelectedPlanSubItem()
	if ok {
		switch subItem {
		case "plan":
			entry, _ := m.planState.Entry(planFile)
			_ = m.planState.SetStatus(planFile, planstate.StatusPlanning)
			return m.spawnPlanAgent(planFile, "plan", buildPlanPrompt(planstate.DisplayName(planFile), entry.Description))
		case "implement":
			_ = m.planState.SetStatus(planFile, planstate.StatusImplementing)
			return m.spawnPlanAgent(planFile, "implement", buildImplementPrompt(planFile))
		case "review":
			_ = m.planState.SetStatus(planFile, planstate.StatusReviewing)
			return m.spawnPlanAgent(planFile, "review", scaffold.LoadReviewPrompt("docs/plans/"+planFile, planstate.DisplayName(planFile)))
		}
	}
}
```

If Plan 1 does not already expose `GetSelectedPlanSubItem()`, add in `ui/sidebar.go`:

```go
func (s *Sidebar) GetSelectedPlanSubItem() (planFile string, subItem string, ok bool) {
	id := s.GetSelectedID()
	if !strings.HasPrefix(id, "__plan_item__") {
		return "", "", false
	}
	payload := strings.TrimPrefix(id, "__plan_item__")
	parts := strings.SplitN(payload, "::", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
```

4. **Run tests**

Run:

```bash
go test ./app -v
go test ./ui -v
```

Expected: PASS.

5. **Commit**

```bash
git add app/app_input.go app/app_state.go app/app_actions.go app/app_plan_actions_test.go ui/sidebar.go
git commit -m "feat(app): dispatch plan sub-items to planner/coder/reviewer sessions"
```

---

### Task 7: Detect Coder Completion and Prompt Push

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Create: `app/app_plan_completion_test.go`

1. **Write the failing test**

Create `app/app_plan_completion_test.go`:

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
)

func TestShouldPromptPushAfterCoderExit(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeCoder}

	if !shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("expected push prompt for exited coder")
	}
}

func TestShouldPromptPushAfterCoderExit_NoPromptForReviewer(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeReviewer}

	if shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("did not expect push prompt for reviewer")
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./app -run 'TestShouldPromptPushAfterCoderExit|TestShouldPromptPushAfterCoderExit_NoPromptForReviewer' -v
```

Expected: FAIL because helper does not exist.

3. **Implement completion detection and push prompt**

In `app/app_state.go`, replace old done-status review transition logic with:

```go
func shouldPromptPushAfterCoderExit(entry planstate.PlanEntry, inst *session.Instance, tmuxAlive bool) bool {
	if inst == nil {
		return false
	}
	if inst.PlanFile == "" {
		return false
	}
	if inst.AgentType != session.AgentTypeCoder {
		return false
	}
	if entry.Status != planstate.StatusImplementing {
		return false
	}
	return !tmuxAlive
}

func (m *home) checkPlanCompletion() tea.Cmd {
	if m.planState == nil {
		return nil
	}
	for _, inst := range m.list.GetInstances() {
		entry, ok := m.planState.Entry(inst.PlanFile)
		if !ok {
			continue
		}
		if !shouldPromptPushAfterCoderExit(entry, inst, inst.TmuxAlive()) {
			continue
		}
		return m.promptPushBranchThenAdvance(inst)
	}
	return nil
}

func (m *home) promptPushBranchThenAdvance(inst *session.Instance) tea.Cmd {
	message := fmt.Sprintf("[!] Implementation finished for '%s'. Push branch now?", planstate.DisplayName(inst.PlanFile))
	m.state = stateConfirm
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	m.confirmationOverlay.SetWidth(64)

	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		worktree, err := inst.GetGitWorktree()
		if err == nil {
			_ = worktree.PushChanges(
				fmt.Sprintf("[klique] push completed implementation for '%s'", inst.Title),
				false,
			)
		}
		_ = m.planState.SetStatus(inst.PlanFile, planstate.StatusReviewing)
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
		_ = m.planState.SetStatus(inst.PlanFile, planstate.StatusReviewing)
	}

	return nil
}
```

In `app/app.go`, keep `completionCmd := m.checkPlanCompletion()` in the metadata tick batch (already present).

4. **Run tests**

Run:

```bash
go test ./app -v
```

Expected: PASS.

5. **Commit**

```bash
git add app/app.go app/app_state.go app/app_plan_completion_test.go
git commit -m "feat(app): prompt push when coder exits and advance plan to reviewing"
```

---

### Task 8: Plan Context Menu Actions (`modify plan`, `start over`)

**Files:**
- Modify: `app/app_actions.go`
- Modify: `app/app_input.go`
- Create: `app/app_plan_context_actions_test.go`

1. **Write the failing test**

Create `app/app_plan_context_actions_test.go`:

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
)

func TestModifyPlanActionSetsPlanning(t *testing.T) {
	h := &home{
		planState: &planstate.PlanState{
			Dir: "/tmp",
			Plans: map[string]planstate.PlanEntry{
				"2026-02-21-auth-refactor.md": {Status: planstate.StatusImplementing},
			},
		},
	}

	err := h.setPlanStatus("2026-02-21-auth-refactor.md", planstate.StatusPlanning)
	if err != nil {
		t.Fatalf("setPlanStatus error: %v", err)
	}
	entry, _ := h.planState.Entry("2026-02-21-auth-refactor.md")
	if entry.Status != planstate.StatusPlanning {
		t.Fatalf("status = %q, want %q", entry.Status, planstate.StatusPlanning)
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./app -run TestModifyPlanActionSetsPlanning -v
```

Expected: FAIL because helper/action wiring is missing.

3. **Implement plan context menu actions**

In `app/app_actions.go`, add:

```go
func (m *home) setPlanStatus(planFile string, status planstate.Status) error {
	if m.planState == nil {
		return fmt.Errorf("plan state is not loaded")
	}
	return m.planState.SetStatus(planFile, status)
}
```

Extend `executeContextAction` with:

```go
case "modify_plan":
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}
	if err := m.setPlanStatus(planFile, planstate.StatusPlanning); err != nil {
		return m, m.handleError(err)
	}
	return m.spawnPlanAgent(planFile, "plan", buildModifyPlanPrompt(planFile))

case "start_over_plan":
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return m, m.handleError(fmt.Errorf("plan not found: %s", planFile))
	}
	for i := len(m.allInstances) - 1; i >= 0; i-- {
		if m.allInstances[i].PlanFile == planFile {
			m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
		}
	}
	m.list.KillInstancesByPlan(planFile)
	if err := git.ResetPlanBranch(m.activeRepoPath, entry.Branch); err != nil {
		return m, m.handleError(err)
	}
	if err := m.setPlanStatus(planFile, planstate.StatusPlanning); err != nil {
		return m, m.handleError(err)
	}
	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
```

Also update plan-header context menu construction (`openContextMenu` and right-click path) to include:

```go
items := []overlay.ContextMenuItem{
	{Label: "Modify plan", Action: "modify_plan"},
	{Label: "Start over", Action: "start_over_plan"},
	{Label: "Kill running instances", Action: "kill_plan_instances"},
	{Label: "Push branch", Action: "push_plan_branch"},
	{Label: "Create PR", Action: "create_pr_plan"},
}
```

4. **Run tests**

Run:

```bash
go test ./app -v
```

Expected: PASS.

5. **Commit**

```bash
git add app/app_actions.go app/app_input.go app/app_plan_context_actions_test.go
git commit -m "feat(app): add modify-plan and start-over plan context actions"
```

---

### Task 9: Planner Agent Branch Policy Contract

**Files:**
- Modify: `.opencode/agents/planner.md`
- Create: `contracts/planner_prompt_contract_test.go`

1. **Write the failing test**

Create `contracts/planner_prompt_contract_test.go`:

```go
package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlannerPromptBranchPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".opencode", "agents", "planner.md"))
	if err != nil {
		t.Fatalf("read planner prompt: %v", err)
	}
	text := string(data)

	required := []string{
		"Always commit plan files to the main branch.",
		"Do NOT create feature branches for planning work.",
		"Only register implementation plans in plan-state.json",
		"never register design docs",
	}

	for _, needle := range required {
		if !strings.Contains(text, needle) {
			t.Fatalf("planner prompt missing required policy text: %q", needle)
		}
	}
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./contracts -run TestPlannerPromptBranchPolicy -v
```

Expected: FAIL because planner prompt does not yet include all required policy lines.

3. **Update planner prompt**

In `.opencode/agents/planner.md`, add this section verbatim:

```markdown
## Branch Policy

Always commit plan files to the main branch. Do NOT create feature branches
for planning work. The feature branch for implementation is created by klique
when the user triggers "implement".

Only register implementation plans in plan-state.json — never register
design docs (*-design.md) as separate entries.
```

4. **Run tests**

Run:

```bash
go test ./contracts -v
```

Expected: PASS.

5. **Commit**

```bash
git add .opencode/agents/planner.md contracts/planner_prompt_contract_test.go
git commit -m "docs(planner): codify main-branch planning policy"
```

---

### Task 10: End-to-End Verification of Plan Lifecycle Wiring

**Files:**
- Test only: existing modified files from Tasks 1-9

1. **Write a failing integration test first**

Add to `app/app_plan_completion_test.go`:

```go
func TestFullPlanLifecycle_StateTransitions(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(
		"2026-02-21-auth-refactor.md",
		"Refactor JWT auth",
		"plan/auth-refactor",
		time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
	))

	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusPlanning))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusImplementing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusReviewing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusFinished))

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, planstate.StatusFinished, entry.Status)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
}
```

2. **Run test to verify it fails**

Run:

```bash
go test ./app -run TestFullPlanLifecycle_StateTransitions -v
```

Expected: FAIL if any status constants/helpers are inconsistent.

3. **Fix any final inconsistencies found by the test**

Apply only minimal consistency fixes across:
- `config/planstate/planstate.go`
- `app/app_state.go`
- `app/app_input.go`
- `session/instance.go`

4. **Run full verification**

Run:

```bash
go test ./... -count=1
go build ./...
```

Expected: all tests pass, build clean.

5. **Commit**

```bash
git add app/app_state.go app/app_input.go config/planstate/planstate.go session/instance.go
git commit -m "test(app): verify full plan lifecycle transitions"
```
