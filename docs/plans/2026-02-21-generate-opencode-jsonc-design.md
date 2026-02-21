# Generate opencode.jsonc from kq init wizard

## Problem

`kq init` collects model, temperature, and effort per agent via the wizard, but never generates
`.opencode/opencode.jsonc`. OpenCode reads its config from this file — not CLI flags — so wizard
selections are silently discarded. The `BuildFlags()` method on the OpenCode harness returns only
`ExtraFlags` with a comment acknowledging this gap.

## Decision Record

- **Approach**: Embedded JSONC template with `{{PLACEHOLDER}}` substitution (Approach A)
- **Permissions**: Role-based defaults baked into template; dynamic path substitution for `$HOME` and project dir
- **Plugins**: Omitted — users add their own
- **Disabled agents**: `build` and `plan` always emitted with `"disable": true`
- **Chat agent**: Auto-injected with fixed defaults (not wizard-configurable):
  `anthropic/claude-sonnet-4-6`, temperature 0.3, read-only permissions, no explicit reasoning effort
- **Conditional blocks**: Only emit wizard agent stanzas for agents whose harness is `opencode`

## Design

### Template

New embedded file: `internal/initcmd/scaffold/templates/opencode/opencode.jsonc`

Structure mirrors the reference config:
```
{
  "$schema": "https://opencode.ai/config.json",
  "agent": {
    "build":    { "disable": true },
    "plan":     { "disable": true },
    "chat":     { static defaults, read-only perms },
    "coder":    { {{CODER_MODEL}}, {{CODER_TEMP}}, {{CODER_EFFORT}}, full write perms },
    "planner":  { {{PLANNER_MODEL}}, {{PLANNER_TEMP}}, {{PLANNER_EFFORT}}, full write perms },
    "reviewer": { {{REVIEWER_MODEL}}, {{REVIEWER_TEMP}}, {{REVIEWER_EFFORT}}, read-only perms }
  }
}
```

Dynamic path tokens:
- `{{HOME_DIR}}` → `os.UserHomeDir()`
- `{{PROJECT_DIR}}` → project root (`dir` arg to scaffold)

Per-agent tokens (only for opencode-harness agents):
- `{{ROLE_MODEL}}` → wizard-selected model string
- `{{ROLE_TEMP}}` → bare float (no quotes): `0.1`
- `{{ROLE_EFFORT}}` → effort level string; entire `"reasoningEffort"` line stripped if empty

### Rendering function

New `renderOpenCodeConfig()` in `scaffold.go`:

1. Read embedded template
2. Substitute `{{HOME_DIR}}` and `{{PROJECT_DIR}}`
3. For each role (`coder`, `planner`, `reviewer`):
   - If agent harness is `opencode`: substitute model/temp/effort tokens
   - If agent harness is NOT `opencode` or agent is disabled: remove that role's entire JSON block
4. Strip `"reasoningEffort"` lines where effort is empty (regex: line containing `{{ROLE_EFFORT}}` with empty value)
5. Strip `"temperature"` lines where temp is nil/unset
6. Write to `.opencode/opencode.jsonc`

### Chat agent prompt

New: `internal/initcmd/scaffold/templates/opencode/agents/chat.md`

Read-only research agent following the same structure as coder/reviewer/planner prompts.
Has access to CLI tools reference (`{{TOOLS_REFERENCE}}`), but no write/edit guidance.
Framed as a Swiss army knife for codebase exploration and questions.

### Files changed

| File | Change |
|------|--------|
| `scaffold/templates/opencode/opencode.jsonc` | New template |
| `scaffold/templates/opencode/agents/chat.md` | New agent prompt |
| `scaffold/scaffold.go` | `renderOpenCodeConfig()` + integrate into `WriteOpenCodeProject` |
| `scaffold/scaffold_test.go` | Tests for config rendering |

### Not changed

- Wizard stages (chat is not a wizard participant)
- Harness interface
- TOML config structure
- `DefaultAgentRoles()` (chat is scaffold-only, not a klique lifecycle role)
