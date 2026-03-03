# Topics as Collision Domains — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reintroduce topics as lightweight collision domains alongside plans. Topics are named groupings with concurrency gates — no git state, no worktrees. Plans own their own branches. Replaces the topic-removal approach from the plan-centric sidebar design.

**Architecture:** Evolve plan-state.json to store both topics (name + created_at) and a `topic` field on plan entries. Refactor the sidebar to render a three-level tree (topic → plan → lifecycle stages). Replace the old topic system (session/topic.go, worktree-owning topics) with this lightweight model. Add a concurrency gate that warns when running two coders in the same topic.

**Tech Stack:** Go, bubbletea, lipgloss, bubblezone

**Important — Codebase Context:**

1. **Sidebar toggle (`ctrl+s`)** — `KeyToggleSidebar` keybind and `sidebarHidden bool` on `home`. PRESERVE this behavior throughout all tasks.
2. **Global background fill** — `ui.FillBackground()` paints `ColorBase`. All sidebar/menu styles include `.Background(ColorBase)`. PRESERVE this.
3. **Menu bar includes toggle sidebar** — `defaultMenuOptions` includes toggle. PRESERVE.
4. **Sidebar toggle tests** — `app/app_test.go` `TestSidebarToggle` with 7 subtests. These MUST still pass.
5. **Existing plan-state.json** — Has entries with `"status": "ready"`, `"done"`, `"completed"`, `"in_progress"`, `"reviewing"`. The `PlanEntry` struct currently has `Status` and `Implemented` fields only. Migration must handle all existing statuses.

---

### Task 1: Evolve Plan State Schema with Topic Support

**Files:**
- Modify: `config/planstate/planstate.go`
- Modify: `config/planstate/planstate_test.go`
- Test: `config/planstate/planstate_test.go`

1. **Write failing tests for the new schema with topic support**

   Add to `config/planstate/planstate_test.go`:

   ```go
   func TestPlanEntryWithTopic(t *testing.T) {
   	dir := t.TempDir()
   	path := filepath.Join(dir, "plan-state.json")
   	require.NoError(t, os.WriteFile(path, []byte(`{
   		"topics": {
   			"ui-refactor": {"created_at": "2026-02-21T14:30:00Z"}
   		},
   		"plans": {
   			"2026-02-21-sidebar.md": {
   				"status": "in_progress",
   				"description": "refactor sidebar",
   				"branch": "plan/sidebar",
   				"topic": "ui-refactor",
   				"created_at": "2026-02-21T14:30:00Z"
   			}
   		}
   	}`), 0o644))

   	ps, err := Load(dir)
   	require.NoError(t, err)

   	entry := ps.Plans["2026-02-21-sidebar.md"]
   	assert.Equal(t, StatusInProgress, entry.Status)
   	assert.Equal(t, "refactor sidebar", entry.Description)
   	assert.Equal(t, "plan/sidebar", entry.Branch)
   	assert.Equal(t, "ui-refactor", entry.Topic)

   	topics := ps.Topics()
   	require.Len(t, topics, 1)
   	assert.Equal(t, "ui-refactor", topics[0].Name)
   }

   func TestPlansByTopic(t *testing.T) {
   	ps := &PlanState{
   		Dir: "/tmp",
   		Plans: map[string]PlanEntry{
   			"a.md": {Status: StatusInProgress, Topic: "ui"},
   			"b.md": {Status: StatusReady, Topic: "ui"},
   			"c.md": {Status: StatusReady, Topic: ""},
   		},
   		TopicEntries: map[string]TopicEntry{
   			"ui": {CreatedAt: time.Now()},
   		},
   	}

   	byTopic := ps.PlansByTopic("ui")
   	assert.Len(t, byTopic, 2)

   	ungrouped := ps.UngroupedPlans()
   	assert.Len(t, ungrouped, 1)
   	assert.Equal(t, "c.md", ungrouped[0].Filename)
   }

   func TestCreatePlanWithTopic(t *testing.T) {
   	dir := t.TempDir()
   	ps, err := Load(dir)
   	require.NoError(t, err)

   	now := time.Now().UTC()
   	require.NoError(t, ps.Create("2026-02-21-feat.md", "a feature", "plan/feat", "my-topic", now))

   	// Topic should be auto-created
   	topics := ps.Topics()
   	require.Len(t, topics, 1)
   	assert.Equal(t, "my-topic", topics[0].Name)

   	entry := ps.Plans["2026-02-21-feat.md"]
   	assert.Equal(t, "my-topic", entry.Topic)
   	assert.Equal(t, StatusReady, entry.Status)
   }

   func TestCreatePlanUngrouped(t *testing.T) {
   	dir := t.TempDir()
   	ps, err := Load(dir)
   	require.NoError(t, err)

   	now := time.Now().UTC()
   	require.NoError(t, ps.Create("2026-02-21-fix.md", "a fix", "plan/fix", "", now))

   	topics := ps.Topics()
   	assert.Len(t, topics, 0)

   	entry := ps.Plans["2026-02-21-fix.md"]
   	assert.Equal(t, "", entry.Topic)
   }

   func TestHasRunningCoderInTopic(t *testing.T) {
   	ps := &PlanState{
   		Dir: "/tmp",
   		Plans: map[string]PlanEntry{
   			"a.md": {Status: StatusInProgress, Topic: "ui"},
   			"b.md": {Status: StatusReady, Topic: "ui"},
   		},
   	}

   	running, planFile := ps.HasRunningCoderInTopic("ui", "b.md")
   	assert.True(t, running)
   	assert.Equal(t, "a.md", planFile)

   	running, _ = ps.HasRunningCoderInTopic("ui", "a.md")
   	assert.False(t, running, "should not flag self")

   	running, _ = ps.HasRunningCoderInTopic("other", "x.md")
   	assert.False(t, running)
   }

   func TestLoadLegacyFlatFormat(t *testing.T) {
   	dir := t.TempDir()
   	path := filepath.Join(dir, "plan-state.json")
   	// Legacy format: flat map without "plans"/"topics" wrapper
   	require.NoError(t, os.WriteFile(path, []byte(`{
   		"2026-02-20-old.md": {"status": "done"},
   		"2026-02-21-active.md": {"status": "in_progress"}
   	}`), 0o644))

   	ps, err := Load(dir)
   	require.NoError(t, err)

   	assert.Len(t, ps.Plans, 2)
   	assert.Equal(t, StatusDone, ps.Plans["2026-02-20-old.md"].Status)
   	assert.Equal(t, StatusInProgress, ps.Plans["2026-02-21-active.md"].Status)
   }
   ```

2. **Run tests to confirm failure**

   Run:

   ```bash
   go test ./config/planstate/... -run 'TestPlanEntryWithTopic|TestPlansByTopic|TestCreatePlanWithTopic|TestCreatePlanUngrouped|TestHasRunningCoderInTopic|TestLoadLegacyFlatFormat' -v
   ```

   Expected: FAIL (missing `Topic`, `Description`, `Branch`, `CreatedAt` fields, missing `TopicEntry`, `TopicEntries`, `Topics()`, `PlansByTopic()`, `UngroupedPlans()`, `HasRunningCoderInTopic()`, `Create()`, and new JSON format).

3. **Implement the evolved schema**

   Replace `config/planstate/planstate.go` with the new schema. Key changes:

   - `PlanEntry` gains `Description`, `Branch`, `Topic`, `CreatedAt` fields
   - New `TopicEntry` struct with just `CreatedAt`
   - `PlanState` gains `TopicEntries map[string]TopicEntry`
   - New JSON format wraps plans under `"plans"` key and topics under `"topics"` key
   - `Load()` detects legacy flat format (no `"plans"` key) and migrates
   - New methods: `Topics() []TopicInfo`, `PlansByTopic(topic string) []PlanInfo`, `UngroupedPlans() []PlanInfo`, `HasRunningCoderInTopic(topic, excludePlan string) (bool, string)`, `Create(filename, description, branch, topic string, createdAt time.Time) error`
   - `save()` writes the new wrapped format
   - Keep existing `Unfinished()`, `IsDone()`, `SetStatus()`, `DisplayName()` working

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
   	StatusInProgress   Status = "in_progress"
   	StatusDone         Status = "done"
   	StatusReviewing    Status = "reviewing"
   	StatusCompleted    Status = "completed"
   )

   type PlanEntry struct {
   	Status      Status    `json:"status"`
   	Description string    `json:"description,omitempty"`
   	Branch      string    `json:"branch,omitempty"`
   	Topic       string    `json:"topic,omitempty"`
   	CreatedAt   time.Time `json:"created_at,omitempty"`
   	Implemented string    `json:"implemented,omitempty"`
   }

   type TopicEntry struct {
   	CreatedAt time.Time `json:"created_at"`
   }

   type PlanState struct {
   	Dir          string
   	Plans        map[string]PlanEntry
   	TopicEntries map[string]TopicEntry
   }

   type PlanInfo struct {
   	Filename    string
   	Status      Status
   	Description string
   	Branch      string
   	Topic       string
   	CreatedAt   time.Time
   }

   type TopicInfo struct {
   	Name      string
   	CreatedAt time.Time
   }

   const stateFile = "plan-state.json"

   // wrappedFormat is the new on-disk format with "plans" and "topics" keys.
   type wrappedFormat struct {
   	Topics map[string]TopicEntry `json:"topics,omitempty"`
   	Plans  map[string]PlanEntry  `json:"plans"`
   }

   func Load(dir string) (*PlanState, error) {
   	path := filepath.Join(dir, stateFile)
   	data, err := os.ReadFile(path)
   	if err != nil {
   		if errors.Is(err, os.ErrNotExist) {
   			return &PlanState{Dir: dir, Plans: make(map[string]PlanEntry), TopicEntries: make(map[string]TopicEntry)}, nil
   		}
   		return nil, fmt.Errorf("read plan state: %w", err)
   	}

   	// Try new wrapped format first
   	var wrapped wrappedFormat
   	if err := json.Unmarshal(data, &wrapped); err != nil {
   		return nil, fmt.Errorf("parse plan state: %w", err)
   	}

   	// Detect legacy flat format: if "plans" key is absent, the top-level
   	// object IS the plan map. We detect this by checking if wrapped.Plans
   	// is nil/empty AND the raw JSON has keys ending in ".md".
   	if len(wrapped.Plans) == 0 {
   		var flat map[string]PlanEntry
   		if err := json.Unmarshal(data, &flat); err == nil && len(flat) > 0 {
   			// Check if any key looks like a plan filename
   			isLegacy := false
   			for k := range flat {
   				if strings.HasSuffix(k, ".md") {
   					isLegacy = true
   					break
   				}
   			}
   			if isLegacy {
   				wrapped.Plans = flat
   				wrapped.Topics = make(map[string]TopicEntry)
   			}
   		}
   	}

   	if wrapped.Plans == nil {
   		wrapped.Plans = make(map[string]PlanEntry)
   	}
   	if wrapped.Topics == nil {
   		wrapped.Topics = make(map[string]TopicEntry)
   	}

   	return &PlanState{Dir: dir, Plans: wrapped.Plans, TopicEntries: wrapped.Topics}, nil
   }

   func (ps *PlanState) Topics() []TopicInfo {
   	result := make([]TopicInfo, 0, len(ps.TopicEntries))
   	for name, entry := range ps.TopicEntries {
   		result = append(result, TopicInfo{Name: name, CreatedAt: entry.CreatedAt})
   	}
   	sort.Slice(result, func(i, j int) bool {
   		return result[i].Name < result[j].Name
   	})
   	return result
   }

   func (ps *PlanState) PlansByTopic(topic string) []PlanInfo {
   	result := make([]PlanInfo, 0)
   	for filename, entry := range ps.Plans {
   		if entry.Topic == topic {
   			result = append(result, PlanInfo{
   				Filename: filename, Status: entry.Status,
   				Description: entry.Description, Branch: entry.Branch,
   				Topic: entry.Topic, CreatedAt: entry.CreatedAt,
   			})
   		}
   	}
   	sort.Slice(result, func(i, j int) bool {
   		return result[i].Filename < result[j].Filename
   	})
   	return result
   }

   func (ps *PlanState) UngroupedPlans() []PlanInfo {
   	result := make([]PlanInfo, 0)
   	for filename, entry := range ps.Plans {
   		if entry.Topic == "" && entry.Status != StatusDone && entry.Status != StatusCompleted {
   			result = append(result, PlanInfo{
   				Filename: filename, Status: entry.Status,
   				Description: entry.Description, Branch: entry.Branch,
   				Topic: "", CreatedAt: entry.CreatedAt,
   			})
   		}
   	}
   	sort.Slice(result, func(i, j int) bool {
   		return result[i].Filename < result[j].Filename
   	})
   	return result
   }

   // HasRunningCoderInTopic checks if any plan in the given topic (other than
   // excludePlan) has status StatusInProgress. Returns the conflicting plan filename.
   func (ps *PlanState) HasRunningCoderInTopic(topic, excludePlan string) (bool, string) {
   	if topic == "" {
   		return false, ""
   	}
   	for filename, entry := range ps.Plans {
   		if filename == excludePlan {
   			continue
   		}
   		if entry.Topic == topic && entry.Status == StatusInProgress {
   			return true, filename
   		}
   	}
   	return false, ""
   }

   func (ps *PlanState) Unfinished() []PlanInfo {
   	result := make([]PlanInfo, 0, len(ps.Plans))
   	for filename, entry := range ps.Plans {
   		if entry.Status == StatusDone || entry.Status == StatusCompleted {
   			continue
   		}
   		result = append(result, PlanInfo{
   			Filename: filename, Status: entry.Status,
   			Description: entry.Description, Branch: entry.Branch,
   			Topic: entry.Topic, CreatedAt: entry.CreatedAt,
   		})
   	}
   	sort.Slice(result, func(i, j int) bool {
   		return result[i].Filename < result[j].Filename
   	})
   	return result
   }

   func (ps *PlanState) Finished() []PlanInfo {
   	result := make([]PlanInfo, 0)
   	for filename, entry := range ps.Plans {
   		if entry.Status != StatusDone && entry.Status != StatusCompleted {
   			continue
   		}
   		result = append(result, PlanInfo{
   			Filename: filename, Status: entry.Status,
   			Description: entry.Description, Branch: entry.Branch,
   			Topic: entry.Topic, CreatedAt: entry.CreatedAt,
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
   	return entry.Status == StatusDone
   }

   func (ps *PlanState) SetStatus(filename string, status Status) error {
   	if ps.Plans == nil {
   		ps.Plans = make(map[string]PlanEntry)
   	}
   	entry := ps.Plans[filename]
   	entry.Status = status
   	ps.Plans[filename] = entry
   	return ps.save()
   }

   func (ps *PlanState) Create(filename, description, branch, topic string, createdAt time.Time) error {
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
   		Topic:       topic,
   		CreatedAt:   createdAt.UTC(),
   	}
   	// Auto-create topic entry if it doesn't exist
   	if topic != "" {
   		if ps.TopicEntries == nil {
   			ps.TopicEntries = make(map[string]TopicEntry)
   		}
   		if _, exists := ps.TopicEntries[topic]; !exists {
   			ps.TopicEntries[topic] = TopicEntry{CreatedAt: createdAt.UTC()}
   		}
   	}
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
   	wrapped := wrappedFormat{
   		Topics: ps.TopicEntries,
   		Plans:  ps.Plans,
   	}
   	data, err := json.MarshalIndent(wrapped, "", "  ")
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

4. **Run all plan-state tests**

   Run:

   ```bash
   go test ./config/planstate/... -v
   ```

   Expected: PASS.

5. **Commit**

   ```bash
   git add config/planstate/planstate.go config/planstate/planstate_test.go
   git commit -m "feat(planstate): add topic support and wrapped JSON format"
   ```

---

### Task 2: Remove Old Topic System

**Files:**
- Modify: `config/state.go`
- Modify: `session/storage.go`
- Delete: `session/topic.go`
- Delete: `session/topic_storage.go`
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Test: `app/app_test.go`

1. **Remove topic persistence from config state**

   In `config/state.go`:
   - Remove `TopicStorage` interface entirely
   - Remove `TopicStorage` from `StateManager` interface (keep `InstanceStorage` and `AppState`)
   - Remove `TopicsData json.RawMessage` field from `State` struct
   - Remove `TopicsData` initialization from `DefaultState()`
   - Remove `SaveTopics()` and `GetTopics()` methods

2. **Remove topic serialization from session storage and delete topic files**

   In `session/storage.go`:
   - Remove `SaveTopics()` and `LoadTopics()` methods
   - Remove any imports that become unused

   Delete files:
   - `session/topic.go`
   - `session/topic_storage.go`

3. **Remove topic fields/states from app layer**

   In `app/app.go`:
   - Remove `stateNewTopic`, `stateNewTopicConfirm`, `stateRenameTopic` state constants
   - Remove `topics []*session.Topic`, `allTopics []*session.Topic`, `pendingTopicName string` fields from `home` struct
   - Remove topic loading block from `newHome()` (lines 251-264: `LoadTopics`, migration, `filterTopicsByRepo`)
   - Remove `saveAllTopics()` call from `handleQuit()`
   - In `View()`: remove `stateNewTopicConfirm` from the confirm overlay case, remove `stateRenameTopic` overlay case
   - **KEEP** `sidebarHidden`, `termWidth`, `termHeight`, `KeyToggleSidebar` handling

   In `app/app_input.go`:
   - Remove `stateNewTopic` handler block (lines 644-672)
   - Remove `stateNewTopicConfirm` handler block (lines 675-710)
   - Remove `stateRenameTopic` handler block (lines 509-544)
   - Remove `stateMoveTo` handler block (lines 713-738) — move-to-topic no longer applies
   - Remove `stateNewTopic` and `stateNewTopicConfirm` from the `handleMenuHighlighting` state skip list
   - Remove `stateMoveTo` and `stateRenameTopic` from the `handleMenuHighlighting` state skip list
   - Remove `keys.KeyNewTopic` case (line 1166-1170)
   - Remove `keys.KeyMoveTo` case (lines 1171-1186)
   - Remove `keys.KeyKillAllInTopic` case (lines 1187-1205)
   - In `keys.KeyNew`, `keys.KeyPrompt`, `keys.KeyNewSkipPermissions` handlers: remove `topicName` variable and `TopicName: topicName` from `InstanceOptions`
   - **KEEP** `keys.KeyFocusSidebar`, `keys.KeyToggleSidebar`, `keys.KeyLeft` handlers intact

   In `app/app_state.go`:
   - Remove `filterTopicsByRepo()` method
   - Remove `saveAllTopics()` method
   - Remove `getMovableTopicNames()` method
   - Simplify `updateSidebarItems()` to remove topic-based counting (temporary — will be replaced in Task 4)
   - Simplify `rebuildInstanceList()` to remove `filterTopicsByRepo` call
   - Simplify `filterInstancesByTopic()` to remove topic ID filtering (temporary)
   - Simplify `filterBySearch()` to remove `inst.TopicName` references

   In `app/app_actions.go`:
   - Remove `kill_all_in_topic`, `delete_topic_and_instances`, `delete_topic`, `rename_topic`, `push_topic` action cases
   - Remove `move_instance` action case
   - Remove any now-unused imports

4. **Verify existing tests still pass**

   Run:

   ```bash
   go test ./config/... ./session/... ./app/... -v
   ```

   Expected: PASS. The `TestSidebarToggle` tests must still pass.

5. **Verify the full build compiles**

   Run:

   ```bash
   go build ./...
   ```

   Expected: No errors.

6. **Commit**

   ```bash
   git add config/state.go session/storage.go
   git rm session/topic.go session/topic_storage.go
   git add app/app.go app/app_input.go app/app_state.go app/app_actions.go
   git commit -m "refactor: remove old topic system (worktree-owning topics)"
   ```

---

### Task 3: Sidebar Tree with Topic Grouping

**Files:**
- Modify: `ui/sidebar.go`
- Modify: `ui/sidebar_test.go` (create if missing)
- Modify: `app/app_state.go`
- Test: `ui/sidebar_test.go`

1. **Write failing sidebar tree tests**

   Create or replace `ui/sidebar_test.go`:

   ```go
   package ui

   import (
   	"testing"

   	"github.com/stretchr/testify/assert"
   	"github.com/stretchr/testify/require"
   )

   func TestSidebarTopicTree(t *testing.T) {
   	s := NewSidebar()

   	s.SetTopicsAndPlans(
   		[]TopicDisplay{
   			{Name: "ui-refactor", Plans: []PlanDisplay{
   				{Filename: "sidebar.md", Status: "in_progress"},
   				{Filename: "menu.md", Status: "ready"},
   			}},
   		},
   		[]PlanDisplay{{Filename: "bugfix.md", Status: "ready"}}, // ungrouped
   		nil, // history
   	)

   	// Topic header should exist
   	require.True(t, s.HasRowID(SidebarTopicPrefix+"ui-refactor"))
   	// Ungrouped plan should exist at top level
   	require.True(t, s.HasRowID(SidebarPlanPrefix+"bugfix.md"))
   }

   func TestSidebarExpandTopic(t *testing.T) {
   	s := NewSidebar()
   	s.SetTopicsAndPlans(
   		[]TopicDisplay{
   			{Name: "ui", Plans: []PlanDisplay{
   				{Filename: "a.md", Status: "ready"},
   			}},
   		},
   		nil, nil,
   	)

   	// Topic starts collapsed — plan should not be visible
   	assert.False(t, s.HasRowID(SidebarPlanPrefix+"a.md"))

   	// Expand topic
   	s.SelectByID(SidebarTopicPrefix + "ui")
   	s.ToggleSelectedExpand()

   	// Now plan should be visible
   	assert.True(t, s.HasRowID(SidebarPlanPrefix+"a.md"))
   }

   func TestSidebarExpandPlanStages(t *testing.T) {
   	s := NewSidebar()
   	s.SetTopicsAndPlans(
   		nil,
   		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
   		nil,
   	)

   	// Expand ungrouped plan
   	s.SelectByID(SidebarPlanPrefix + "fix.md")
   	s.ToggleSelectedExpand()

   	// Stage rows should appear
   	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))
   	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::implement"))
   	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::review"))
   	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::finished"))
   }

   func TestSidebarGetSelectedPlanStage(t *testing.T) {
   	s := NewSidebar()
   	s.SetTopicsAndPlans(
   		nil,
   		[]PlanDisplay{{Filename: "fix.md", Status: "reviewing"}},
   		nil,
   	)

   	s.SelectByID(SidebarPlanPrefix + "fix.md")
   	s.ToggleSelectedExpand()
   	s.SelectByID(SidebarPlanStagePrefix + "fix.md::review")

   	planFile, stage, ok := s.GetSelectedPlanStage()
   	require.True(t, ok)
   	assert.Equal(t, "fix.md", planFile)
   	assert.Equal(t, "review", stage)
   }

   func TestSidebarGetSelectedTopicName(t *testing.T) {
   	s := NewSidebar()
   	s.SetTopicsAndPlans(
   		[]TopicDisplay{{Name: "auth", Plans: nil}},
   		nil, nil,
   	)

   	s.SelectByID(SidebarTopicPrefix + "auth")
   	name := s.GetSelectedTopicName()
   	assert.Equal(t, "auth", name)
   }

   func TestSidebarPlanHistory(t *testing.T) {
   	s := NewSidebar()
   	s.SetTopicsAndPlans(
   		nil, nil,
   		[]PlanDisplay{{Filename: "old.md", Status: "completed"}},
   	)

   	assert.True(t, s.HasRowID(SidebarPlanHistoryToggle))
   }
   ```

2. **Run tests to confirm failure**

   Run:

   ```bash
   go test ./ui/... -run 'TestSidebarTopicTree|TestSidebarExpandTopic|TestSidebarExpandPlanStages|TestSidebarGetSelectedPlanStage|TestSidebarGetSelectedTopicName|TestSidebarPlanHistory' -v
   ```

   Expected: FAIL (missing new types and methods).

3. **Implement the three-level sidebar tree**

   Refactor `ui/sidebar.go`. Key changes:

   - Add constants: `SidebarTopicPrefix = "__topic__"`, `SidebarPlanHistoryToggle = "__plan_history_toggle__"`, `SidebarPlanStagePrefix = "__plan_stage__"`
   - Add `TopicDisplay` struct: `Name string`, `Plans []PlanDisplay`
   - Extend `PlanDisplay` with `Description`, `Branch`, `Topic`, `CreatedAt` fields
   - Add `sidebarRow` struct with `Kind` (topic/plan/stage/section/historyToggle), `ID`, `Label`, `PlanFile`, `Stage`, `Locked`, `Done`, `Active`, `Count`, `Collapsed` fields
   - Replace `items []SidebarItem` with `rows []sidebarRow` on `Sidebar`
   - Add `expandedTopics map[string]bool` and `expandedPlans map[string]bool` to `Sidebar`
   - New `SetTopicsAndPlans(topics []TopicDisplay, ungrouped []PlanDisplay, history []PlanDisplay)` method that stores data and calls `rebuildRows()`
   - `rebuildRows()` builds the flat row list from the tree structure, respecting expand/collapse state
   - `ToggleSelectedExpand() bool` — toggles topic or plan expansion
   - `GetSelectedPlanStage() (planFile, stage string, ok bool)` — returns stage info if a stage row is selected
   - `GetSelectedTopicName() string` — returns topic name if a topic row is selected
   - `IsSelectedPlanHeader() bool` — returns true if a plan header row is selected
   - `IsSelectedTopicHeader() bool` — returns true if a topic header row is selected
   - `HasRowID(id string) bool` and `SelectByID(id string) bool` — test helpers
   - Stage glyph rendering: `✓` done (ColorMuted), `●` active (ColorFoam), `○` available (ColorText), `○` locked (ColorOverlay)
   - Topic header glyph: `◆` prefix, `ColorSubtle` foreground
   - Keep `SetRepoName`, `SetRepoHovered`, search functionality, `Up()`, `Down()`, `ClickItem()` working
   - All new styles include `.Background(ColorBase)` to match existing theme

4. **Wire app state to new sidebar API**

   Update `app/app_state.go`:

   - Replace `updateSidebarItems()` to build `TopicDisplay` and `PlanDisplay` slices from `planState`
   - Replace `updateSidebarPlans()` to call `SetTopicsAndPlans()`
   - Update `filterInstancesByTopic()` to handle topic and plan filtering

   ```go
   func (m *home) updateSidebarPlans() {
   	if m.planState == nil {
   		m.sidebar.SetTopicsAndPlans(nil, nil, nil)
   		return
   	}

   	// Build topic displays
   	topicInfos := m.planState.Topics()
   	topics := make([]ui.TopicDisplay, 0, len(topicInfos))
   	for _, t := range topicInfos {
   		plans := m.planState.PlansByTopic(t.Name)
   		planDisplays := make([]ui.PlanDisplay, 0, len(plans))
   		for _, p := range plans {
   			if p.Status == planstate.StatusDone || p.Status == planstate.StatusCompleted {
   				continue // finished plans go to history
   			}
   			planDisplays = append(planDisplays, ui.PlanDisplay{
   				Filename: p.Filename, Status: string(p.Status),
   				Description: p.Description, Branch: p.Branch,
   				Topic: p.Topic,
   			})
   		}
   		if len(planDisplays) > 0 {
   			topics = append(topics, ui.TopicDisplay{Name: t.Name, Plans: planDisplays})
   		}
   	}

   	// Build ungrouped plans
   	ungroupedInfos := m.planState.UngroupedPlans()
   	ungrouped := make([]ui.PlanDisplay, 0, len(ungroupedInfos))
   	for _, p := range ungroupedInfos {
   		ungrouped = append(ungrouped, ui.PlanDisplay{
   			Filename: p.Filename, Status: string(p.Status),
   			Description: p.Description, Branch: p.Branch,
   		})
   	}

   	// Build history
   	finishedInfos := m.planState.Finished()
   	history := make([]ui.PlanDisplay, 0, len(finishedInfos))
   	for _, p := range finishedInfos {
   		history = append(history, ui.PlanDisplay{
   			Filename: p.Filename, Status: string(p.Status),
   			Description: p.Description, Branch: p.Branch,
   			Topic: p.Topic,
   		})
   	}

   	m.sidebar.SetTopicsAndPlans(topics, ungrouped, history)
   }
   ```

5. **Run sidebar tests**

   Run:

   ```bash
   go test ./ui/... -v
   ```

   Expected: PASS.

6. **Run full test suite**

   Run:

   ```bash
   go test ./... 2>&1 | tail -30
   ```

   Expected: PASS (including `TestSidebarToggle`).

7. **Commit**

   ```bash
   git add ui/sidebar.go ui/sidebar_test.go app/app_state.go
   git commit -m "feat(sidebar): three-level tree with topic grouping"
   ```

---

### Task 4: Plan Creation Flow with Topic Picker

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Modify: `keys/keys.go`
- Modify: `ui/menu.go`
- Modify: `app/help.go`
- Test: `app/app_test.go`

1. **Add plan creation states and keybind**

   In `app/app.go`, add new state constants:

   ```go
   const (
   	// ... existing states ...
   	stateNewPlanName
   	stateNewPlanDescription
   	stateNewPlanTopic
   )
   ```

   Add field to `home`:

   ```go
   pendingPlanName string
   pendingPlanDesc string
   ```

2. **Repurpose `p` key to new plan**

   In `keys/keys.go`:
   - Add `KeyNewPlan` constant
   - Change `"p"` mapping from `KeySubmit` to `KeyNewPlan`
   - Remove `KeyNewTopic`, `KeyMoveTo`, `KeyKillAllInTopic` constants and their mappings (`"T"`, `"m"`, `"X"`)
   - Add binding for `KeyNewPlan`: `key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "new plan"))`
   - Remove bindings for `KeyNewTopic`, `KeyMoveTo`, `KeyKillAllInTopic`
   - **KEEP** `KeyToggleSidebar` and its `"ctrl+s"` mapping

3. **Update menu bar**

   In `ui/menu.go`:
   - Add `keys.KeyNewPlan` to `defaultMenuOptions`
   - Remove `keys.KeyKillAllInTopic` from `addInstanceOptions()` systemGroup
   - **KEEP** `keys.KeyToggleSidebar` handling

4. **Implement three-step plan creation flow**

   In `app/app_input.go`:

   - Add `keys.KeyNewPlan` case that enters `stateNewPlanName` with a text input overlay
   - Add `stateNewPlanName` handler: on submit, store name, transition to `stateNewPlanDescription`
   - Add `stateNewPlanDescription` handler: on submit, store description, transition to `stateNewPlanTopic`
   - Add `stateNewPlanTopic` handler: show picker overlay with existing topic names + free-text. On submit, call `createPlanEntry()` with all three values
   - Add all three states to the `handleMenuHighlighting` skip list

   In `app/app_state.go`, add:

   ```go
   func (m *home) createPlanEntry(name, description, topic string) error {
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
   	return m.planState.Create(filename, description, branch, topic, time.Now().UTC())
   }

   func slugifyPlanName(name string) string {
   	name = strings.ToLower(strings.TrimSpace(name))
   	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
   	return strings.Trim(name, "-")
   }

   // getTopicNames returns existing topic names for the picker.
   func (m *home) getTopicNames() []string {
   	if m.planState == nil {
   		return nil
   	}
   	topics := m.planState.Topics()
   	names := make([]string, len(topics))
   	for i, t := range topics {
   		names[i] = t.Name
   	}
   	return names
   }
   ```

5. **Update help text**

   In `app/help.go`, replace the "Topics" section:

   ```go
   headerStyle.Render("\uf03a Plans:"),
   keyStyle.Render("p")+descStyle.Render("         - Create a new plan"),
   keyStyle.Render("space")+descStyle.Render("     - Expand/collapse plan or topic"),
   keyStyle.Render("↵/o")+descStyle.Render("       - Plan/topic menu or run stage"),
   keyStyle.Render("v")+descStyle.Render("         - View selected plan"),
   keyStyle.Render("/")+descStyle.Render("         - Search plans and instances"),
   keyStyle.Render("←/h, →/l")+descStyle.Render("  - Switch sidebar and instance list"),
   ```

6. **Run tests**

   Run:

   ```bash
   go test ./keys/... ./ui/... ./app/... -v
   ```

   Expected: PASS (including `TestSidebarToggle`).

7. **Commit**

   ```bash
   git add keys/keys.go ui/menu.go app/app.go app/app_input.go app/app_state.go app/help.go
   git commit -m "feat(app): three-step plan creation with topic picker"
   ```

---

### Task 5: Space/Enter Interactions and Concurrency Gate

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_actions.go`
- Modify: `ui/sidebar.go`
- Test: `app/app_test.go`

1. **Wire Space key to expand/collapse**

   In `app/app_input.go`, update `keys.KeySpace` case:

   ```go
   case keys.KeySpace:
   	if m.focusedPanel == 0 && m.sidebar.ToggleSelectedExpand() {
   		return m, nil
   	}
   	return m.openContextMenu()
   ```

2. **Wire Enter key to stage actions and context menus**

   In `app/app_input.go`, update `keys.KeyEnter` case:

   ```go
   case keys.KeyEnter:
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
   		// Plan file selected (legacy path): spawn coder session
   		if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
   			return m.spawnPlanSession(planFile)
   		}
   	}
   	// ... existing instance attach flow ...
   ```

3. **Implement concurrency gate in triggerPlanStage**

   In `app/app_input.go` or `app/app_actions.go`:

   ```go
   func (m *home) triggerPlanStage(planFile, stage string) (tea.Model, tea.Cmd) {
   	entry, ok := m.planState.Plans[planFile]
   	if !ok {
   		return m, m.handleError(fmt.Errorf("missing plan state for %s", planFile))
   	}

   	// Check if stage is locked
   	if isLocked(entry.Status, stage) {
   		prev := map[string]string{"implement": "plan", "review": "implement", "finished": "review"}[stage]
   		m.toastManager.Error(fmt.Sprintf("complete %s first", prev))
   		return m, m.toastTickCmd()
   	}

   	// Concurrency gate for implement stage
   	if stage == "implement" && entry.Topic != "" {
   		if hasConflict, conflictPlan := m.planState.HasRunningCoderInTopic(entry.Topic, planFile); hasConflict {
   			conflictName := planstate.DisplayName(conflictPlan)
   			message := fmt.Sprintf("⚠ %s is already running in topic \"%s\"\n\nRunning both plans may cause issues.\nContinue anyway?", conflictName, entry.Topic)
   			proceedAction := func() tea.Msg {
   				m.executePlanStage(planFile, stage)
   				return instanceChangedMsg{}
   			}
   			return m, m.confirmAction(message, proceedAction)
   		}
   	}

   	m.executePlanStage(planFile, stage)
   	return m, nil
   }

   func (m *home) executePlanStage(planFile, stage string) {
   	switch stage {
   	case "plan":
   		_ = m.planState.SetStatus(planFile, planstate.StatusInProgress)
   	case "implement":
   		_ = m.planState.SetStatus(planFile, planstate.StatusInProgress)
   	case "review":
   		_ = m.planState.SetStatus(planFile, planstate.StatusReviewing)
   	case "finished":
   		_ = m.planState.SetStatus(planFile, planstate.StatusCompleted)
   	}
   	m.loadPlanState()
   	m.updateSidebarPlans()
   	m.updateSidebarItems()
   }

   func isLocked(status planstate.Status, stage string) bool {
   	switch stage {
   	case "plan":
   		return false
   	case "implement":
   		return status == planstate.StatusReady
   	case "review":
   		return status == planstate.StatusReady || status == planstate.StatusInProgress
   	case "finished":
   		return status != planstate.StatusReviewing && status != planstate.StatusDone && status != planstate.StatusCompleted
   	default:
   		return true
   	}
   }
   ```

4. **Implement plan and topic context menus**

   In `app/app_actions.go`:

   ```go
   func (m *home) openPlanContextMenu() (tea.Model, tea.Cmd) {
   	planFile := m.sidebar.GetSelectedPlanFile()
   	if planFile == "" {
   		return m, nil
   	}
   	items := []overlay.ContextMenuItem{
   		{Label: "View plan", Action: "view_plan"},
   		{Label: "View design doc", Action: "view_design_doc"},
   		{Label: "Push branch", Action: "push_plan_branch"},
   		{Label: "Create PR", Action: "create_plan_pr"},
   		{Label: "Rename plan", Action: "rename_plan"},
   		{Label: "Delete plan", Action: "delete_plan"},
   	}
   	// Position near the sidebar
   	m.contextMenu = overlay.NewContextMenu(m.sidebarWidth/2, m.contentHeight/2, items)
   	m.state = stateContextMenu
   	return m, nil
   }

   func (m *home) openTopicContextMenu() (tea.Model, tea.Cmd) {
   	topicName := m.sidebar.GetSelectedTopicName()
   	if topicName == "" {
   		return m, nil
   	}
   	items := []overlay.ContextMenuItem{
   		{Label: "Rename topic", Action: "rename_topic_new"},
   		{Label: "Delete topic (ungroup plans)", Action: "delete_topic_new"},
   	}
   	m.contextMenu = overlay.NewContextMenu(m.sidebarWidth/2, m.contentHeight/2, items)
   	m.state = stateContextMenu
   	return m, nil
   }
   ```

   Add action handlers in `executeContextAction()`:

   ```go
   case "rename_topic_new":
   	topicName := m.sidebar.GetSelectedTopicName()
   	if topicName == "" {
   		return m, nil
   	}
   	m.state = stateRenameInstance // reuse rename overlay state
   	m.textInputOverlay = overlay.NewTextInputOverlay("Rename topic", topicName)
   	m.textInputOverlay.SetSize(50, 3)
   	// Store topic name for the rename handler
   	// (will need a pendingTopicRename field or similar)
   	return m, nil

   case "delete_topic_new":
   	topicName := m.sidebar.GetSelectedTopicName()
   	if topicName == "" {
   		return m, nil
   	}
   	// Ungroup all plans in this topic
   	for filename, entry := range m.planState.Plans {
   		if entry.Topic == topicName {
   			entry.Topic = ""
   			m.planState.Plans[filename] = entry
   		}
   	}
   	delete(m.planState.TopicEntries, topicName)
   	m.planState.SetStatus("", "") // trigger save (use a dedicated save method)
   	m.updateSidebarPlans()
   	m.updateSidebarItems()
   	return m, tea.WindowSize()

   case "view_plan":
   	return m.viewSelectedPlan()

   case "view_design_doc":
   	// Open companion *-design.md if it exists
   	planFile := m.sidebar.GetSelectedPlanFile()
   	if planFile == "" {
   		return m, nil
   	}
   	designFile := strings.TrimSuffix(planFile, ".md") + "-design.md"
   	m.cachedPlanFile = "" // force re-render
   	// Temporarily swap to render the design doc
   	origPlanFile := planFile
   	_ = origPlanFile
   	// Use the same viewSelectedPlan mechanism but with the design file
   	// (This is a stub — full implementation in Plan 2)
   	m.toastManager.Info("Design doc viewer coming soon")
   	return m, m.toastTickCmd()
   ```

5. **Run full test suite**

   Run:

   ```bash
   go test ./... 2>&1 | tail -30
   ```

   Expected: PASS.

6. **Commit**

   ```bash
   git add app/app_input.go app/app_actions.go ui/sidebar.go app/app_test.go
   git commit -m "feat(sidebar): space/enter interactions and concurrency gate"
   ```

---

### Task 6: Final Cleanup and Verification

**Files:**
- Modify: `app/app_input.go` (cleanup)
- Modify: `session/storage.go` (cleanup)
- Modify: `session/instance.go` (cleanup `TopicName` references)
- Test: all

1. **Remove `TopicName` from Instance**

   In `session/instance.go`:
   - Remove `TopicName string` field from `Instance` struct
   - Remove `sharedWorktree bool` field

   In `session/storage.go`:
   - Remove `TopicName` from `InstanceData` struct

   In `app/app_input.go`:
   - Remove any remaining `instance.TopicName` or `TopicName:` references in instance creation

2. **Update right-click context menu for sidebar**

   In `app/app_input.go` `handleRightClick()`:
   - Replace the topic-based sidebar right-click with plan/topic-aware right-click
   - Use `IsSelectedPlanHeader()` and `IsSelectedTopicHeader()` to determine which context menu to show

3. **Run full verification**

   Run:

   ```bash
   go build ./...
   go test ./...
   ```

   Expected: Build succeeds, all tests pass.

4. **Commit**

   ```bash
   git add session/instance.go session/storage.go app/app_input.go
   git commit -m "refactor: remove TopicName from instances, clean up old topic references"
   ```

---

### Notes for the Executor

- **Preserve sidebar toggle**: `sidebarHidden`, `KeyToggleSidebar`, `KeyFocusSidebar` two-step reveal, and `KeyLeft` sidebar-reveal logic are unrelated to topics. Do NOT touch them.
- **Preserve global background fill**: All new styles must include `.Background(ColorBase)`.
- **Legacy format migration**: The existing `plan-state.json` is a flat map. `Load()` must detect and handle this format.
- **Task ordering matters**: Task 1 (schema) must complete before Task 3 (sidebar) because the sidebar needs `Topics()`, `PlansByTopic()`, etc. Task 2 (remove old topics) should happen before Task 3 to avoid merge conflicts with the old `SetItems()` API.
- **Stub behavior**: Some actions (view design doc, full agent spawning) are stubs in this plan. They'll be implemented in the Plan 2 (lifecycle sessions) update.
- **Instance `TopicName` removal**: This is done last (Task 6) because earlier tasks may still reference it during the transition. The `TopicName` field on `InstanceData` in storage.go should use `omitempty` so old serialized data with topic names still deserializes without error — the field just gets ignored.
