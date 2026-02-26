# Enforce CLI-tools via kas init

## Problem

The cli-tools skill instructs agents to use modern replacements (`rg`, `sd`, `comby`, `difft`, `scc`) instead of legacy tools (`grep`, `sed`, `awk`, `diff`, `wc -l`). Agents routinely ignore these instructions. We need enforcement that intercepts Bash tool calls before execution and rejects banned commands with an error pointing to the correct replacement.

## Scope

Two harnesses, two enforcement mechanisms, one shared banned-tools definition:

| Harness     | Mechanism                                | Location                                                                     |
| ----------- | ---------------------------------------- | ---------------------------------------------------------------------------- |
| Claude Code | `PreToolUse` hook (bash script)          | `~/.claude/hooks/enforce-cli-tools.sh` + entry in `~/.claude/settings.json`  |
| opencode    | `tool.execute.before` plugin hook (JS)   | `~/.config/opencode/plugins/enforce-cli-tools.js`                            |

Both installed automatically during `kas init` when the respective harness is selected.

## Banned tools table

Single source of truth, embedded in kasmos binary:

| Banned                             | Replacement                  | Message                                                              |
| ---------------------------------- | ---------------------------- | -------------------------------------------------------------------- |
| `grep`                             | `rg`                         | rg is faster, respects .gitignore, and has better defaults           |
| `sed`                              | `sd` or `comby`              | sd for simple replacements, comby for structural/multi-line rewrites |
| `awk`                              | `yq`/`jq`, `sd`, or `comby` | yq/jq for structured data, sd for text, comby for code patterns     |
| `diff` (standalone, not `git diff`)| `difft`                      | difftastic provides syntax-aware structural diffs                    |
| `wc -l`                            | `scc`                        | scc provides language-aware counts with complexity estimates         |

## Architecture

```
kas init
  ├─ wizard stages (existing)
  ├─ InstallSuperpowers (existing)
  ├─ SyncGlobalSkills (existing)
  ├─ InstallEnforcement (NEW)
  │    ├─ Claude: write hook script + merge into settings.json
  │    └─ opencode: write plugin JS file
  ├─ Write TOML config (existing)
  └─ Scaffold project files (existing)
```

New `InstallEnforcement() error` method on the `Harness` interface. Codex gets a no-op.

## Claude Code enforcement

**Hook script**: Embedded Go template at `internal/initcmd/scaffold/templates/claude/enforce-cli-tools.sh`. Reads JSON from stdin, extracts `tool_input.command`, checks for banned tools with word-boundary regex, exits 2 with descriptive stderr on match.

**Settings.json merge**:
1. Read `~/.claude/settings.json` (create if missing)
2. Parse as JSON (`map[string]any`)
3. Ensure `hooks.PreToolUse` array exists
4. Check if entry with `enforce-cli-tools.sh` already exists (idempotent)
5. If not, append `{ "matcher": "Bash", "hooks": [{ "type": "command", "command": "~/.claude/hooks/enforce-cli-tools.sh" }] }`
6. Write back with 2-space indentation

**File placement**: `~/.claude/hooks/enforce-cli-tools.sh` (chmod +x). Always overwritten on re-init (we own this file).

## opencode enforcement

**Plugin file**: `~/.config/opencode/plugins/enforce-cli-tools.js`. Uses `tool.execute.before` hook.

**Strategy** (two-layer, since throw-to-block is undocumented):
1. **Primary**: Throw an error — if opencode propagates it as a tool failure, the agent sees the error message
2. **Fallback**: If throwing doesn't block, mutate `output.args.command` to `echo "BLOCKED: ..." >&2; exit 2`

Plugin structure:
```js
export const EnforceCLIToolsPlugin = async ({ client, directory }) => {
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      const cmd = output.args?.command;
      if (!cmd) return;
      // check banned tools, rewrite command on match
    }
  };
};
```

**Installation**: Written to `~/.config/opencode/plugins/enforce-cli-tools.js`. opencode auto-discovers plugins from the plugins directory. Always overwritten on re-init.

## Integration into kas init

New stage in `initcmd.Run()`:

```
Stage 4a:   InstallSuperpowers (existing)
Stage 4a-2: SyncGlobalSkills (existing)
Stage 4a-3: InstallEnforcement (NEW)
Stage 4b:   Write TOML config (existing)
Stage 4c:   Scaffold project files (existing)
```

Iterates selected harnesses, calls `InstallEnforcement()` on each. Output follows existing pattern:
```
Installing enforcement hooks...
  claude       OK
  opencode     OK
```

**Idempotent**: Hook script and plugin JS are always overwritten (kasmos owns them). Settings.json entries are checked before adding (no duplicates).

## Testing

- **Unit tests**: Shared test cases — list of `(command, shouldBlock, expectedMessage)` tuples
- **Claude hook**: Pipe JSON through bash script, assert exit codes and stderr
- **opencode plugin**: Import JS module, call hook function with mock input/output, assert args mutation or throw
- **Integration**: `kas init` in temp dir with mock harness detection, verify files written to correct locations and settings.json merged correctly

## Alternatives considered

1. **Shell wrappers in managed PATH**: Bulletproof but dangerous — PATH leak breaks the user's terminal. Rejected.
2. **MCP proxy server**: Can't replace native Bash tool, agent would ignore the proxy. Fundamental design flaw. Rejected.
3. **Stricter skill messaging**: Already tried, agents ignore it. That's why we're here.
