# Rename Plan Entity to Task

**Goal:** Rename the "plan" lifecycle entity to "task" throughout the codebase to eliminate the terminology collision between "plan" (the lifecycle entity) and "planning" (the FSM status). Instances belong to tasks which optionally belong to topics — "a task awaiting planning" is clearer than "a plan awaiting planning."

**Architecture:** Four-phase mechanical rename: (1) rename Go package directories and update import paths, (2) rename exported types and struct fields, (3) rename CLI commands, user-facing strings, and internal variables, (4) update HTTP API paths, SQL schema, and JSON serialization with backwards compatibility. Each phase uses `sd`/`comby` for batch replacements. The `docs/plans/` directory is left as-is since a pending task will move plan content to the database.

**Tech Stack:** Go 1.24, cobra CLI, SQLite, HTTP API, `sd` for batch string replacement, `mv` for file/directory renames

**Size:** Large (estimated ~5 hours, 7 tasks, 3 waves)

---

## Wave 1: Rename Package Directories and Update Import Paths

### Task 1: Rename config/planstore to config/taskstore

**Files:**
- Create: `config/taskstore/` (entire directory — copy from `config/planstore/`)
- Remove: `config/planstore/`
- Modify: all files importing `config/planstore` (18 files)

**Step 1: write the failing test**

No new tests — this is a package rename. Existing tests validate correctness.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/planstore/... -count=1
```

expected: PASS (baseline)

**Step 3: rename the package and update imports**

```bash
# 1. Copy directory
cp -r config/planstore config/taskstore

# 2. Rename package declaration in all files
sd 'package planstore' 'package taskstore' config/taskstore/*.go

# 3. Update import paths across entire codebase
sd '"github.com/kastheco/kasmos/config/planstore"' '"github.com/kastheco/kasmos/config/taskstore"' $(fd -e go)

# 4. Update import aliases (some files use named imports)
sd 'planstore "github.com/kastheco/kasmos/config/taskstore"' 'taskstore "github.com/kastheco/kasmos/config/taskstore"' $(fd -e go)

# 5. Update all qualified references: planstore.X → taskstore.X
sd 'planstore\.' 'taskstore.' $(fd -e go)

# 6. Rename PlanEntry → TaskEntry in the taskstore package
sd 'PlanEntry' 'TaskEntry' config/taskstore/*.go

# 7. Update PlanEntry references across the codebase (qualified)
sd 'taskstore\.TaskEntry' 'taskstore.TaskEntry' $(fd -e go)
# (this is a no-op but ensures consistency — the real rename is the sd above)

# 8. Rename internal functions: scanPlanEntry → scanTaskEntry, scanPlanEntries → scanTaskEntries
sd 'scanPlanEntry' 'scanTaskEntry' config/taskstore/sqlite.go
sd 'scanPlanEntries' 'scanTaskEntries' config/taskstore/sqlite.go

# 9. Rename jsonPlanEntry → jsonTaskEntry, jsonPlanState → jsonTaskState in migrate.go
sd 'jsonPlanEntry' 'jsonTaskEntry' config/taskstore/migrate.go
sd 'jsonPlanState' 'jsonTaskState' config/taskstore/migrate.go

# 10. Update factory.go function names and comments
sd 'planStoreURL' 'storeURL' config/taskstore/factory.go
sd 'planstore\.db' 'kasmos.db' config/taskstore/factory.go
sd 'NewStoreFromConfig' 'NewStoreFromConfig' config/taskstore/factory.go  # name stays generic

# 11. Update http.go URL builder method names
sd 'func (s \*HTTPStore) planURL' 'func (s *HTTPStore) taskURL' config/taskstore/http.go
sd 'func (s \*HTTPStore) planItemURL' 'func (s *HTTPStore) taskItemURL' config/taskstore/http.go
sd 'func (s \*HTTPStore) planContentURL' 'func (s *HTTPStore) taskContentURL' config/taskstore/http.go
sd 's\.planURL' 's.taskURL' config/taskstore/http.go
sd 's\.planItemURL' 's.taskItemURL' config/taskstore/http.go
sd 's\.planContentURL' 's.taskContentURL' config/taskstore/http.go

# 12. Remove old directory
rm -rf config/planstore
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./config/taskstore/... -count=1
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename config/planstore package to config/taskstore"
```

### Task 2: Rename config/planstate to config/taskstate

**Files:**
- Create: `config/taskstate/` (copy from `config/planstate/`)
- Remove: `config/planstate/`
- Modify: all files importing `config/planstate` (22 files)

**Step 1: write the failing test**

No new tests — package rename only.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/planstate/... -count=1
```

expected: PASS (baseline)

**Step 3: rename the package and update imports**

```bash
# 1. Copy directory
cp -r config/planstate config/taskstate

# 2. Rename package declaration
sd 'package planstate' 'package taskstate' config/taskstate/*.go

# 3. Update import paths across codebase
sd '"github.com/kastheco/kasmos/config/planstate"' '"github.com/kastheco/kasmos/config/taskstate"' $(fd -e go)

# 4. Update all qualified references: planstate.X → taskstate.X
sd 'planstate\.' 'taskstate.' $(fd -e go)

# 5. Rename types: PlanState → TaskState, PlanEntry → TaskEntry, PlanInfo → TaskInfo
sd 'PlanState' 'TaskState' config/taskstate/*.go
sd 'PlanEntry' 'TaskEntry' config/taskstate/*.go
sd 'PlanInfo' 'TaskInfo' config/taskstate/*.go

# 6. Update these type references across the codebase
sd 'taskstate\.PlanState' 'taskstate.TaskState' $(fd -e go)
sd 'taskstate\.PlanEntry' 'taskstate.TaskEntry' $(fd -e go)
sd 'taskstate\.PlanInfo' 'taskstate.TaskInfo' $(fd -e go)
sd '\*PlanState' '*TaskState' config/taskstate/*.go
sd 'PlanInfo' 'TaskInfo' $(fd -e go -p 'app/|cmd/|ui/')

# 7. Rename internal method: toPlanstoreEntry → toTaskstoreEntry
sd 'toPlanstoreEntry' 'toTaskstoreEntry' config/taskstate/planstate.go

# 8. Rename methods that reference "Plan" in their name
sd 'PlansByTopic' 'TasksByTopic' config/taskstate/planstate.go
sd 'UngroupedPlans' 'UngroupedTasks' config/taskstate/planstate.go
sd 'HasRunningCoderInTopic' 'HasRunningCoderInTopic' config/taskstate/planstate.go  # name is fine as-is

# 9. Update references to renamed methods across codebase
sd '\.PlansByTopic' '.TasksByTopic' $(fd -e go)
sd '\.UngroupedPlans' '.UngroupedTasks' $(fd -e go)

# 10. Rename the source file
mv config/taskstate/planstate.go config/taskstate/taskstate.go
mv config/taskstate/planstate_test.go config/taskstate/taskstate_test.go

# 11. Remove old directory
rm -rf config/planstate
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./config/taskstate/... -count=1
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename config/planstate package to config/taskstate"
```

### Task 3: Rename config/planfsm to config/taskfsm

**Files:**
- Create: `config/taskfsm/` (copy from `config/planfsm/`)
- Remove: `config/planfsm/`
- Modify: all files importing `config/planfsm` (11 files)

**Step 1: write the failing test**

No new tests — package rename only.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/planfsm/... -count=1
```

expected: PASS (baseline)

**Step 3: rename the package and update imports**

```bash
# 1. Copy directory
cp -r config/planfsm config/taskfsm

# 2. Rename package declaration
sd 'package planfsm' 'package taskfsm' config/taskfsm/*.go

# 3. Update import paths across codebase
sd '"github.com/kastheco/kasmos/config/planfsm"' '"github.com/kastheco/kasmos/config/taskfsm"' $(fd -e go)

# 4. Update all qualified references: planfsm.X → taskfsm.X
sd 'planfsm\.' 'taskfsm.' $(fd -e go)

# 5. Rename PlanStateMachine → TaskStateMachine
sd 'PlanStateMachine' 'TaskStateMachine' config/taskfsm/*.go
sd 'taskfsm\.PlanStateMachine' 'taskfsm.TaskStateMachine' $(fd -e go)
sd '\*PlanStateMachine' '*TaskStateMachine' $(fd -e go)

# 6. Rename the Signal.PlanFile field → TaskFile
sd 'PlanFile string' 'TaskFile string' config/taskfsm/signals.go
sd 'sig\.PlanFile' 'sig.TaskFile' config/taskfsm/signals.go
sd 'PlanFile:' 'TaskFile:' config/taskfsm/signals.go config/taskfsm/signals_test.go

# 7. Rename WaveSignal.PlanFile → TaskFile
sd 'PlanFile string' 'TaskFile string' config/taskfsm/wave_signal.go
sd 'ws\.PlanFile' 'ws.TaskFile' config/taskfsm/wave_signal.go config/taskfsm/wave_signal_test.go
sd 'PlanFile:' 'TaskFile:' config/taskfsm/wave_signal.go config/taskfsm/wave_signal_test.go

# 8. Update references to sig.PlanFile and ws.PlanFile in app/
sd 'sig\.PlanFile' 'sig.TaskFile' $(fd -e go -p 'app/')
sd 'ws\.PlanFile' 'ws.TaskFile' $(fd -e go -p 'app/')

# 9. Remove old directory
rm -rf config/planfsm
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./config/taskfsm/... -count=1
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename config/planfsm package to config/taskfsm"
```

### Task 4: Rename config/planparser to config/taskparser

**Files:**
- Create: `config/taskparser/` (copy from `config/planparser/`)
- Remove: `config/planparser/`
- Modify: all files importing `config/planparser` (9 files)

**Step 1: write the failing test**

No new tests — package rename only.

**Step 2: run test to verify baseline passes**

```bash
go test ./config/planparser/... -count=1
```

expected: PASS (baseline)

**Step 3: rename the package and update imports**

```bash
# 1. Copy directory
cp -r config/planparser config/taskparser

# 2. Rename package declaration
sd 'package planparser' 'package taskparser' config/taskparser/*.go

# 3. Rename source files
mv config/taskparser/planparser.go config/taskparser/taskparser.go
mv config/taskparser/planparser_test.go config/taskparser/taskparser_test.go

# 4. Update import paths across codebase
sd '"github.com/kastheco/kasmos/config/planparser"' '"github.com/kastheco/kasmos/config/taskparser"' $(fd -e go)

# 5. Update all qualified references: planparser.X → taskparser.X
sd 'planparser\.' 'taskparser.' $(fd -e go)

# 6. Remove old directory
rm -rf config/planparser
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./config/taskparser/... -count=1
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename config/planparser package to config/taskparser"
```

## Wave 2: Rename Struct Fields, Instance Fields, and Internal Variables

> **depends on wave 1:** all import paths and package names must be stable before renaming the fields and variables that reference them.

### Task 5: Rename PlanFile to TaskFile, plan-prefixed variables/methods, and file renames

**Files:**
- Modify: `session/instance.go` — `PlanFile` → `TaskFile` field
- Modify: `session/storage.go` — `PlanFile` → `TaskFile` field + JSON tag + backwards-compat unmarshal
- Modify: `session/instance_lifecycle.go` — references
- Modify: `config/auditlog/event.go`, `config/auditlog/logger.go`, `config/auditlog/sqlite.go` — `PlanFile` → `TaskFile`
- Modify: `app/app.go`, `app/app_state.go`, `app/app_actions.go`, `app/app_input.go` — all `.PlanFile` refs, struct fields, methods, message types, state constants
- Modify: `app/wave_orchestrator.go` — `planFile` field → `taskFile`, `PlanFile()` → `TaskFile()`
- Modify: `app/clickup_progress.go`, `app/wave_prompt.go` — rename functions
- Modify: `cmd/plan.go` → `cmd/task.go` — rename functions
- Modify: `session/git/plan_lifecycle.go` → `session/git/task_lifecycle.go` — rename functions
- Modify: `config/config.go`, `config/toml.go` — `PlanStore` → `DatabaseURL`, TOML/JSON key `plan_store` → `database_url`
- Modify: `ui/navigation_panel.go` — references
- Rename: all test files with `plan` in name
- Modify: all `app/*_test.go`, `cmd/*_test.go` files

**Step 1: write the failing test**

No new tests — field and method rename. Existing tests validate correctness.

**Step 2: run test to verify baseline passes**

```bash
go test ./... -count=1 2>&1 | tail -10
```

expected: PASS (baseline)

**Step 3: rename fields, variables, methods, and files**

```bash
# ============================================================
# PART A: PlanFile → TaskFile struct field rename
# ============================================================

# 1. Rename the struct field in session/instance.go and session/storage.go
sd 'PlanFile string' 'TaskFile string' session/instance.go session/storage.go

# 2. Rename JSON tag
sd '"plan_file,omitempty"' '"task_file,omitempty"' session/storage.go

# 3. Rename all .PlanFile references across the codebase
sd '\.PlanFile' '.TaskFile' $(fd -e go)

# 4. Rename PlanFile: struct literal keys
sd 'PlanFile:' 'TaskFile:' $(fd -e go)

# 5. Rename in auditlog
sd 'PlanFile' 'TaskFile' config/auditlog/event.go config/auditlog/logger.go config/auditlog/sqlite.go config/auditlog/sqlite_test.go

# 6. Rename the test file
mv session/instance_planfile_test.go session/instance_taskfile_test.go

# 7. Update the wave orchestrator's planFile field → taskFile
sd 'planFile  string' 'taskFile  string' app/wave_orchestrator.go
sd 'planFile:' 'taskFile:' app/wave_orchestrator.go
sd 'o\.planFile' 'o.taskFile' app/wave_orchestrator.go
sd 'func (o \*WaveOrchestrator) PlanFile' 'func (o *WaveOrchestrator) TaskFile' app/wave_orchestrator.go
sd 'orch\.PlanFile\(\)' 'orch.TaskFile()' $(fd -e go -p 'app/')

# 8. Add backwards-compatible JSON deserialization to session/storage.go
```

For step 8, add this method to `session/storage.go` after the `InstanceData` struct:

```go
// UnmarshalJSON implements custom JSON unmarshaling to handle the rename from
// plan_file to task_file. Existing state.json files may use the old field name.
func (d *InstanceData) UnmarshalJSON(data []byte) error {
	type Alias InstanceData
	aux := &struct {
		*Alias
		PlanFile string `json:"plan_file,omitempty"`
	}{Alias: (*Alias)(d)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if d.TaskFile == "" && aux.PlanFile != "" {
		d.TaskFile = aux.PlanFile
	}
	return nil
}
```

```bash
# ============================================================
# PART B: app/ home struct fields and pending fields
# ============================================================

sd 'planState ' 'taskState ' app/app.go
sd 'planStore ' 'taskStore ' app/app.go
sd 'planStoreProject' 'taskStoreProject' $(fd -e go -p 'app/')
sd 'planStateDir' 'taskStateDir' $(fd -e go -p 'app/')
sd 'm\.planState' 'm.taskState' $(fd -e go -p 'app/')
sd 'm\.planStore' 'm.taskStore' $(fd -e go -p 'app/')
sd 'h\.planState' 'h.taskState' $(fd -e go -p 'app/')
sd 'h\.planStore' 'h.taskStore' $(fd -e go -p 'app/')
sd 'pendingPlannerPlanFile' 'pendingPlannerTaskFile' $(fd -e go -p 'app/')
sd 'pendingChatAboutPlan' 'pendingChatAboutTask' $(fd -e go -p 'app/')
sd 'pendingChangeTopicPlan' 'pendingChangeTopicTask' $(fd -e go -p 'app/')
sd 'pendingSetStatusPlan' 'pendingSetStatusTask' $(fd -e go -p 'app/')

# ============================================================
# PART C: app/ method renames
# ============================================================

sd 'loadPlanState' 'loadTaskState' $(fd -e go -p 'app/')
sd 'updateSidebarPlans' 'updateSidebarTasks' $(fd -e go -p 'app/')
sd 'spawnPlanAgent' 'spawnTaskAgent' $(fd -e go -p 'app/')
sd 'planBranch' 'taskBranch' $(fd -e go -p 'app/')
sd 'materializePlanFile' 'materializeTaskFile' $(fd -e go -p 'app/')
sd 'ingestPlanContent' 'ingestTaskContent' $(fd -e go -p 'app/')
sd 'triggerPlanStage' 'triggerTaskStage' $(fd -e go -p 'app/')
sd 'executePlanStage' 'executeTaskStage' $(fd -e go -p 'app/')
sd 'findPlanInstance' 'findTaskInstance' $(fd -e go -p 'app/')
sd 'openPlanContextMenu' 'openTaskContextMenu' $(fd -e go -p 'app/')

# ============================================================
# PART D: app/ message types and state constants
# ============================================================

sd 'planRefreshMsg' 'taskRefreshMsg' $(fd -e go -p 'app/')
sd 'planStageConfirmedMsg' 'taskStageConfirmedMsg' $(fd -e go -p 'app/')
# plannerCompleteMsg stays — refers to planner agent role, not the entity
sd 'stateRenamePlan' 'stateRenameTask' $(fd -e go -p 'app/')
sd 'stateChatAboutPlan' 'stateChatAboutTask' $(fd -e go -p 'app/')

# ============================================================
# PART E: app/ prompt builders
# ============================================================

sd 'buildPlanPrompt' 'buildPlanningPrompt' $(fd -e go -p 'app/')
sd 'buildModifyPlanPrompt' 'buildModifyTaskPrompt' $(fd -e go -p 'app/')
sd 'buildChatAboutPlanPrompt' 'buildChatAboutTaskPrompt' $(fd -e go -p 'app/')
sd 'spawnChatAboutPlan' 'spawnChatAboutTask' $(fd -e go -p 'app/')

# ============================================================
# PART F: session/git/plan_lifecycle.go renames
# ============================================================

sd 'PlanBranchFromFile' 'TaskBranchFromFile' $(fd -e go)
sd 'PlanWorktreePath' 'TaskWorktreePath' $(fd -e go)
sd 'NewSharedPlanWorktree' 'NewSharedTaskWorktree' $(fd -e go)
sd 'CommitPlanScaffoldOnMain' 'CommitTaskScaffoldOnMain' $(fd -e go)
sd 'EnsurePlanBranch' 'EnsureTaskBranch' $(fd -e go)
sd 'MergePlanBranch' 'MergeTaskBranch' $(fd -e go)
sd 'ResetPlanBranch' 'ResetTaskBranch' $(fd -e go)
mv session/git/plan_lifecycle.go session/git/task_lifecycle.go
mv session/git/plan_lifecycle_test.go session/git/task_lifecycle_test.go

# ============================================================
# PART G: cmd/ renames
# ============================================================

sd 'NewPlanCmd' 'NewTaskCmd' $(fd -e go)
sd 'executePlanRegister' 'executeTaskRegister' cmd/plan.go
sd 'executePlanList' 'executeTaskList' cmd/plan.go cmd/plan_test.go
sd 'executePlanListWithStore' 'executeTaskListWithStore' cmd/plan.go
sd 'executePlanSetStatus' 'executeTaskSetStatus' cmd/plan.go
sd 'executePlanTransition' 'executeTaskTransition' cmd/plan.go
sd 'executePlanImplement' 'executeTaskImplement' cmd/plan.go
sd 'executePlanLinkClickUp' 'executeTaskLinkClickUp' cmd/plan.go cmd/plan_test.go
sd 'loadPlanState' 'loadTaskState' cmd/plan.go
# projectFromPlansDir stays — refers to docs/plans/ directory path
mv cmd/plan.go cmd/task.go
mv cmd/plan_test.go cmd/task_test.go

# ============================================================
# PART H: Rename test files with "plan" in name
# ============================================================

mv app/app_plan_actions_test.go app/app_task_actions_test.go
mv app/app_plan_completion_test.go app/app_task_completion_test.go
mv app/app_plan_context_actions_test.go app/app_task_context_actions_test.go
mv app/app_plan_creation_test.go app/app_task_creation_test.go
mv app/plan_cancel_rename_delay_test.go app/task_cancel_rename_delay_test.go
mv app/plan_title.go app/task_title.go
mv app/plan_title_test.go app/task_title_test.go

# ============================================================
# PART I: config/ renames (PlanStore → DatabaseURL)
# The config field is the URL to the store server. Since the DB
# will serve as the core kasmos database (not just tasks), use
# a generic name. Internal struct fields (planStore→taskStore)
# stay task-specific since they hold a taskstore.Store client.
# ============================================================

sd 'PlanStore string' 'DatabaseURL string' config/config.go
sd '"plan_store,omitempty"' '"database_url,omitempty"' config/config.go
sd '\.PlanStore' '.DatabaseURL' $(fd -e go -p 'config/')
sd 'cfg\.PlanStore' 'cfg.DatabaseURL' $(fd -e go)
sd 'config\.PlanStore' 'config.DatabaseURL' $(fd -e go)
sd 'PlanStore string' 'DatabaseURL string' config/toml.go
sd '"plan_store,omitempty"' '"database_url,omitempty"' config/toml.go
sd '\.PlanStore' '.DatabaseURL' config/toml.go
sd 'tomlResult\.PlanStore' 'tomlResult.DatabaseURL' config/config.go

# (factory.go planStoreURL→storeURL already done in Task 1)

# Update comment in config.go
sd '// PlanStore is the URL of the remote plan store server' '// DatabaseURL is the URL of the remote kasmos store server' config/config.go

# Update comment in factory.go
sd 'local SQLite planstore' 'local SQLite kasmos database' config/taskstore/factory.go

# Update appConfig.PlanStore → appConfig.DatabaseURL in app/app.go
sd 'appConfig\.PlanStore' 'appConfig.DatabaseURL' $(fd -e go -p 'app/')

# Rename planStoreURL local var in app/app.go to storeURL
sd 'planStoreURL' 'storeURL' $(fd -e go -p 'app/')
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename PlanFile→TaskFile, plan-prefixed vars/methods, and file renames"
```

## Wave 3: CLI Commands, HTTP API Paths, SQL Schema, and User-Facing Strings

> **depends on wave 2:** CLI commands reference the renamed functions from wave 2. HTTP API and SQL changes must happen after the Go code is updated.

### Task 6: Update CLI command names and user-facing strings

**Files:**
- Modify: `cmd/task.go` — change cobra `Use: "plan"` to `Use: "task"`, update help text
- Modify: `app/app_actions.go` — update context menu labels and toast messages
- Modify: `app/app_state.go` — update toast messages and audit log messages
- Modify: `app/app.go` — update toast messages and confirmation dialogs
- Modify: `app/app_input.go` — update overlay titles

**Step 1: write the failing test**

No new tests — string updates only.

**Step 2: run test to verify baseline passes**

```bash
go test ./cmd/... ./app/... -count=1 2>&1 | tail -10
```

expected: PASS (baseline)

**Step 3: update CLI and user-facing strings**

```bash
# --- cmd/task.go cobra command names ---
sd 'Use:   "plan"' 'Use:   "task"' cmd/task.go
sd 'Short: "manage plan lifecycle' 'Short: "manage task lifecycle' cmd/task.go
sd '"list all plans with status"' '"list all tasks with status"' cmd/task.go
sd '"register an untracked plan file' '"register an untracked task file' cmd/task.go
sd '"force-override a plan' '"force-override a task' cmd/task.go
sd '"apply an FSM event to a plan"' '"apply an FSM event to a task"' cmd/task.go
sd '"trigger implementation of a specific wave"' '"trigger implementation of a specific wave"' cmd/task.go  # fine as-is
sd 'registered: %s' 'registered: %s' cmd/task.go  # fine as-is
sd 'implementation triggered: %s' 'implementation triggered: %s' cmd/task.go  # fine as-is
sd '"backfill ClickUp task IDs from plan content' '"backfill ClickUp task IDs from task content' cmd/task.go

# --- app/ context menu labels (lowercase per project convention) ---
sd '"start plan"' '"start planning"' app/app_actions.go
sd '"view plan"' '"view task"' app/app_actions.go
sd '"rename plan"' '"rename task"' app/app_actions.go
sd '"cancel plan"' '"cancel task"' app/app_actions.go
sd '"start over plan"' '"start over task"' app/app_actions.go
sd '"chat about this"' '"chat about this"' app/app_actions.go  # fine as-is
sd '"set topic"' '"set topic"' app/app_actions.go  # fine as-is
sd '"merge to main"' '"merge to main"' app/app_actions.go  # fine as-is
sd '"inspect plan"' '"inspect task"' app/app_actions.go

# --- app/ toast messages and confirmation dialogs ---
sd "plan '%s' is ready" "task '%s' is ready" app/app.go
sd "cancel plan '%s'?" "cancel task '%s'?" app/app_actions.go
sd "start over plan '%s'" "start over task '%s'" app/app_actions.go
sd 'plan cancelled by user' 'task cancelled by user' app/app_actions.go
sd 'plan merged to main' 'task merged to main' app/app_actions.go
sd 'plan needs ## Wave headers' 'task needs ## Wave headers' app/app_actions.go
sd 'plan not found' 'task not found' app/app_actions.go app/app_state.go
sd 'no plan state loaded' 'no task state loaded' app/app_actions.go app/app_state.go
sd 'missing plan state' 'missing task state' app/app_actions.go

# --- app/ audit log messages ---
sd 'plan merged to main:' 'task merged to main:' app/app_actions.go
sd 'plan cancelled by user:' 'task cancelled by user:' app/app_actions.go

# --- overlay titles ---
sd '"rename instance"' '"rename instance"' app/app_actions.go  # fine
sd '"ask about this plan"' '"ask about this task"' app/app_actions.go
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./cmd/... ./app/... -count=1 2>&1 | tail -10
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: update CLI command from kas plan to kas task and user-facing strings"
```

### Task 7: Update HTTP API paths and SQLite schema migration

**Files:**
- Modify: `config/taskstore/server.go` — change `/plans` routes to `/tasks`
- Modify: `config/taskstore/http.go` — change URL builders from `/plans` to `/tasks`
- Modify: `config/taskstore/sqlite.go` — add migration renaming `plans` table to `tasks`, update all SQL
- Modify: `config/taskstore/server_test.go` — update test paths
- Modify: `config/taskstore/http_test.go` — update test paths

**Step 1: write the failing test**

Add a migration test to `config/taskstore/sqlite_test.go`:

```go
func TestSQLiteMigration_PlansTableToTasks(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Store should work — the migration creates the tasks table
	err = store.Create("proj", TaskEntry{Filename: "test.md", Status: StatusReady})
	require.NoError(t, err)

	entries, err := store.List("proj")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "test.md", entries[0].Filename)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run TestSQLiteMigration_PlansTableToTasks -v
```

expected: FAIL or PASS depending on whether schema already uses `tasks`

**Step 3: update HTTP paths and SQL schema**

```bash
# --- HTTP API paths in server.go ---
sd '/v1/projects/{project}/plans' '/v1/projects/{project}/tasks' config/taskstore/server.go
sd '"plan not found:' '"task not found:' config/taskstore/server.go

# --- HTTP client paths in http.go ---
sd '/plans"' '/tasks"' config/taskstore/http.go
sd '/plans/' '/tasks/' config/taskstore/http.go

# --- SQL schema: rename table from plans to tasks ---
sd 'CREATE TABLE IF NOT EXISTS plans' 'CREATE TABLE IF NOT EXISTS tasks' config/taskstore/sqlite.go

# --- SQL queries: plans → tasks ---
sd 'INTO plans ' 'INTO tasks ' config/taskstore/sqlite.go
sd 'FROM plans' 'FROM tasks' config/taskstore/sqlite.go
sd 'UPDATE plans' 'UPDATE tasks' config/taskstore/sqlite.go
sd 'ALTER TABLE plans' 'ALTER TABLE tasks' config/taskstore/sqlite.go
sd 'table_info(plans)' 'table_info(tasks)' config/taskstore/sqlite.go
sd '"list plans:' '"list tasks:' config/taskstore/sqlite.go
sd '"list plans by status:' '"list tasks by status:' config/taskstore/sqlite.go
sd '"list plans by topic:' '"list tasks by topic:' config/taskstore/sqlite.go
sd '"iterate plans:' '"iterate tasks:' config/taskstore/sqlite.go

# --- Test files ---
sd '/plans' '/tasks' config/taskstore/server_test.go config/taskstore/http_test.go
```

Then add the table rename migration to `config/taskstore/sqlite.go`. In the `NewSQLiteStore` function, after the existing migrations, add:

```go
// Rename plans table to tasks (idempotent — only runs if plans exists and tasks doesn't).
const plansToTasksMigration = `ALTER TABLE plans RENAME TO tasks`
```

Call it in `NewSQLiteStore` after the existing migration calls, wrapped in an idempotent check:

```go
// Migrate: rename plans → tasks (if old table exists)
migrateRenameTable(db, "plans", "tasks")
```

Add the helper:

```go
func migrateRenameTable(db *sql.DB, oldName, newName string) {
	// Check if old table exists
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", oldName).Scan(&count)
	if err != nil || count == 0 {
		return // old table doesn't exist, nothing to migrate
	}
	// Check if new table already exists
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", newName).Scan(&count)
	if err != nil || count > 0 {
		return // new table already exists
	}
	_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName))
}
```

Also update the `schema` const to use `tasks` instead of `plans` (already done by the `sd` command above). The migration handles existing databases; new databases get the `tasks` table directly from the schema.

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./config/taskstore/... -count=1 -v
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: rename HTTP API paths /plans→/tasks and migrate SQLite schema"
```
