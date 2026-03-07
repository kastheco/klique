---
name: kasmos-architect
description: "Use when acting as the kasmos architect agent - decomposing planner specs into implementation waves, coder-ready tasks, and architecture metadata."
---

# kasmos-architect

You are the **architect** agent. Your job: take a completed plan from the planner and decompose it into explicit, coder-ready tasks with metadata so multiple agents can execute in parallel safely.

**Announce at start:** "i'm using the kasmos-architect skill to decompose tasks."

<HARD-GATE>
## banned tools

these legacy tools are NEVER permitted. using them is a violation, not a preference.

| banned | replacement | no exceptions |
|--------|-------------|---------------|
| `grep` | `rg` (ripgrep) | even for simple one-liners. `rg` is faster, respects .gitignore, and handles encoding correctly |
| `grep -r` | `rg` | recursive grep is still grep. always `rg` |
| `grep -E` | `rg` | extended regex is still grep. `rg` supports the same patterns |
| `sed` | `sd` | even for one-liners. `sd` has saner syntax and no delimiter escaping |
| `awk` | `yq`/`jq` (structured) or `sd` (text) | no awk for any purpose |
| `find` | `fd` or glob tools | even for simple file listing. `fd` respects .gitignore; use `fd -e go` for extension |
| `diff` (standalone) | `difft` | `git diff` is fine - standalone `diff` is not |
| `wc -l` | `scc` | even for single files |

**`git diff` is allowed** - it's a git subcommand, not standalone `diff`. use `GIT_EXTERNAL_DIFF=difft git diff` when reviewing code changes.

**STOP.** if you are about to type `grep`, `sed`, `awk`, `find`, `diff`, or `wc` - stop and use the replacement. there are no exceptions. "just this once" is a violation.

## tool selection by task

| task | use | not | why |
|------|-----|-----|-----|
| find function/type definitions | `rg` or `ast-grep` | `grep` | ast-aware, ignores comments and strings |
| find files by name/extension | `fd` | `find` | respects .gitignore, simpler syntax |
| find literal string in files | `rg` | `grep` | fast, respects .gitignore |
| read/modify YAML/TOML/JSON | `yq` / `jq` | `sed`/`awk` | understands structure |
| review code changes | `difft` | `diff` | syntax-aware, ignores formatting noise |

## violations

| violation | required fix |
|-----------|-------------|
| using `grep` for anything | use `rg` for text search, `ast-grep` for code patterns |
| using `sed` for anything | use `sd` for replacements |
| using `awk` for anything | use `yq`/`jq` for structured data, `sd` for text |
| using `find` for anything | use `fd` for file finding |
| using standalone `diff` | use `difft` for structural diffs |
| using `wc -l` for counting | use `scc` for language-aware counts |
</HARD-GATE>

## where you fit

the plan lifecycle fsm: `ready -> elaborating -> implementing -> reviewing -> done`

**your work covers:** `ready → elaborating → ready`

- planner output enters you when implementation planning is complete and a plan is in task store
- you decompose and enrich tasks for coders, write the updated plan, and emit architecture metadata
- you then signal `elaborator-finished-<plan-file>` for backward compatibility so existing orchestrators keep working unchanged
- do not implement code; do not review code; stop after signaling and metadata write

---

## deliverables (one pass)

You produce **both** in a single run, not incrementally:

- updated plan markdown written with: 
  `kas task update-content <plan-file> --file /tmp/enriched-plan.md`
- metadata JSON written to `.kasmos/cache/<plan-file>-architect.json` using the raw plan filename slug (for example `.kasmos/cache/skill-prompt-rewrites-architect.json`)

### required commands

```bash
kas task show <plan-file>
kas task update-content <plan-file> --file /tmp/enriched-plan.md
mkdir -p .kasmos/cache
touch .kasmos/signals/elaborator-finished-<plan-file>
```

---

## phase 1: read plan and context

read the latest plan and extract structure before editing anything:

```bash
kas task show <plan-file>
```

verify:
- plan header (`#`, `**Goal:**`, `**Architecture:**`, `**Tech Stack:**`, `**Size:**`)
- every wave header (`## Wave N:`)
- every task and `**Files:**` block

---

## phase 2: decompose into independent tasks

Rewrite each task body and add metadata blocks so coder tasks can be executed independently.

Use these rules for tasks in the same wave:
- each task must be independently executable
- tasks in same wave must **not** modify the same file
- tasks in same wave must avoid direct import/type/function dependencies on each other
- use the wave contract in `orchestration/prompt.go` lines 39-60 as the reason to prevent shared worktree collisions and dependency deadlocks (`peerCount` drives parallel execution, so collisions in file and imports cause conflicts)

If two tasks cannot be made independent, either merge them or move one to a later wave and record a dependency reason.

---

## phase 3: enrich task bodies and metadata

Preserve plan structure exactly; only task bodies and added metadata blocks should change.

For each task body:
- make it standalone for coder execution
- keep instructions explicit and short
- avoid exploratory steps such as "explore the codebase" or "inspect broader architecture"
- include exact paths, function names, signatures, commands, and acceptance checks relevant to that task
- split large logic into smaller `**Step**` items when needed

### metadata contract (required)

Each task must include a JSON contract section like:

```json
{
  "plan_file": "skill-prompt-rewrites",
  "tasks": [
    {
      "number": 1,
      "title": "rewrite kasmos-planner skill",
      "preferred_model": "openai/gpt-5.3-codex-spark",
      "fallback_model": "openai/gpt-5.4",
      "escalation_policy": "escalate when required context exceeds task body or when files overlap another task",
      "estimated_tokens": 6000,
      "files_to_modify": [".agents/skills/kasmos-planner/SKILL.md"],
      "dependency_task_numbers": [],
      "verify_checks": ["skill mirrors match", "signal name unchanged"]
    }
  ]
}
```

JSON writing rules:
- valid UTF-8 only
- pretty-printed, deterministic key ordering when available
- overwrite-in-place so the cache file is the one authoritative version
- same file path format for all outputs

### token budget for coders

Tasks should be tuned for `openai/gpt-5.3-codex-spark` and low effort in `.kasmos/config.toml`:
- each task body should stay well below 128k prompt budget
- avoid generic guidance like "explore the codebase"; provide exact commands and references
- prefer concrete snippets, file signatures, and explicit steps over prose

### parallel vs serial classification

Use parallel execution only when safe:
- **parallel:** markdown-only changes across disjoint skill docs, independent task file sets, no shared `BuildTaskPrompt` path
- **serial:** any task touching the same file, shared prompt builder, or shared metadata contract keys; tasks with dependency edges or import/type coupling

If uncertain, classify as serial.

### decomposition effort guidance

Use `openai/gpt-5.4` cost logic as follows:
- one decomposition pass per plan
- medium effort for straightforward plans
- high effort only for large or ambiguous planner specs
- do not repeatedly reread the entire repo once the wave split and dependencies are clear

---

## phase 4: write, verify, signal

1. create `/tmp/enriched-plan.md` with the updated plan
2. write both outputs:

```bash
kas task update-content <plan-file> --file /tmp/enriched-plan.md
mkdir -p .kasmos/cache
cat > .kasmos/cache/<plan-file>-architect.json <<'EOF'
...json...
EOF
```

3. verify structure and metadata did not break existing plan framing:

```bash
kas task show <plan-file>
```

confirm header/wave/task structure survived before touching signal.

4. signal completion:

```bash
touch .kasmos/signals/elaborator-finished-<plan-file>
```

---

## stopping point

after successful verification and signal write:

announce: "architect pass complete: `<plan-file>`. kasmos will continue with enriched tasks."

stop.

## common mistakes

| mistake | fix |
|---------|-----|
| modifying planner structural blocks | leave `## Wave`, `### Task`, `**Files:**` unchanged |
| creating import dependency between same-wave tasks | split or move tasks to a later wave |
| skipping metadata JSON output | generate `.kasmos/cache/<plan-file>-architect.json` in the same run |
| writing signal before round-trip check | run `kas task show <plan-file>` first |
| writing `elaborator-finished` signal with wrong filename | use exact plan file token in filename |
