# Chat & Custodian Customizations Implementation Plan

**Goal:** Rename "custodial" → "custodian" throughout the codebase and make both chat and custodian agent types configurable in the setup wizard.

**Architecture:** The rename touches the `session.AgentType*` constant, runtime profile resolution in `app/app_state.go`, scaffold templates (agent `.md` files, `opencode.jsonc`, embedded skills), and `check/project.go`. Wizard configurability requires adding `custodian` to wizard role lists, removing the `maxNavigableIndex` cap that hides chat/custodian, adding wizard placeholder substitution for the custodian block in `opencode.jsonc`, and adding custodian to the phase mapping. Custodian moves from `staticAgentRoles` (always scaffolded) to wizard-managed (scaffolded like coder/reviewer/planner).

**Tech Stack:** Go, bubbletea wizard TUI, embedded templates, TOML config

**Size:** Medium (estimated ~3 hours, 4 tasks, 1 wave)

---

## Wave 1: Rename + Wizard Integration

All tasks are independent at the compilation level — each leaves the codebase compilable. However, full runtime correctness requires all 4 tasks applied together (profile key must match constant value, template filenames must match role names, skill names must match `check/project.go`).

### Task 1: Rename AgentType Constant + Runtime References

**Files:**
- Modify: `session/instance.go`
- Modify: `app/app_state.go`
- Modify: `app/app_test.go`

**Changes:**

1. `session/instance.go` — rename constant:
   ```go
   // Before
   AgentTypeCustodial = "custodial"
   // After
   AgentTypeCustodian = "custodian"
   ```

2. `app/app_state.go` — update all references:
   - `session.AgentTypeCustodial` → `session.AgentTypeCustodian` (3 occurrences: `programForAgent` switch case, `spawnAdHocAgent` program + type assignment)
   - `ResolveProfile("custodial", ...)` → `ResolveProfile("custodian", ...)` in `programForAgent`

3. `app/app_test.go` — update assertion:
   ```go
   // Before
   assert.Equal(t, session.AgentTypeCustodial, last.AgentType, ...)
   // After
   assert.Equal(t, session.AgentTypeCustodian, last.AgentType, ...)
   ```

**Verify:**
```bash
go build ./...
go test ./session/... ./app/... -v -count=1
```

**Commit:** `refactor: rename custodial agent type to custodian`

---

### Task 2: Wizard — Add Custodian Role + Make Chat/Custodian Navigable

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go`
- Modify: `internal/initcmd/wizard/roles.go`
- Modify: `internal/initcmd/wizard/model_agents.go`
- Modify: `internal/initcmd/wizard/model.go`
- Modify: `internal/initcmd/wizard/model_agents_test.go`
- Modify: `internal/initcmd/wizard/roles_test.go`

**Changes:**

1. `wizard.go` — add custodian to `DefaultAgentRoles()`:
   ```go
   func DefaultAgentRoles() []string {
       return []string{"coder", "reviewer", "planner", "chat", "custodian"}
   }
   ```

2. `wizard.go` — add custodian to `RoleDefaults()`:
   ```go
   "custodian": {
       Role:        "custodian",
       Model:       "anthropic/claude-sonnet-4-6",
       Effort:      "low",
       Temperature: "0.1",
       Enabled:     true,
   },
   ```

3. `roles.go` — add custodian to `RoleDescription()`:
   ```go
   "custodian": "Operational agent for workflow fixes and cleanup.\nResets plan states, manages branches, removes stale worktrees.",
   ```

4. `roles.go` — add custodian to `RolePhaseText()`:
   ```go
   "custodian": "Default for phases: custodian",
   ```

5. `model_agents.go` — remove the `maxNavigableIndex` cap so all 5 roles (coder, reviewer, planner, chat, custodian) are navigable:
   ```go
   func (m *agentStepModel) maxNavigableIndex() int {
       max := len(m.agents) - 1
       if max < 0 {
           return 0
       }
       return max
   }
   ```

6. `model.go` — add custodian to the `PhaseMapping`:
   ```go
   PhaseMapping: map[string]string{
       "implementing":   "coder",
       "spec_review":    "reviewer",
       "quality_review": "reviewer",
       "planning":       "planner",
       "custodian":      "custodian",
   },
   ```

7. Update tests:
   - `model_agents_test.go` — update `TestAgentStep_BrowseNavigation` to include all 5 roles and verify chat + custodian are now navigable (remove the "chat is skipped" assertion, add downward navigation to index 3 and 4).
   - `roles_test.go` — add custodian to any role coverage tests.

**Verify:**
```bash
go test ./internal/initcmd/wizard/... -v -count=1
```

**Commit:** `feat: add custodian to wizard roles and make chat/custodian navigable`

---

### Task 3: Scaffold — Rename Templates + Wizard-Managed Custodian

**Files:**
- Rename: `internal/initcmd/scaffold/templates/claude/agents/custodial.md` → `custodian.md`
- Rename: `internal/initcmd/scaffold/templates/opencode/agents/custodial.md` → `custodian.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/opencode.jsonc`
- Modify: `internal/initcmd/scaffold/scaffold.go`
- Modify: `internal/initcmd/scaffold/scaffold_test.go`

**Changes:**

1. Rename template files from `custodial.md` to `custodian.md` for both claude and opencode harnesses. Update content: "Custodial Agent" → "Custodian Agent", "custodial agent" → "custodian agent", "kasmos-custodial" → "kasmos-custodian" skill reference.

2. `opencode.jsonc` — rename the `"custodial"` block key to `"custodian"` and add wizard placeholders:
   ```jsonc
   "custodian": {
       "model": "{{CUSTODIAN_MODEL}}",
       ...
       {{CUSTODIAN_EFFORT_LINE}}
       "temperature": {{CUSTODIAN_TEMP}},
       "textVerbosity": "low"
   }
   ```

3. `scaffold.go` — remove `"custodial"` from `staticAgentRoles` (empty the slice or remove the entry), since custodian is now wizard-managed:
   ```go
   var staticAgentRoles = []string{}
   ```

4. `scaffold.go` — add `"custodian"` to the `renderOpenCodeConfig` substitution loop alongside coder/planner/reviewer:
   ```go
   for _, role := range []string{"coder", "planner", "reviewer", "custodian"} {
   ```

5. Update `scaffold_test.go`:
   - Rename `TestScaffold_IncludesCustodialAgent` → `TestScaffold_IncludesCustodianAgent` and update to pass custodian as a wizard-managed agent in the agents list instead of checking it as a static agent.
   - Update `TestScaffoldFiltersByHarness`: change `custodial.md` assertions to `custodian.md` and add custodian to the agents list.
   - Update comments referencing "custodial static agent".

**Verify:**
```bash
go test ./internal/initcmd/scaffold/... -v -count=1
```

**Commit:** `feat: rename custodial templates to custodian and make wizard-managed`

---

### Task 4: Rename Embedded Skill (kasmos-custodial → kasmos-custodian)

**Files:**
- Rename directory: `internal/initcmd/scaffold/templates/skills/kasmos-custodial/` → `kasmos-custodian/`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-custodian/SKILL.md` (content update)
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md`
- Modify: `internal/check/project.go`
- Rename directory: `.opencode/skills/kasmos-custodial/` → `kasmos-custodian/`
- Modify: `.opencode/skills/kasmos-custodian/SKILL.md` (content update)
- Modify: `.opencode/skills/kasmos-lifecycle/SKILL.md`

**Changes:**

1. Rename the embedded skill template directory from `kasmos-custodial` to `kasmos-custodian`.

2. Update `kasmos-custodian/SKILL.md` — rename all self-references:
   - Frontmatter `name: kasmos-custodial` → `name: kasmos-custodian`
   - Description: `kasmos custodial agent` → `kasmos custodian agent`
   - Heading: `# kasmos-custodial` → `# kasmos-custodian`
   - Body text: "custodial agent" → "custodian agent"
   - Signal filenames: `custodial-cleanup-*` → `custodian-cleanup-*`, `custodial-triage-*` → `custodian-triage-*`, `custodial-done-*` → `custodian-done-*`

3. Update `kasmos-lifecycle/SKILL.md` — update the role table:
   - `| custodial | handles ops: ... | \`kasmos-custodial\` |` → `| custodian | handles ops: ... | \`kasmos-custodian\` |`
   - Description reference: `use kasmos-planner, kasmos-coder, kasmos-reviewer, or kasmos-custodial instead` → `... kasmos-custodian instead`

4. `internal/check/project.go` — update `EmbeddedSkillNames`:
   ```go
   var EmbeddedSkillNames = []string{
       "kasmos-coder",
       "kasmos-custodian",  // was kasmos-custodial
       "kasmos-lifecycle",
       "kasmos-planner",
       "kasmos-reviewer",
   }
   ```

5. Apply the same directory rename and content updates to the live `.opencode/skills/` copies:
   - Rename `.opencode/skills/kasmos-custodial/` → `.opencode/skills/kasmos-custodian/`
   - Update `.opencode/skills/kasmos-custodian/SKILL.md` content
   - Update `.opencode/skills/kasmos-lifecycle/SKILL.md` references

**Verify:**
```bash
go build ./...
go test ./internal/check/... -v -count=1
```

**Commit:** `refactor: rename kasmos-custodial skill to kasmos-custodian`
