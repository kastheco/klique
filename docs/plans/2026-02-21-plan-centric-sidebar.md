# Plan-Centric Sidebar Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the klique sidebar from topic-based grouping to plan-centric tree view with progressive lifecycle stages.

**Architecture:** Remove topic system entirely. Evolve plan-state schema with new fields and statuses. Refactor Sidebar widget to render expandable tree nodes. Repurpose keybinds.

**Tech Stack:** Go, bubbletea, lipgloss, bubblezone

**Important — Recent Codebase Changes (post-plan-authoring):**

The following changes landed after this plan was originally written. The executor MUST account for them:

1. **Sidebar toggle (`ctrl+s`)** — `KeyToggleSidebar` keybind and `sidebarHidden bool` field on `home` struct. When the sidebar is hidden, `sidebarWidth = 0` in `updateHandleWindowSizeEvent` and the sidebar view is omitted from `View()`. The `s` key (KeyFocusSidebar) does a two-step reveal: first press shows the sidebar, second press focuses it. Left arrow does the same when panel 1 is focused. **Preserve this toggle behavior** — do NOT remove it during the topic cleanup. Keep `KeyToggleSidebar`, `sidebarHidden`, and the two-step reveal logic in `KeyFocusSidebar` and `KeyLeft` handlers.

2. **Global background fill** — `ui.FillBackground()` in `ui/fill.go` paints `ColorBase` behind every terminal cell. Called at the end of `View()`. `termWidth`/`termHeight` fields on `home` track terminal dimensions. All sidebar styles now include `.Background(ColorBase)`. All menu styles include `.Background(ColorBase)`. Sidebar and menu `lipgloss.Place` calls use `lipgloss.WithWhitespaceBackground(ColorBase)`.

3. **Menu bar includes toggle sidebar** — `defaultMenuOptions` now includes `keys.KeyToggleSidebar`. `defaultSystemGroupSize` is `6` (not 5). The instance `systemGroup` in `addInstanceOptions()` starts with `keys.KeyToggleSidebar`. When repurposing keybinds and updating menu options, **keep `KeyToggleSidebar` in both arrays**.

4. **Sidebar toggle tests** — `app/app_test.go` has `TestSidebarToggle` with 7 subtests covering ctrl+s hide/show, focus transfer, two-step reveal via left/h/s keys. These tests construct `home` structs with `sidebarHidden` field. **These tests must still pass** after the topic removal refactor.

---

### Task 1: Plan State Schema Evolution

**Files:**
- Modify: `config/planstate/planstate.go`
- Modify: `config/planstate/planstate_test.go`
- Test: `config/planstate/planstate_test.go`

1. **Write the failing tests for the new schema and lifecycle**

   Replace `config/planstate/planstate_test.go` with:

   ```go
   package planstate

   import (
    	"os"
    	"path/filepath"
    	"testing"
    	"time"

    	"github.com/stretchr/testify/assert"
    	"github.com/stretchr/testify/require"
   )

   func TestLoadPlanCentricSchema(t *testing.T) {
    	dir := t.TempDir()
    	path := filepath.Join(dir, "plan-state.json")
    	require.NoError(t, os.WriteFile(path, []byte(`{
   		"2026-02-21-my-feature.md": {
   			"status": "planning",
   			"description": "refactor auth to JWT",
   			"branch": "plan/my-feature",
   			"created_at": "2026-02-21T14:30:00Z"
   		}
   	}`), 0o644))

    	ps, err := Load(dir)
    	require.NoError(t, err)

    	entry := ps.Plans["2026-02-21-my-feature.md"]
    	assert.Equal(t, StatusPlanning, entry.Status)
    	assert.Equal(t, "refactor auth to JWT", entry.Description)
    	assert.Equal(t, "plan/my-feature", entry.Branch)
    	assert.Equal(t, time.Date(2026, 2, 21, 14, 30, 0, 0, time.UTC), entry.CreatedAt)
   }

   func TestLoad_MigratesLegacyStatuses(t *testing.T) {
    	dir := t.TempDir()
    	path := filepath.Join(dir, "plan-state.json")
    	require.NoError(t, os.WriteFile(path, []byte(`{
   		"a.md": {"status": "in_progress"},
   		"b.md": {"status": "done"},
   		"c.md": {"status": "completed"}
   	}`), 0o644))

    	ps, err := Load(dir)
    	require.NoError(t, err)

    	assert.Equal(t, StatusImplementing, ps.Plans["a.md"].Status)
    	assert.Equal(t, StatusReviewing, ps.Plans["b.md"].Status)
    	assert.Equal(t, StatusFinished, ps.Plans["c.md"].Status)
   }

   func TestUnfinishedAndHistorySplit(t *testing.T) {
    	ps := &PlanState{
    		Dir: "/tmp",
    		Plans: map[string]PlanEntry{
    			"2026-02-20-old.md": {
    				Status:    StatusFinished,
    				CreatedAt: time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
    			},
    			"2026-02-21-new.md": {
    				Status:    StatusFinished,
    				CreatedAt: time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
    			},
    			"2026-02-21-active.md": {
    				Status: StatusImplementing,
    			},
    		},
    	}

    	unfinished := ps.Unfinished()
    	require.Len(t, unfinished, 1)
    	assert.Equal(t, "2026-02-21-active.md", unfinished[0].Filename)

    	history := ps.Finished()
    	require.Len(t, history, 2)
    	assert.Equal(t, "2026-02-21-new.md", history[0].Filename)
    	assert.Equal(t, "2026-02-20-old.md", history[1].Filename)
   }

   func TestCreatePlanEntry(t *testing.T) {
    	dir := t.TempDir()
    	ps, err := Load(dir)
    	require.NoError(t, err)

    	now := time.Date(2026, 2, 21, 16, 10, 0, 0, time.UTC)
    	require.NoError(t, ps.Create(
    		"2026-02-21-sidebar-refactor.md",
    		"convert sidebar to tree",
    		"plan/sidebar-refactor",
    		now,
    	))

    	reloaded, err := Load(dir)
    	require.NoError(t, err)
    	entry := reloaded.Plans["2026-02-21-sidebar-refactor.md"]
    	assert.Equal(t, StatusReady, entry.Status)
    	assert.Equal(t, "convert sidebar to tree", entry.Description)
    	assert.Equal(t, "plan/sidebar-refactor", entry.Branch)
    	assert.Equal(t, now, entry.CreatedAt)
   }
   ```

2. **Run the tests to confirm failure**

   Run:

   ```bash
   go test ./config/planstate/... -run 'TestLoadPlanCentricSchema|TestLoad_MigratesLegacyStatuses|TestUnfinishedAndHistorySplit|TestCreatePlanEntry' -v
   ```

   Expected: FAIL (missing `description`/`branch`/`created_at`, missing new statuses, missing `Finished()` and `Create()`).

3. **Implement the schema, lifecycle, and migration behavior**

   Replace `config/planstate/planstate.go` with:

   ```go
   package planstate

   import (
    	"encoding/json"
    	"errors"
    	"fmt"
    	"os"
    	"path/filepath"
    	"sort"
    	"strings"
    	"time"
   )

   type Status string

   const (
    	StatusReady        Status = "ready"
    	StatusPlanning     Status = "planning"
    	StatusImplementing Status = "implementing"
    	StatusReviewing    Status = "reviewing"
    	StatusFinished     Status = "finished"

    	// compatibility aliases for existing call sites
    	StatusInProgress Status = StatusImplementing
    	StatusDone       Status = StatusReviewing
    	StatusCompleted  Status = StatusFinished
   )

   type PlanEntry struct {
    	Status      Status    `json:"status"`
    	Description string    `json:"description,omitempty"`
    	Branch      string    `json:"branch,omitempty"`
    	CreatedAt   time.Time `json:"created_at,omitempty"`
   }

   type PlanState struct {
    	Dir   string
    	Plans map[string]PlanEntry
   }

   type PlanInfo struct {
    	Filename    string
    	Status      Status
    	Description string
    	Branch      string
    	CreatedAt   time.Time
   }

   const stateFile = "plan-state.json"

   func Load(dir string) (*PlanState, error) {
    	path := filepath.Join(dir, stateFile)
    	data, err := os.ReadFile(path)
    	if err != nil {
    		if errors.Is(err, os.ErrNotExist) {
    			return &PlanState{Dir: dir, Plans: make(map[string]PlanEntry)}, nil
    		}
    		return nil, fmt.Errorf("read plan state: %w", err)
    	}

    	var plans map[string]PlanEntry
    	if err := json.Unmarshal(data, &plans); err != nil {
    		return nil, fmt.Errorf("parse plan state: %w", err)
    	}
    	if plans == nil {
    		plans = make(map[string]PlanEntry)
    	}

    	for k, v := range plans {
    		v.Status = normalizeStatus(v.Status)
    		plans[k] = v
    	}

    	return &PlanState{Dir: dir, Plans: plans}, nil
   }

   func normalizeStatus(s Status) Status {
    	switch s {
    	case StatusReady, StatusPlanning, StatusImplementing, StatusReviewing, StatusFinished:
    		return s
    	case "in_progress":
    		return StatusImplementing
    	case "done":
    		return StatusReviewing
    	case "completed":
    		return StatusFinished
    	default:
    		return StatusReady
    	}
   }

   func (ps *PlanState) Unfinished() []PlanInfo {
    	result := make([]PlanInfo, 0, len(ps.Plans))
    	for filename, entry := range ps.Plans {
    		if normalizeStatus(entry.Status) == StatusFinished {
    			continue
    		}
    		result = append(result, PlanInfo{
    			Filename:    filename,
    			Status:      normalizeStatus(entry.Status),
    			Description: entry.Description,
    			Branch:      entry.Branch,
    			CreatedAt:   entry.CreatedAt,
    		})
    	}
    	sort.Slice(result, func(i, j int) bool {
    		return result[i].Filename < result[j].Filename
    	})
    	return result
   }

   func (ps *PlanState) Finished() []PlanInfo {
    	result := make([]PlanInfo, 0, len(ps.Plans))
    	for filename, entry := range ps.Plans {
    		if normalizeStatus(entry.Status) != StatusFinished {
    			continue
    		}
    		result = append(result, PlanInfo{
    			Filename:    filename,
    			Status:      StatusFinished,
    			Description: entry.Description,
    			Branch:      entry.Branch,
    			CreatedAt:   entry.CreatedAt,
    		})
    	}
    	sort.Slice(result, func(i, j int) bool {
    		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
    			return result[i].CreatedAt.After(result[j].CreatedAt)
    		}
    		return result[i].Filename > result[j].Filename
    	})
    	return result
   }

   func (ps *PlanState) IsDone(filename string) bool {
    	entry, ok := ps.Plans[filename]
    	if !ok {
    		return false
    	}
    	return normalizeStatus(entry.Status) == StatusReviewing
   }

   func (ps *PlanState) Create(filename, description, branch string, createdAt time.Time) error {
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

   func (ps *PlanState) SetStatus(filename string, status Status) error {
    	if ps.Plans == nil {
    		ps.Plans = make(map[string]PlanEntry)
    	}
    	entry := ps.Plans[filename]
    	entry.Status = normalizeStatus(status)
    	ps.Plans[filename] = entry
    	return ps.save()
   }

   func DisplayName(filename string) string {
    	name := strings.TrimSuffix(filename, ".md")
    	if len(name) > 11 && name[4] == '-' && name[7] == '-' && name[10] == '-' {
    		name = name[11:]
    	}
    	return name
   }

   func (ps *PlanState) save() error {
    	data, err := json.MarshalIndent(ps.Plans, "", "  ")
    	if err != nil {
    		return fmt.Errorf("marshal plan state: %w", err)
    	}
    	path := filepath.Join(ps.Dir, stateFile)
    	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
    		return fmt.Errorf("write plan state: %w", err)
    	}
    	return nil
   }
   ```

4. **Run plan-state tests again**

   Run:

   ```bash
   go test ./config/planstate/... -v
   ```

   Expected: PASS.

5. **Commit**

   ```bash
   git add config/planstate/planstate.go config/planstate/planstate_test.go
   git commit -m "feat(planstate): adopt plan-centric schema and lifecycle"
   ```

---

### Task 2: Remove Topic System End-to-End

**Files:**
- Modify: `config/state.go`
- Create: `config/state_test.go`
- Modify: `session/storage.go`
- Delete: `session/topic.go`
- Delete: `session/topic_storage.go`
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Test: `config/state_test.go`
- Test: `app/app_test.go`

1. **Write failing state migration tests before removing topic storage**

   Create `config/state_test.go`:

   ```go
   package config

   import (
    	"encoding/json"
    	"os"
    	"path/filepath"
    	"testing"

    	"github.com/stretchr/testify/assert"
    	"github.com/stretchr/testify/require"
   )

   func TestDefaultState_NoTopicsField(t *testing.T) {
    	s := DefaultState()
    	raw, err := json.Marshal(s)
    	require.NoError(t, err)
    	assert.NotContains(t, string(raw), `"topics"`)
   }

   func TestLoadState_IgnoresLegacyTopicsField(t *testing.T) {
    	home := t.TempDir()
    	t.Setenv("HOME", home)

    	configDir := filepath.Join(home, ".klique")
    	require.NoError(t, os.MkdirAll(configDir, 0o755))

    	legacy := `{
   		"help_screens_seen": 0,
   		"instances": [],
   		"topics": [{"name":"legacy-topic"}],
   		"recent_repos": []
   	}`
    	require.NoError(t, os.WriteFile(filepath.Join(configDir, StateFileName), []byte(legacy), 0o644))

    	s := LoadState()
    	assert.NotNil(t, s)
    	assert.Equal(t, json.RawMessage("[]"), s.GetInstances())
   }
   ```

2. **Run tests and capture the failing baseline**

   Run:

   ```bash
   go test ./config/... -run 'TestDefaultState_NoTopicsField|TestLoadState_IgnoresLegacyTopicsField' -v
   ```

   Expected: FAIL (`DefaultState` still includes topics payload).

3. **Remove topic persistence from config state**

   Update `config/state.go`:

   ```go
   // Remove TopicStorage and keep StateManager minimal.
   type StateManager interface {
    	InstanceStorage
    	AppState
   }

   type State struct {
    	HelpScreensSeen uint32          `json:"help_screens_seen"`
    	InstancesData   json.RawMessage `json:"instances"`
    	RecentRepos     []string        `json:"recent_repos,omitempty"`
   }

   func DefaultState() *State {
    	return &State{
    		HelpScreensSeen: 0,
    		InstancesData:   json.RawMessage("[]"),
    	}
   }

   // Remove SaveTopics/GetTopics methods entirely.
   ```

4. **Remove topic serialization from session storage and delete topic types**

   Update `session/storage.go` by deleting `SaveTopics` and `LoadTopics`, and remove topic-only types usage.

   Delete files:

   ```diff
   *** Delete File: session/topic.go
   *** Delete File: session/topic_storage.go
   ```

5. **Remove topic fields/states and topic flows from app layer**

   Apply these structural changes:

   - In `app/app.go`: remove `stateNewTopic`, `stateNewTopicConfirm`, `stateRenameTopic`, `topics`, `allTopics`, `pendingTopicName`, topic loading in `newHome`, and `saveAllTopics()` call in `handleQuit`. **Keep** `sidebarHidden`, `termWidth`, `termHeight` fields — they are unrelated to topics.
   - In `app/app_input.go`: remove new-topic, rename-topic, move-to-topic, and kill-all-topic state handlers. **Keep** `KeyToggleSidebar`, `KeyFocusSidebar` two-step reveal, and `KeyLeft` sidebar-reveal logic — they are unrelated to topics.
   - In `app/app_actions.go`: remove `kill_all_in_topic`, `delete_topic_and_instances`, `delete_topic`, `rename_topic`, `push_topic` actions.
   - In `app/app_state.go`: remove `filterTopicsByRepo`, `saveAllTopics`, and topic-based filtering/counting.
   - In `app/app.go` `View()`: keep the `sidebarHidden` conditional in the layout and the `FillBackground` call at the end.

   Use this replacement for `updateSidebarItems` in `app/app_state.go`:

   ```go
   func (m *home) updateSidebarItems() {
    	planCounts := make(map[string]int)
    	ungroupedCount := 0
    	for _, inst := range m.list.GetInstances() {
    		if inst.PlanFile == "" {
    			ungroupedCount++
    			continue
    		}
    		planCounts[inst.PlanFile]++
    	}
    	m.sidebar.SetPlanCounts(planCounts, ungroupedCount)
   }
   ```

6. **Run focused tests for config/session/app after topic removal**

   Run:

   ```bash
   go test ./config/... ./session/... ./app/... -v
   ```

   Expected: PASS (no topic APIs or topic state references remain).

7. **Commit**

   ```bash
   git add config/state.go config/state_test.go
   git add session/storage.go
   git add app/app.go app/app_input.go app/app_state.go app/app_actions.go
   git rm session/topic.go session/topic_storage.go
   git commit -m "refactor(app): remove topic system and storage plumbing"
   ```

---

### Task 3: Sidebar Tree Nodes and Plan History

**Files:**
- Modify: `ui/sidebar.go`
- Modify: `ui/sidebar_test.go`
- Modify: `app/app_state.go`
- Modify: `app/app_input.go`
- Test: `ui/sidebar_test.go`

1. **Write failing sidebar tree tests first**

   Replace `ui/sidebar_test.go` with:

   ```go
   package ui

   import (
    	"testing"
    	"time"

    	"github.com/kastheco/klique/config/planstate"
   )

   func TestSidebarBuildRows_PlanTreeAndHistory(t *testing.T) {
    	s := NewSidebar()

    	s.SetPlans(
    		[]PlanDisplay{{Filename: "2026-02-21-active.md", Status: string(planstate.StatusImplementing)}},
    		[]PlanDisplay{{Filename: "2026-02-20-finished.md", Status: string(planstate.StatusFinished), CreatedAt: time.Now()}},
    	)
    	s.SetPlanCounts(map[string]int{"2026-02-21-active.md": 2}, 1)

    	if !s.HasRowID(SidebarPlanPrefix + "2026-02-21-active.md") {
    		t.Fatalf("expected active plan header row")
    	}
    	if !s.HasRowID(SidebarPlanHistoryToggle) {
    		t.Fatalf("expected plan history toggle row")
    	}
   }

   func TestSidebarToggleExpand_OnPlanHeader(t *testing.T) {
    	s := NewSidebar()
    	s.SetPlans([]PlanDisplay{{Filename: "2026-02-21-active.md", Status: string(planstate.StatusPlanning)}}, nil)
    	s.SetPlanCounts(map[string]int{"2026-02-21-active.md": 1}, 0)

    	s.SelectByID(SidebarPlanPrefix + "2026-02-21-active.md")
    	if !s.ToggleSelectedExpand() {
    		t.Fatalf("expected plan expansion toggle to return true")
    	}
    	if !s.HasRowID(SidebarPlanStagePrefix + "2026-02-21-active.md::plan") {
    		t.Fatalf("expected plan stage row after expansion")
    	}
   }

   func TestSidebarSelectedStageMetadata(t *testing.T) {
    	s := NewSidebar()
    	s.SetPlans([]PlanDisplay{{Filename: "2026-02-21-active.md", Status: string(planstate.StatusReviewing)}}, nil)
    	s.SetPlanCounts(map[string]int{"2026-02-21-active.md": 1}, 0)

    	s.SelectByID(SidebarPlanPrefix + "2026-02-21-active.md")
    	s.ToggleSelectedExpand()
    	s.SelectByID(SidebarPlanStagePrefix + "2026-02-21-active.md::review")

    	planFile, stage, ok := s.GetSelectedPlanStage()
    	if !ok {
    		t.Fatalf("expected stage selection")
    	}
    	if planFile != "2026-02-21-active.md" || stage != "review" {
    		t.Fatalf("got (%q, %q), want (%q, %q)", planFile, stage, "2026-02-21-active.md", "review")
    	}
   }
   ```

2. **Run sidebar tests and confirm failure**

   Run:

   ```bash
   go test ./ui/... -run 'TestSidebarBuildRows_PlanTreeAndHistory|TestSidebarToggleExpand_OnPlanHeader|TestSidebarSelectedStageMetadata' -v
   ```

   Expected: FAIL (`SetPlans` signature and tree APIs do not exist yet).

3. **Implement tree rows, stage rows, and history section in sidebar**

   **IMPORTANT:** All existing sidebar styles now include `.Background(ColorBase)` for the Rosé Pine Moon theme. When adding new styles for plan tree items (stage rows, history items), always include `.Background(ColorBase)` to match. The sidebar's final `lipgloss.Place` call uses `lipgloss.WithWhitespaceBackground(ColorBase)`.

   In `ui/sidebar.go`, add these core types/APIs:

   ```go
   const (
    	SidebarPlanHistoryToggle = "__plan_history_toggle__"
    	SidebarPlanStagePrefix   = "__plan_stage__"
   )

   type PlanDisplay struct {
    	Filename    string
    	Status      string
    	Description string
    	Branch      string
    	CreatedAt   time.Time
   }

   type sidebarRowKind int

   const (
    	rowItem sidebarRowKind = iota
    	rowSection
    	rowPlanHeader
    	rowPlanStage
    	rowHistoryToggle
   )

   type sidebarRow struct {
    	Kind      sidebarRowKind
    	ID        string
    	Label     string
    	PlanFile  string
    	Stage     string
    	Locked    bool
    	Done      bool
    	Active    bool
    	Count     int
    	Collapsed bool
   }

   func (s *Sidebar) SetPlans(active []PlanDisplay, history []PlanDisplay) { /* store + rebuildRows() */ }
   func (s *Sidebar) SetPlanCounts(countByPlan map[string]int, ungroupedCount int) { /* store + rebuildRows() */ }
   func (s *Sidebar) ToggleSelectedExpand() bool { /* plan header/history toggle */ }
   func (s *Sidebar) GetSelectedPlanStage() (planFile, stage string, ok bool) { /* parse row */ }
   func (s *Sidebar) HasRowID(id string) bool { /* test helper */ }
   func (s *Sidebar) SelectByID(id string) bool { /* test helper */ }
   ```

   Implement progressive stage state mapping:

   ```go
   func stageFlags(status planstate.Status, stage string) (done, active, locked bool) {
    	switch stage {
    	case "plan":
    		return status == planstate.StatusImplementing || status == planstate.StatusReviewing || status == planstate.StatusFinished,
    			status == planstate.StatusPlanning,
    			false
    	case "implement":
    		if status == planstate.StatusReady || status == planstate.StatusPlanning {
    			return false, false, true
    		}
    		return status == planstate.StatusReviewing || status == planstate.StatusFinished,
    			status == planstate.StatusImplementing,
    			false
    	case "review":
    		if status == planstate.StatusReady || status == planstate.StatusPlanning || status == planstate.StatusImplementing {
    			return false, false, true
    		}
    		return status == planstate.StatusFinished,
    			status == planstate.StatusReviewing,
    			false
    	case "finished":
    		return status == planstate.StatusFinished, false, status != planstate.StatusFinished
    	default:
    		return false, false, true
    	}
   }
   ```

4. **Wire app plan lists into active + history sidebar sections**

   Update `app/app_state.go` `updateSidebarPlans()`:

   ```go
   func (m *home) updateSidebarPlans() {
    	if m.planState == nil {
    		m.sidebar.SetPlans(nil, nil)
    		return
    	}

    	unfinished := m.planState.Unfinished()
    	history := m.planState.Finished()

    	active := make([]ui.PlanDisplay, 0, len(unfinished))
    	for _, p := range unfinished {
    		active = append(active, ui.PlanDisplay{
    			Filename: p.Filename, Status: string(p.Status), Description: p.Description, Branch: p.Branch, CreatedAt: p.CreatedAt,
    		})
    	}

    	done := make([]ui.PlanDisplay, 0, len(history))
    	for _, p := range history {
    		done = append(done, ui.PlanDisplay{
    			Filename: p.Filename, Status: string(p.Status), Description: p.Description, Branch: p.Branch, CreatedAt: p.CreatedAt,
    		})
    	}

    	m.sidebar.SetPlans(active, done)
   }
   ```

5. **Run sidebar tests after implementation**

   Run:

   ```bash
   go test ./ui/... -v
   ```

   Expected: PASS.

6. **Commit**

   ```bash
   git add ui/sidebar.go ui/sidebar_test.go app/app_state.go app/app_input.go
   git commit -m "feat(ui): render expandable plan tree and plan history"
   ```

---

### Task 4: Keybind Repurpose and Two-Step New Plan Flow

**Files:**
- Modify: `keys/keys.go`
- Create: `keys/keys_test.go`
- Modify: `ui/menu.go`
- Create: `ui/menu_test.go`
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/help.go`
- Test: `keys/keys_test.go`
- Test: `ui/menu_test.go`
- Test: `app/app_test.go`

1. **Write failing tests for key remap and menu updates**

   Create `keys/keys_test.go`:

   ```go
   package keys

   import "testing"

   func TestGlobalKeyMap_PRepurposedToNewPlan(t *testing.T) {
    	if GlobalKeyStringsMap["p"] != KeyNewPlan {
    		t.Fatalf("p should map to KeyNewPlan")
    	}
   }

   func TestGlobalKeyMap_TopicKeysRemoved(t *testing.T) {
    	if _, ok := GlobalKeyStringsMap["T"]; ok {
    		t.Fatalf("T key should be removed")
    	}
    	if _, ok := GlobalKeyStringsMap["X"]; ok {
    		t.Fatalf("X key should be removed")
    	}
   }
   ```

   Create `ui/menu_test.go`:

   ```go
   package ui

   import (
    	"testing"

    	"github.com/kastheco/klique/keys"
   )

   func TestDefaultMenuIncludesNewPlanAndNoTopicActions(t *testing.T) {
    	m := NewMenu()
    	m.SetState(StateEmpty)

    	foundNewPlan := false
    	foundKillAllTopic := false
    	for _, opt := range m.options {
    		if opt == keys.KeyNewPlan {
    			foundNewPlan = true
    		}
    		if opt == keys.KeyKillAllInTopic {
    			foundKillAllTopic = true
    		}
    	}

    	if !foundNewPlan {
    		t.Fatalf("default menu should include new-plan key")
    	}
    	if foundKillAllTopic {
    		t.Fatalf("default menu should not include topic kill action")
    	}
   }
   ```

2. **Run tests and verify they fail before code changes**

   Run:

   ```bash
   go test ./keys/... ./ui/... -run 'TestGlobalKeyMap_PRepurposedToNewPlan|TestGlobalKeyMap_TopicKeysRemoved|TestDefaultMenuIncludesNewPlanAndNoTopicActions' -v
   ```

   Expected: FAIL (`p` still maps to push, topic keys still exist).

3. **Repurpose key definitions and remove topic keybinds**

   Update `keys/keys.go`:

   ```go
   const (
    	// ...
    	KeyNewPlan
    	// remove KeyNewTopic and KeyKillAllInTopic
    	// KEEP KeyToggleSidebar — it was recently added for ctrl+s sidebar toggle
   )

   var GlobalKeyStringsMap = map[string]KeyName{
    	// ...
    	"p":      KeyNewPlan,    // was KeySubmit (push branch)
    	"ctrl+s": KeyToggleSidebar, // KEEP — recent addition
    	// remove "T" and "X"
    	// remove "p": KeySubmit (push now happens via context menu / end-of-implementation flow)
   }

   var GlobalkeyBindings = map[KeyName]key.Binding{
    	// ...
    	KeyNewPlan: key.NewBinding(
    		key.WithKeys("p"),
    		key.WithHelp("p", "new plan"),
    	),
    	// KEEP KeyToggleSidebar binding — do not remove
   }
   ```

4. **Implement the two-step plan creation state flow in app input handling**

   Add states in `app/app.go`:

   ```go
   const (
    	// ...
    	stateNewPlanName
    	stateNewPlanDescription
   )

   type home struct {
    	// ...
    	pendingPlanName string
   }
   ```

   Add key handling and submit flow in `app/app_input.go`:

   ```go
   case keys.KeyNewPlan:
    	m.state = stateNewPlanName
    	m.textInputOverlay = overlay.NewTextInputOverlay("Plan name", "")
    	m.textInputOverlay.SetSize(60, 3)
    	return m, nil

   if m.state == stateNewPlanName {
    	shouldClose := m.textInputOverlay.HandleKeyPress(msg)
    	if shouldClose {
    		if !m.textInputOverlay.IsSubmitted() {
    			m.textInputOverlay = nil
    			m.state = stateDefault
    			return m, tea.WindowSize()
    		}
    		name := strings.TrimSpace(m.textInputOverlay.GetValue())
    		if name == "" {
    			return m, m.handleError(fmt.Errorf("plan name cannot be empty"))
    		}
    		m.pendingPlanName = name
    		m.state = stateNewPlanDescription
    		m.textInputOverlay = overlay.NewTextInputOverlay("Plan description", "")
    		m.textInputOverlay.SetSize(80, 6)
    	}
    	return m, nil
   }

   if m.state == stateNewPlanDescription {
    	shouldClose := m.textInputOverlay.HandleKeyPress(msg)
    	if shouldClose {
    		if !m.textInputOverlay.IsSubmitted() {
    			m.pendingPlanName = ""
    			m.textInputOverlay = nil
    			m.state = stateDefault
    			return m, tea.WindowSize()
    		}

    		description := strings.TrimSpace(m.textInputOverlay.GetValue())
    		if err := m.createPlanEntry(m.pendingPlanName, description); err != nil {
    			return m, m.handleError(err)
    		}

    		m.pendingPlanName = ""
    		m.textInputOverlay = nil
    		m.state = stateDefault
    		m.loadPlanState()
    		m.updateSidebarPlans()
    		m.updateSidebarItems()
    		return m, tea.WindowSize()
    	}
    	return m, nil
   }
   ```

   Add helper in `app/app_state.go`:

   ```go
   func (m *home) createPlanEntry(name, description string) error {
    	if m.planState == nil {
    		ps, err := planstate.Load(m.planStateDir)
    		if err != nil {
    			return err
    		}
    		m.planState = ps
    	}

    	slug := slugifyPlanName(name)
    	filename := fmt.Sprintf("%s-%s.md", time.Now().UTC().Format("2006-01-02"), slug)
    	branch := "plan/" + slug
    	return m.planState.Create(filename, description, branch, time.Now().UTC())
   }

   func slugifyPlanName(name string) string {
    	name = strings.ToLower(strings.TrimSpace(name))
    	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
    	return strings.Trim(name, "-")
   }
   ```

5. **Update menu bar and help text to match keybind changes**

   In `ui/menu.go`, include `keys.KeyNewPlan` and remove topic actions. **Keep `keys.KeyToggleSidebar`** — it was added recently and must be preserved:

   ```go
   var defaultMenuOptions = []keys.KeyName{
    	keys.KeyNew,
    	keys.KeyNewPlan,
    	keys.KeySearch,
    	keys.KeyToggleSidebar,
    	keys.KeySpace,
    	keys.KeyRepoSwitch,
    	keys.KeyHelp,
    	keys.KeyQuit,
   }
   var defaultSystemGroupSize = 7 // ctrl+s toggle sidebar, / search, space actions, R repo switch, ? help, q quit + new plan
   ```

   In `addInstanceOptions()`, update the systemGroup to remove topic kill and keep toggle:

   ```go
   systemGroup := []keys.KeyName{keys.KeyToggleSidebar, keys.KeySearch, keys.KeyRepoSwitch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}
   ```

   In `app/help.go`, replace topic section with plan-centric entries:

   ```go
   headerStyle.Render("\uf03a Plans:"),
   keyStyle.Render("p")+descStyle.Render("         - Create a new plan"),
   keyStyle.Render("space")+descStyle.Render("     - Expand/collapse selected plan"),
   keyStyle.Render("↵/o")+descStyle.Render("       - Plan menu / run selected stage"),
   keyStyle.Render("/")+descStyle.Render("         - Search plans and instances"),
   ```

6. **Run tests for keys/menu/app plan creation flow**

   Run:

   ```bash
   go test ./keys/... ./ui/... ./app/... -v
   ```

   Expected: PASS.

7. **Commit**

   ```bash
   git add keys/keys.go keys/keys_test.go
   git add ui/menu.go ui/menu_test.go
   git add app/app.go app/app_input.go app/app_state.go app/help.go app/app_test.go
   git commit -m "feat(app): repurpose p key to create plans with two-step prompt"
   ```

---

### Task 5: Plan Header/Sub-Item Interactions and Final Verification

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_actions.go`
- Modify: `ui/sidebar.go`
- Modify: `ui/sidebar_test.go`
- Modify: `app/app_test.go`
- Test: `app/app_test.go`
- Test: `ui/sidebar_test.go`

1. **Write failing interaction tests for Enter/Space behavior on plan nodes**

   Add to `app/app_test.go`:

   ```go
   func TestSidebarSpaceTogglesPlanExpansion(t *testing.T) {
    	h := newMinimalHomeForInputTests(t)
    	h.sidebar.SetPlans([]ui.PlanDisplay{{Filename: "2026-02-21-tree.md", Status: "planning"}}, nil)
    	h.sidebar.SetPlanCounts(map[string]int{"2026-02-21-tree.md": 1}, 0)
    	h.sidebar.SelectByID(ui.SidebarPlanPrefix + "2026-02-21-tree.md")

    	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeySpace})

    	if !h.sidebar.HasRowID(ui.SidebarPlanStagePrefix + "2026-02-21-tree.md::plan") {
    		t.Fatalf("expected expanded plan stages after pressing space")
    	}
   }

   func TestSidebarEnterOnLockedStageShowsErrorToast(t *testing.T) {
    	h := newMinimalHomeForInputTests(t)
    	h.sidebar.SetPlans([]ui.PlanDisplay{{Filename: "2026-02-21-tree.md", Status: "ready"}}, nil)
    	h.sidebar.SetPlanCounts(map[string]int{"2026-02-21-tree.md": 1}, 0)
    	h.sidebar.SelectByID(ui.SidebarPlanPrefix + "2026-02-21-tree.md")
    	h.sidebar.ToggleSelectedExpand()
    	h.sidebar.SelectByID(ui.SidebarPlanStagePrefix + "2026-02-21-tree.md::implement")

    	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

    	if !h.toastManager.HasActiveToasts() {
    		t.Fatalf("expected locked-stage toast")
    	}
   }
   ```

2. **Run interaction tests to confirm failure before wiring behavior**

   Run:

   ```bash
   go test ./app/... -run 'TestSidebarSpaceTogglesPlanExpansion|TestSidebarEnterOnLockedStageShowsErrorToast' -v
   ```

   Expected: FAIL (space still opens context menu and Enter behavior is not stage-aware).

3. **Wire Space/Enter to plan header and stage actions**

   Update `app/app_input.go` key handlers:

   ```go
   case keys.KeySpace:
    	if m.focusedPanel == 0 && m.sidebar.ToggleSelectedExpand() {
    		return m, nil
    	}
    	return m.openContextMenu()

   case keys.KeyEnter:
    	if m.focusedPanel == 0 {
    		if planFile, stage, ok := m.sidebar.GetSelectedPlanStage(); ok {
    			return m.triggerPlanStage(planFile, stage)
    		}
    		if m.sidebar.IsSelectedPlanHeader() {
    			return m.openContextMenu()
    		}
    	}
    	// existing instance attach flow continues here
   ```

   Add action gate helper:

   ```go
   func (m *home) triggerPlanStage(planFile, stage string) (tea.Model, tea.Cmd) {
    	entry, ok := m.planState.Plans[planFile]
    	if !ok {
    		return m, m.handleError(fmt.Errorf("missing plan state for %s", planFile))
    	}

    	if lockedStage(entry.Status, stage) {
    		prev := map[string]string{"implement": "plan", "review": "implement", "finished": "review"}[stage]
    		m.toastManager.Error(fmt.Sprintf("complete %s first", prev))
    		return m, m.toastTickCmd()
    	}

    	// Plan 1 behavior: wire UI + status transitions only. Session spawning arrives in Plan 2.
    	switch stage {
    	case "plan":
    		_ = m.planState.SetStatus(planFile, planstate.StatusPlanning)
    	case "implement":
    		_ = m.planState.SetStatus(planFile, planstate.StatusImplementing)
    	case "review":
    		_ = m.planState.SetStatus(planFile, planstate.StatusReviewing)
    	case "finished":
    		_ = m.planState.SetStatus(planFile, planstate.StatusFinished)
    	}

    	m.loadPlanState()
    	m.updateSidebarPlans()
    	m.updateSidebarItems()
    	return m, nil
   }
   ```

4. **Replace sidebar context menu entries with plan-centric actions**

   In `app/app_actions.go` and plan-side branch of `openContextMenu()`/`handleRightClick()` use:

   ```go
   items := []overlay.ContextMenuItem{
    	{Label: "Modify plan", Action: "modify_plan"},
    	{Label: "Start over", Action: "start_over_plan"},
    	{Label: "Kill running instances", Action: "kill_plan_instances"},
    	{Label: "Push branch", Action: "push_plan_branch"},
    	{Label: "Create PR", Action: "create_plan_pr"},
    	{Label: "Rename plan", Action: "rename_plan"},
    	{Label: "Delete plan", Action: "delete_plan"},
    	{Label: "View design doc", Action: "view_design_doc"},
   }
   ```

   For Plan 1, implement actions as safe stubs where needed (toast + no side effects) except `view_design_doc` (real implementation opens companion `*-design.md` in preview pane if file exists).

5. **Run full verification suite for this feature slice**

   Run:

   ```bash
   go test ./config/planstate/... -v
   go test ./keys/... -v
   go test ./ui/... -v
   go test ./app/... -v
   go test ./... 
   ```

   Expected:
- package-focused tests pass
- full repository test run passes without topic references

6. **Commit**

   ```bash
   git add app/app_input.go app/app_actions.go app/app_test.go
   git add ui/sidebar.go ui/sidebar_test.go
   git commit -m "feat(sidebar): add plan header and stage interactions"
   ```

---

### Notes for the Executor

- Keep the implementation strictly within Plan 1 scope (no agent spawning changes, no `AgentType`, no `m` reassignment behavior beyond removing topic logic).
- If a test depends on Plan 2 behavior, stub with explicit TODO and assert Plan 1 behavior only.
- Preserve backward compatibility for legacy `plan-state.json` statuses during load migration.
- Do not reintroduce topic structs, topic storage fields, or topic keybinds.
