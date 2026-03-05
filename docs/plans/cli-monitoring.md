# CLI Monitoring & Observability Commands

**Goal:** Add CLI commands for querying the audit log, managing orphaned tmux sessions, and viewing instance status summaries. Closes Category 5 (Monitoring & Observability) in the UI/CLI feature parity report.

**Order:** 3 of 4 in CLI parity series. Independent of `cli-signal-processing.md` — CAN run in parallel with it (and with `cli-clickup-integration.md`). Only requires `cli-plan-lifecycle.md` to be done (for shared CLI infrastructure patterns).

**Depends on:** `cli-plan-lifecycle.md`

**Architecture:** Three independent subcommand groups:
- `kas audit` — queries the existing SQLite audit log (`config/auditlog/`). Data already exists, just needs a CLI reader.
- `kas tmux` — wraps tmux CLI to discover orphaned `kas_*` sessions, adopt them into the instance list, or kill them. Reuses logic from `app_actions.go:handleTmuxBrowserAction`.
- `kas instance status` — adds a summary subcommand showing running/ready/paused/killed counts.

Each is self-contained and testable independently.

**Tech Stack:** Go, cobra, `config/auditlog`, `session/tmux`, `config.State`

**Size:** Small-Medium (estimated ~3 hours, 3 tasks, 1 wave — all tasks are independent)

---

## Wave 1

### Task 1: `kas audit` subcommand

Add `kas audit list` cobra command that queries the existing SQLite audit log.

**Files:**
- `cmd/audit.go` — new cobra command: `kas audit list [--limit=N] [--event=X]`
- `cmd/audit_test.go` — table-driven tests for flag parsing, output format, empty log

**Details:**
- Implement these top-level functions in `cmd/audit.go`:

```go
func NewAuditCmd() *cobra.Command
func executeAuditList(logger auditlog.Logger, project string, limit int, event string) (string, error)
func openAuditLogger() (*auditlog.SQLiteLogger, error)
```

- Follow the existing cmd-layer pattern from `cmd/instance.go` and `cmd/task.go`: keep cobra `RunE` thin, put deterministic logic in `execute...` helpers, and wrap operational errors with `%w` context (`fmt.Errorf("query audit events: %w", err)`).
- Use `resolveRepoInfo()` (already in `cmd/task.go`) to derive `project`; query only that project via `auditlog.QueryFilter{Project: project, Limit: limit}` so output mirrors the active repo context used by the TUI.
- Build event filtering by setting `Kinds` only when `--event` is non-empty:

```go
filter := auditlog.QueryFilter{Project: project, Limit: limit}
if event != "" {
	filter.Kinds = []auditlog.EventKind{auditlog.EventKind(event)}
}
```

- Use `taskstore.ResolvedDBPath()` in `openAuditLogger()` (same shared DB path used in `app/app.go` when initializing the audit logger), then `auditlog.NewSQLiteLogger(dbPath)`.
- Keep `--limit` default at 50; reject non-positive values in command argument handling (return a validation error before querying).
- Format output with `text/tabwriter` using columns: `TIME`, `EVENT`, `DETAILS`. Derive `DETAILS` as `Message` plus `Detail` when present (for richer context). Timestamp format should be stable and sortable (e.g. `2006-01-02 15:04:05`).
- Empty-state behavior must exactly print `no audit entries found` (lowercase, no punctuation), then return nil error.
- Required imports for `cmd/audit.go`:

```go
import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)
```

- Testing guidance for `cmd/audit_test.go`: use in-memory logger (`auditlog.NewSQLiteLogger(":memory:")`), seed mixed events, and table-drive cases for default limit, `--event` filter, table output shape, and empty log output.

- Command wiring should mirror the existing thin `RunE` style in `cmd/task.go:547` and `cmd/instance.go:396`:

```go
func NewAuditCmd() *cobra.Command {
	auditCmd := &cobra.Command{Use: "audit", Short: "query audit events"}
	var limit int
	var event string
	listCmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("limit must be > 0")
			}
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			logger, err := openAuditLogger()
			if err != nil {
				return err
			}
			defer logger.Close()
			out, err := executeAuditList(logger, project, limit, event)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	listCmd.Flags().IntVar(&limit, "limit", 50, "max rows")
	listCmd.Flags().StringVar(&event, "event", "", "event kind filter")
	auditCmd.AddCommand(listCmd)
	return auditCmd
}
```

- `executeAuditList` should be deterministic and pure (string builder + filter construction only):
  - Build filter from arguments first, then call `logger.Query(filter)`.
  - For each event row, format `TIME` with local display format `2006-01-02 15:04:05`.
  - Build `DETAILS` as:
    - `Message` when `Detail == ""`
    - `Message + " | " + Detail` when both are non-empty
    - `Detail` when `Message == ""`
  - Return wrapped query errors exactly as `fmt.Errorf("query audit events: %w", err)` (pattern from `config/auditlog/sqlite.go:142`).

- Edge cases to handle explicitly:
  - `limit <= 0` should fail fast in `RunE` before opening DB (no side effects).
  - `logger.Query` returning empty slice should produce exactly `no audit entries found\n`.
  - Unknown `--event` values are not validated in cmd layer; they map to `auditlog.EventKind(event)` and naturally return zero rows.
  - `openAuditLogger()` should only wrap initialization failures (`open sqlite db for audit log`, `run audit log schema`) and not swallow them.

- Test function layout (table-driven, testify) in `cmd/audit_test.go`:
  - `TestAuditList_FormatsTable`
  - `TestAuditList_Empty`
  - `TestAuditList_EventFilter`
  - `TestAuditCmd_RejectsNonPositiveLimit`
  - Use `auditlog.NewSQLiteLogger(":memory:")`, seed with at least two different kinds, and assert header row includes `TIME\tEVENT\tDETAILS`.


- Codebase anchors to mirror while implementing:
  - `cmd/task.go:543` (`show`) and `cmd/task.go:563` (`update-content`) demonstrate the preferred cmd pattern here: resolve repo context, call a pure helper, print returned string, and keep side effects localized.
  - `app/app.go:388` and `app/app.go:435` confirm audit logging uses the same DB file as taskstore via `taskstore.ResolvedDBPath()`; `openAuditLogger()` should follow that exact path source.
  - `config/auditlog/sqlite.go:92` already guarantees `ORDER BY timestamp DESC`; keep cmd output in query order (no extra sorting).
- Root wiring requirement so the command is reachable: extend `NewRootCmd()` in `cmd/cmd.go` to register `NewAuditCmd()` alongside existing subcommands.
- Keep `executeAuditList` deterministic by isolating formatting helpers inside `cmd/audit.go` (no stdout writes in helpers):

```go
func formatAuditDetails(message, detail string) string
func renderAuditRows(events []auditlog.Event) string
```

- Testing specifics from current patterns:
  - Follow `cmd/instance_test.go` style (`assert` + `require`, table-driven cases, helper constructors).
  - In seeded events, set explicit `Timestamp` values so assertions on table row ordering are deterministic even when rows are emitted rapidly.
  - Add one invalid-limit command execution test that asserts the exact error text `limit must be > 0` before any DB open attempt.
- Keep `openAuditLogger()` intentionally thin so upstream error text from `auditlog.NewSQLiteLogger` remains intact for assertions:

```go
func openAuditLogger() (*auditlog.SQLiteLogger, error) {
	return auditlog.NewSQLiteLogger(taskstore.ResolvedDBPath())
}
```

- Add command reachability coverage (same style as `cmd/serve_test.go`) to avoid wiring regressions:
  - `cmd, _, err := NewRootCmd().Find([]string{"audit", "list"})`
  - assert `err == nil` and `cmd.Name() == "list"`.

- Additional implementation guardrails from codebase reads:
  - Keep cobra/output boundaries identical to `cmd/task.go:543` and `cmd/task.go:552`: `RunE` does repo resolution + dependency setup, while `executeAuditList` returns `(string, error)` with zero stdout writes.
  - Render timestamps with `event.Timestamp.Local().Format("2006-01-02 15:04:05")` so CLI output aligns with existing UI display behavior in `app/app_state.go:1967`; storage remains UTC via `auditFormatTime` in `config/auditlog/sqlite.go:186`.
  - Preserve query ordering as returned (already DESC in `config/auditlog/sqlite.go:138`), and avoid re-sorting in cmd helpers.
  - Add tiny pure-helper tests to lock behavior without cobra plumbing:

```go
func TestFormatAuditDetails(t *testing.T)
func TestRenderAuditRows_HeaderAndOrder(t *testing.T)
```

**Tests:**
- `go test ./cmd/... -run TestAudit -count=1`

---

### Task 2: `kas tmux` subcommand group

Add `kas tmux list`, `kas tmux adopt`, and `kas tmux kill` cobra commands for orphan tmux session management.

**Files:**
- `cmd/tmux.go` — new cobra command group with `list`, `adopt <session> <title>`, `kill <session>`
- `cmd/tmux_test.go` — unit tests (mock tmux output where needed)

**Details:**
- Important constraint: do **not** import `session/tmux` from `cmd` package (import cycle: `session/tmux -> cmd`). Follow the same local-mirror strategy documented in `cmd/instance.go` for tmux name/state handling.
- Implement local parsing around `tmux ls -F` using the same format string as `session/tmux/tmux_session.go`:

```go
"#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}"
```

- Implement these signatures in `cmd/tmux.go`:

```go
type tmuxSessionRow struct {
	Name     string
	Title    string
	Created  time.Time
	Windows  int
	Attached bool
	Width    int
	Height   int
	Managed  bool
}

func NewTmuxCmd() *cobra.Command
func discoverKasSessions(exec Executor, known map[string]struct{}) ([]tmuxSessionRow, error)
func executeTmuxList(state config.StateManager, exec Executor) (string, error)
func executeTmuxAdopt(state config.StateManager, sessionName, title, repoRoot string, now time.Time, exec Executor) error
func executeTmuxKill(state config.StateManager, sessionName string, exec Executor) error
```

- Build `known` tmux names from persisted instances via existing helpers in the same package: `loadInstanceRecords(state)` + `kasTmuxName(record.Title)` (pattern from `app/app_state.go:discoverTmuxSessions`).
- `kas tmux list`: discover all `kas_` sessions, then print only orphan rows (`Managed == false`) with `NAME`, `TITLE`, `WINDOWS`, `ATTACHED`, `AGE`; if none, print `no orphan tmux sessions found`.
- `kas tmux adopt <session-name> <title>`:
  - Validate session exists in current orphan set (exact name match; no prefix matching).
  - Validate `title` is non-empty and does not collide with an existing instance title.
  - Append a new `instanceRecord` to state without mutating existing records (preserve full JSON round-trip behavior established in `cmd/instance.go`).
  - Seed adopted record as `Status: instanceReady`, `Program: "unknown"`, `Path: repoRoot`, `CreatedAt/UpdatedAt: now`.
- `kas tmux kill <session-name>`:
  - Validate target exists in orphan set before killing.
  - Execute `tmux kill-session -t <session-name>` through injected `Executor`.
  - Return wrapped execution errors (`fmt.Errorf("kill tmux session %s: %w", sessionName, err)`).
- Treat `tmux ls` exit errors indicating no server/no sessions as an empty list (same behavior as `session/tmux.DiscoverAll`); malformed output lines should be skipped, not fatal.
- Required imports for `cmd/tmux.go`:

```go
import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/spf13/cobra"
)
```

- Testing guidance for `cmd/tmux_test.go`: table-drive parser/list formatting, orphan filtering, adopt validation failures, and state preservation checks (reuse `newTestStateFromRecords` + `assertRecordFieldsEqual` patterns from `cmd/instance_test.go`).

- Command tree wiring in `NewTmuxCmd()` should match root command style in `cmd/cmd.go:44` and other groups:

```go
func NewTmuxCmd() *cobra.Command {
	tmuxCmd := &cobra.Command{Use: "tmux", Short: "manage orphan tmux sessions"}
	listCmd := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: ...}
	adoptCmd := &cobra.Command{Use: "adopt <session> <title>", Args: cobra.ExactArgs(2), RunE: ...}
	killCmd := &cobra.Command{Use: "kill <session>", Args: cobra.ExactArgs(1), RunE: ...}
	tmuxCmd.AddCommand(listCmd, adoptCmd, killCmd)
	return tmuxCmd
}
```

- `discoverKasSessions` implementation shape:
  - Execute `tmux ls -F "#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}"` through injected `Executor.Output`.
  - Treat `*exec.ExitError` as empty list (same contract as `session/tmux.DiscoverAll` in `session/tmux/tmux_session.go:595`).
  - Parse with `strings.SplitN(line, "|", 6)`; skip malformed lines (`len(parts) < 6`) without failing.
  - Ignore non-`kas_` sessions and compute `Managed` from `known[name]`.
  - Parse epoch/windows/width/height with `strconv`; keep zero values on parse failure (non-fatal).

- Reuse existing state round-trip patterns from `cmd/instance.go:262`, `cmd/instance.go:358`:
  - Read records with `loadInstanceRecords(state)`.
  - For `adopt`, append a fully populated `instanceRecord` and re-marshal the full slice (`json.Marshal(records)` then `state.SaveInstances(raw)`) so existing fields are preserved.
  - Derive known session names with `kasTmuxName(r.Title)` exactly (same sanitization path used elsewhere).

- `executeTmuxAdopt` validation order (deterministic and testable):
  1. `strings.TrimSpace(title) != ""`
  2. title uniqueness against existing instance records (`r.Title == title`)
  3. orphan existence by exact session name (`row.Name == sessionName` and `!row.Managed`)
  4. persist new record with `Status: instanceReady`, `Program: "unknown"`, `Path: repoRoot`, `CreatedAt: now`, `UpdatedAt: now`
  - Return explicit errors for each validation branch so tests can assert messages.

- `executeTmuxKill` requirements:
  - Recompute orphan set and ensure target exists before attempting kill.
  - Use `exec.Run(exec.Command("tmux", "kill-session", "-t", sessionName))`.
  - Wrap failure as `fmt.Errorf("kill tmux session %s: %w", sessionName, err)`.

- `executeTmuxList` formatting contract:
  - Only include orphan rows (`Managed == false`).
  - Header: `NAME\tTITLE\tWINDOWS\tATTACHED\tAGE`.
  - AGE can reuse overlay semantics from `ui/overlay/tmuxBrowserOverlay.go:240` (`s ago`, `m ago`, `h ago`, `d ago`); keep it deterministic in tests by passing fixed `Created` values.
  - Empty result must be exactly `no orphan tmux sessions found\n`.

- Suggested test cases in `cmd/tmux_test.go`:
  - `TestDiscoverKasSessions_ParsesAndSkipsMalformed`
  - `TestExecuteTmuxList_FiltersManaged`
  - `TestExecuteTmuxList_Empty`
  - `TestExecuteTmuxAdopt_Validation`
  - `TestExecuteTmuxAdopt_PreservesExistingFields` (use `assertRecordFieldsEqual` from `cmd/instance_test.go:207`)
  - `TestExecuteTmuxKill_WrapsExecutorError`
  - Use `cmd/cmd_test.NewMockExecutor()` for command stubbing.


- Codebase anchors to mirror while implementing:
  - `cmd/cmd.go:10` defines the package-level `Executor` abstraction; reuse it directly in tmux helpers so tests can inject `cmd/cmd_test.NewMockExecutor()`.
  - `session/tmux/tmux_session.go:591` is the canonical behavior for tmux discovery (`tmux ls -F ...`, treat `*exec.ExitError` as empty, skip malformed rows).
  - `app/app_state.go:1801` shows how known managed names are derived from instance titles and mapped to `kas_` names.
  - `app/app_state.go:1900` is the source of truth for orphan adoption defaults (`Program: "unknown"`, path rooted at active repo).
- Root wiring requirement so `kas tmux ...` is reachable: extend `NewRootCmd()` in `cmd/cmd.go` to register `NewTmuxCmd()`.
- Add a small pure helper for age formatting in `cmd/tmux.go` so output tests stay deterministic:

```go
func relativeAge(now, created time.Time) string
```

  Implement with the same buckets as `ui/overlay/tmuxBrowserOverlay.go:241` (`s/m/h/d ago`).
- Adopt persistence must preserve full record payloads exactly like existing state mutations in `cmd/instance.go:358`:
  - load all records via `loadInstanceRecords(state)`
  - append one fully initialized `instanceRecord`
  - `json.Marshal(records)`
  - `state.SaveInstances(raw)`
- Testing specifics from current patterns:
  - Reuse `newTestStateFromRecords` and `assertRecordFieldsEqual` from `cmd/instance_test.go` for round-trip guarantees.
  - Use `cmd/cmd_test.NewMockExecutor()` and assert invoked commands via `ToString(cmd)` when validating `kill-session` behavior.
  - Include a case where `tmux ls` returns non-`kas_` sessions + malformed lines in one payload; parser should return only valid `kas_` rows without error.
- Use explicit, stable validation errors in `executeTmuxAdopt` / `executeTmuxKill` so tests can assert exact branches:
  - `title must not be empty`
  - `instance title already exists: <title>`
  - `orphan tmux session not found: <session>`
- Seed adopted records with a fully explicit struct literal (do not rely on incidental zero values), mirroring `instanceRecord` in `cmd/instance.go`:

```go
rec := instanceRecord{
	Title:     title,
	Path:      repoRoot,
	Status:    instanceReady,
	Program:   "unknown",
	CreatedAt: now,
	UpdatedAt: now,
}
```

- Add root registration coverage so the command group cannot be orphaned from `kas`:
  - `cmd, _, err := NewRootCmd().Find([]string{"tmux", "list"})`
  - `cmd, _, err := NewRootCmd().Find([]string{"tmux", "adopt"})`
  - `cmd, _, err := NewRootCmd().Find([]string{"tmux", "kill"})`

- Additional implementation guardrails from codebase reads:
  - Keep the local tmux row parser behavior byte-for-byte compatible with `session/tmux/tmux_session.go:591` (exact format string, `SplitN(..., 6)`, skip malformed lines, and treat any `*exec.ExitError` as empty result).
  - Derive the human title exactly as `strings.TrimPrefix(name, "kas_")` (same as `session/tmux/tmux_session.go:632`); do not normalize case or punctuation further.
  - Use `config.StateManager` + `loadInstanceRecords`/`SaveInstances` round-trip from `cmd/instance.go:262` and `cmd/instance.go:358` to avoid dropping optional fields during adopt writes.
  - Prefer command-level tests for deterministic behavior, and pure-function tests for parsing/age formatting:

```go
func TestRelativeAge_Buckets(t *testing.T)
func TestDiscoverKasSessions_ExitErrorIsEmpty(t *testing.T)
```

  - For `kill`, verify exact command invocation with `ToString(cmd)` from `cmd/cmd.go:36`, asserting `tmux kill-session -t <session>`.

**Tests:**
- `go test ./cmd/... -run TestTmux -count=1`

---

### Task 3: `kas instance status` subcommand

Add `kas instance status` summary command showing running/ready/paused/killed instance counts.

**Files:**
- `cmd/instance.go` — new or extend existing cobra command: `kas instance status`
- `cmd/instance_test.go` — tests for status aggregation and output format

**Details:**
- Extend `NewInstanceCmd()` in `cmd/instance.go` with a new `status` subcommand (`Use: "status"`, `Args: cobra.NoArgs`) that reads state via `config.LoadState()` and delegates to a pure helper.
- Implement these signatures:

```go
type instanceStatusSummary struct {
	Running int
	Ready   int
	Paused  int
	Killed  int
}

func summarizeInstanceStatus(records []instanceRecord) instanceStatusSummary
func executeInstanceStatus(state config.StateManager) (string, error)
```

- Reuse `loadInstanceRecords(state)` for JSON parsing; keep parse errors wrapped with `parse instances: %w` convention already used in `cmd/instance.go`.
- Aggregation rules should mirror existing app summary semantics (`app/app_state.go`):
  - `running`: statuses `instanceRunning` and `instanceLoading`
  - `ready`: status `instanceReady`
  - `paused`: status `instancePaused`
  - `killed`: any unknown status value (future-proof bucket for stale/invalid state)
- Output format: tabwriter table with two columns (`STATE`, `COUNT`) and rows in fixed order: running, ready, paused, killed.
- Empty-state behavior must exactly print `no instances found` and return nil error.
- Keep existing `kas instance list` behavior untouched; `status` is additive, not a list replacement.
- Testing guidance for `cmd/instance_test.go`:
  - Add table-driven coverage for mixed statuses including `instanceLoading` and an unknown status value.
  - Verify empty-state string.
  - Verify deterministic row order in text output.
  - Verify subcommand registration by checking `NewInstanceCmd().Find([]string{"status"})`.

- Subcommand wiring should follow existing `list` command structure in `cmd/instance.go:393`:

```go
statusCmd := &cobra.Command{
	Use:   "status",
	Short: "show status summary for all instances",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := executeInstanceStatus(config.LoadState())
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}
instanceCmd.AddCommand(statusCmd)
```

- `summarizeInstanceStatus(records []instanceRecord)` should contain only aggregation logic (no I/O, no formatting):
  - `instanceRunning` and `instanceLoading` increment `Running`.
  - `instanceReady` increments `Ready`.
  - `instancePaused` increments `Paused`.
  - default branch increments `Killed`.
  - This matches existing app counting in `app/app_state.go:469` where running includes loading.

- `executeInstanceStatus(state config.StateManager)` should:
  - Call `loadInstanceRecords(state)` and propagate parse failures (`parse instances: %w`) unchanged.
  - Return `no instances found\n` when `len(records) == 0`.
  - Format table via `tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)` with fixed row order:
    1. `running`
    2. `ready`
    3. `paused`
    4. `killed`
  - Return `(sb.String(), nil)` for deterministic output tests.

- Edge cases to cover explicitly:
  - Unknown persisted status values (e.g. `-1`, `99`) go to `killed`, not dropped.
  - Records with zero-value fields other than `Status` still count normally.
  - Invalid JSON in `state.InstancesData` returns error, does not fall back silently.

- Concrete tests to add to `cmd/instance_test.go`:
  - `TestSummarizeInstanceStatus_Mixed`
  - `TestExecuteInstanceStatus_Empty`
  - `TestExecuteInstanceStatus_OutputOrder`
  - `TestExecuteInstanceStatus_ParseError`
  - `TestNewInstanceCmd_HasStatusSubcommand` (same style as `cmd/serve_test.go:10`).


- Codebase anchors to mirror while implementing:
  - `app/app_state.go:470` is the existing status-summary semantic source: running includes both running and loading.
  - `cmd/instance.go:262` (`loadInstanceRecords`) is the canonical parse path; preserve its wrapped error contract (`parse instances: %w`).
  - `cmd/instance.go:393` shows current subcommand wiring style to replicate for `status`.
- Keep `summarizeInstanceStatus` strictly pure and exhaustive over known local enum values:

```go
func summarizeInstanceStatus(records []instanceRecord) instanceStatusSummary {
    var s instanceStatusSummary
    for _, r := range records {
        switch r.Status {
        case instanceRunning, instanceLoading:
            s.Running++
        case instanceReady:
            s.Ready++
        case instancePaused:
            s.Paused++
        default:
            s.Killed++
        }
    }
    return s
}
```

- `executeInstanceStatus` should follow existing formatter style in this file:
  - `tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)`
  - fixed header `STATE\tCOUNT`
  - fixed row order `running`, `ready`, `paused`, `killed`
  - exact empty-state text `no instances found\n`
- Testing specifics from current patterns:
  - Add a subcommand registration test using `cmd, _, err := NewInstanceCmd().Find([]string{"status"})` (same shape as `cmd/serve_test.go:11`).
  - Add a parse-error test by setting `state.InstancesData` to invalid JSON and asserting `executeInstanceStatus` returns an error containing `parse instances`.
  - Add an unknown-status aggregation case (for example `instanceStatus(99)`) and assert it increments `Killed`.
- Keep rendering deterministic and local to `executeInstanceStatus` (no shared global writer), following existing table style in this package:

```go
fmt.Fprintln(w, "STATE\tCOUNT")
fmt.Fprintf(w, "running\t%d\n", summary.Running)
fmt.Fprintf(w, "ready\t%d\n", summary.Ready)
fmt.Fprintf(w, "paused\t%d\n", summary.Paused)
fmt.Fprintf(w, "killed\t%d\n", summary.Killed)
```

- Add root-level command wiring coverage in addition to `NewInstanceCmd()` coverage:
  - `cmd, _, err := NewRootCmd().Find([]string{"instance", "status"})`
  - assert `err == nil` and `cmd.Name() == "status"`.

- Additional implementation guardrails from codebase reads:
  - Keep status semantics aligned with existing app behavior in `app/app_state.go:470`: `loading` contributes to the running bucket.
  - Reuse parsing path `loadInstanceRecords(state)` (`cmd/instance.go:262`) so malformed JSON still returns the established wrapped error prefix `parse instances:`.
  - Keep rendering style consistent with existing cmd tables in `cmd/instance.go:245` (`tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)`) and return newline-terminated output strings for stable snapshot assertions.
  - Add an explicit unknown-status case using the local enum type cast:

```go
records := []instanceRecord{{Title: "stale", Status: instanceStatus(99)}}
summary := summarizeInstanceStatus(records)
assert.Equal(t, 1, summary.Killed)
```

**Tests:**
- `go test ./cmd/... -run TestInstance -count=1`
