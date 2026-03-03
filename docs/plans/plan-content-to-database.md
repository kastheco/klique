# Migrate Plan Content to Database

**Goal:** Rename `planstore.db` to `kasmos.db`, then migrate plan content (waves, tasks, steps) from flat markdown files into relational database tables. Plan files become optional exports — the DB is the source of truth. This unlocks structured queries over plan progress, cross-plan dependency tracking, task-level status, and richer TUI features.

**Key changes:**
- Rename `~/.config/kasmos/planstore.db` → `~/.config/kasmos/kasmos.db` (migration path for existing installs)
- New tables: `waves`, `tasks`, `task_steps` with foreign keys to `plans`
- Parse existing plan markdown into structured rows during migration
- Update `planstore` package to read/write structured plan content
- Update `planparser` to produce structured output compatible with DB insertion
- Remove dependency on plan `.md` files for runtime operation (keep as optional export)
- Update TUI info tab, coder prompts, and wave orchestration to read from DB

**Size:** Large (estimated multi-wave, needs detailed planning)
