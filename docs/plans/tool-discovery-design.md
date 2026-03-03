# Tool Discovery for kq init

## Problem

The init wizard scaffolds agent files with `{{TOOLS_REFERENCE}}` injected from `tools-reference.md`. This reference lists 14 CLI tools unconditionally. If a tool isn't installed, the agent gets instructions to use a binary that doesn't exist — producing broken commands and wasted context window.

## Decision

Add an interactive tool discovery stage to the init wizard that scans PATH for available tools and lets the user confirm which to include. The scaffold then generates a filtered tools-reference containing only selected tools.

## Approach: Static catalog in wizard + filter function in scaffold

### Data model

`wizard.State` gains:

```go
SelectedTools []string // binary names the user confirmed, e.g. ["sg", "difft", "sd"]
```

### Wizard stage (stage 4)

`wizard/stage_tools.go` defines a static catalog:

```go
var toolCatalog = []struct{ Binary, Name string }{
    {"sg", "ast-grep"}, {"comby", "comby"}, {"difft", "difftastic"},
    {"sd", "sd"}, {"yq", "yq"}, {"mlr", "miller"}, {"glow", "glow"},
    {"typos", "typos"}, {"scc", "scc"}, {"tokei", "tokei"},
    {"watchexec", "watchexec"}, {"hyperfine", "hyperfine"},
    {"procs", "procs"}, {"mprocs", "mprocs"},
}
```

`runToolsStage(state *State) error`:
- Runs `exec.LookPath` on each binary
- Builds huh multi-select: found tools pre-checked with detected path, missing tools unchecked with "(not found)" label
- User can re-add missing tools (for planned installs)
- Stores final selection in `state.SelectedTools`

Slotted as stage 4 in `wizard.Run()`, after phases and before return.

### Markdown filter

`scaffold/tools_filter.go` exposes:

```go
func FilterToolsReference(content string, selected []string) string
```

Line-by-line state machine with states: `preamble`, `in_category`, `in_tool`, `in_table`.

- **Tool entries**: `^- \*\*name\*\* (\`binary\`):` starts a tool block. Buffers bullet + all indented continuation lines. Emits only if binary is in `selected`.
- **Category headers**: `^### ` buffered, emitted only when first included tool in category is confirmed. Discarded if category has zero included tools.
- **"When to Use What" table**: Header/separator always emitted. Data rows emitted only if backtick-quoted binaries in the row are all in `selected`. Entire table suppressed if all data rows filtered.
- **Preamble**: `## Available CLI Tools` + intro paragraph always emitted.

### Scaffold integration

`scaffold.go`:
- `loadToolsReference()` becomes `loadFilteredToolsReference(selected []string) string`
- `writePerRoleProject`, `WriteCodexProject`, `ScaffoldAll` gain `selectedTools []string` parameter
- `renderTemplate` receives the filtered string via callers

`initcmd.go`:
- Passes `state.SelectedTools` to `scaffold.ScaffoldAll`

### Testing

`scaffold/tools_filter_test.go` — table-driven:
- All tools selected: output matches input
- No tools selected: only preamble remains
- One category fully absent: header stripped
- Partial category: header kept, missing entries removed
- Table row filtering: rows with missing tools removed
- Multi-line sub-bullets: entire entry emitted or dropped atomically

`wizard/stage_tools_test.go` — table-driven:
- Detection logic extracted to `detectTools(catalog, lookupFn)` with injectable lookup
- Tests simulate found/not-found without touching PATH

### Files changed

| File | Change |
|------|--------|
| `wizard/wizard.go` | Add `SelectedTools` to `State`, add stage 4 call |
| `wizard/stage_tools.go` | **New.** Catalog, detection, huh stage |
| `wizard/stage_tools_test.go` | **New.** Detection tests |
| `scaffold/tools_filter.go` | **New.** `FilterToolsReference` state machine |
| `scaffold/tools_filter_test.go` | **New.** Filter tests |
| `scaffold/scaffold.go` | `loadFilteredToolsReference`, add `selectedTools` param |
| `scaffold/scaffold_test.go` | Update for new parameter |
| `initcmd.go` | Pass `state.SelectedTools` to scaffold |
| `initcmd_test.go` | Update for new parameter |

### Wizard flow

```
harness → agents → phases → tools → scaffold
```

4 new files, 5 modified files. No new packages, no new dependencies.
