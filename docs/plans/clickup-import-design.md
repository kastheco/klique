# ClickUp Import — Design

**Date:** 2026-02-23
**Status:** Approved

## Goal

Import ClickUp tasks as kasmos plans. When a ClickUp MCP server is detected in the
user's agent config, show a "+ Import from ClickUp" action in the sidebar plans list.
The imported task is scaffolded into a plan markdown file and a planner agent is
auto-spawned to brainstorm it into an implementable plan with waves.

## Architecture: Hybrid (kasmos fetches, agent brainstorms)

kasmos runs a thin MCP client to search/fetch ClickUp tasks directly, giving the TUI
full control over search results display and task selection. A planner agent handles
the creative work of analyzing the task and producing waves/architecture.

## MCP Detection

kasmos scans these locations at startup (first match wins):

1. Project `.mcp.json` in active repo root
2. `~/.claude/settings.json` → `mcpServers`
3. `~/.claude/settings.local.json` → `mcpServers`

Detection matches any server key containing `clickup` (case-insensitive). The presence
of a ClickUp MCP entry controls whether the import action is visible in the sidebar.

## Transport & Auth

The thin MCP client supports two transports based on the server config:

| Config Pattern | Transport | Auth |
|---|---|---|
| `"type": "http", "url": "https://..."` | Streamable HTTP | OAuth 2.1 PKCE |
| `"command": "npx", "args": [...]` | stdio (JSON-RPC) | Env vars from config |

**OAuth 2.1 PKCE flow** (for HTTP transport):
- First use: open browser to ClickUp auth URL with PKCE challenge, start localhost
  HTTP server on random port for callback, exchange code for token, cache.
- Cache location: `~/.config/kasmos/clickup_oauth.json`
- Token refresh: automatic on expiry using refresh token. Re-trigger browser flow
  if refresh fails.
- TUI feedback: toast "Opening browser for ClickUp authorization..."

**stdio transport**: spawn process from config's `command`/`args`, pass env vars,
speak JSON-RPC 2.0 over stdin/stdout.

The client implements three MCP operations: `initialize`, `tools/list`, `tools/call`.
Tool names for search/get are discovered dynamically from `tools/list` by matching
known ClickUp tool patterns (`clickup_search_tasks`, `clickup_get_task`, etc.).

## TUI Flow

### Sidebar Placement

```
Plans
  topic-a/
    feature-x (implement)
  feature-y (ready)
  + Import from ClickUp          <- only when MCP detected
  -- History --
  old-plan
```

The import action renders as a new `rowKindImportAction` between the last plan/topic
and the History toggle. It uses foam/green color with a `+` prefix, styled to look
like an action rather than a plan entry.

### User Journey

**Step 1 — Click import action**
Opens `TextInputOverlay` titled "Search ClickUp Tasks". User types query, presses Enter.

**Step 2 — Search in progress**
Toast with spinner: "Searching ClickUp...". kasmos calls search tool via thin MCP
client. Results arrive as `[]SearchResult{ID, Name, Status, ListName}`.

**Step 3 — Results picker**
`PickerOverlay` shows formatted results:
```
TASK-123 - Design auth flow (In Progress) -- Backend List
TASK-456 - Update API docs (Open) -- Docs List
```
User navigates with arrow keys, selects with Enter.

**Step 4 — Fetch full task**
Toast: "Fetching task details...". kasmos calls get-task tool for full description,
subtasks, custom fields, comments.

**Step 5 — Plan scaffold**
kasmos writes `docs/plans/YYYY-MM-DD-<sanitized-task-name>.md`:

```markdown
**Goal:** <task description>
**Source:** ClickUp <TASK-ID> (<url>)
**ClickUp Status:** <status>

## Reference: ClickUp Subtasks
- [ ] Subtask 1
- [ ] Subtask 2

## Reference: Custom Fields
- Priority: High
- Sprint: 2026-W09
```

Registers in `plan-state.json` with `status: "planning"`.

**Step 6 — Auto-spawn planner**
`spawnPlanAgent(planFile, "plan", prompt)` with brainstorming-oriented prompt:

> Analyze this imported ClickUp task. The task details and subtasks are included as
> reference. Determine if the task is well-specified enough for implementation or
> needs further analysis. Write a proper implementation plan with `## Wave` sections,
> task breakdowns, architecture notes, and tech stack. Use the ClickUp subtasks as
> a starting point but reorganize into waves based on dependencies.

Normal planner lifecycle takes over (sentinel detection -> confirmation -> implement).

## New Packages

```
internal/
  mcpclient/                     # Thin MCP client (~300 LOC)
    client.go                    # Client interface: Initialize, ListTools, CallTool
    transport_stdio.go           # stdio JSON-RPC transport
    transport_http.go            # Streamable HTTP transport
    oauth.go                     # OAuth 2.1 PKCE (browser, localhost callback, cache)
  clickup/                       # ClickUp-specific logic
    detect.go                    # Scan config files for ClickUp MCP server
    import.go                    # Search, fetch, scaffold plan markdown
    types.go                     # Task, SearchResult, SubTask structs
```

## Modified Files

```
ui/sidebar.go                    # rowKindImportAction, conditional rendering
app/app_actions.go               # Handle import action, orchestrate overlay flow
app/app_state.go                 # Wire new messages into Update loop
```

## Message Types (bubbletea)

```go
type ClickUpDetectedMsg struct{ ServerConfig MCPServerConfig }
type ClickUpSearchStartMsg struct{ Query string }
type ClickUpSearchResultMsg struct{ Results []clickup.SearchResult; Err error }
type ClickUpTaskFetchedMsg struct{ Task clickup.Task; Err error }
type ClickUpImportCompleteMsg struct{ PlanFile string }
```

All I/O in `tea.Cmd` goroutines. Overlay state machine:

```
idle -> searchOverlay -> searching -> pickerOverlay -> fetching -> scaffolding -> plannerSpawned
```

## Unchanged

No modifications to: `config/planfsm/`, `config/planstate/`, `config/planparser/`,
`session/`. The import flow creates plans using the existing plan creation and planner
spawning path. The FSM, state storage, and wave orchestration are reused as-is.

## Error Handling

- **MCP not detected**: import action hidden, no error
- **OAuth cancelled/failed**: toast error, return to sidebar
- **Search returns empty**: picker shows "No tasks found", user can retry or cancel
- **Task fetch fails**: toast error with details, return to sidebar
- **Plan file conflict**: append numeric suffix to filename
- **MCP server unreachable**: toast error with connection details

## Testing

- `internal/mcpclient/`: unit test JSON-RPC encoding, mock transports
- `internal/clickup/detect_test.go`: table-driven config parsing with various formats
- `internal/clickup/import_test.go`: scaffold generation from mock task data
- `ui/sidebar_test.go`: verify import row visibility based on detection state
- `app/`: message handling tests for the import flow state machine
