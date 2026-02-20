# klique Agents

## Coder
Implementation agent. Writes code, fixes bugs, runs tests.
Follow TDD. Follow the constitution at `.kittify/memory/constitution.md`.

## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs when reviewing changes.
Follow the constitution at `.kittify/memory/constitution.md`.

## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Follow the constitution at `.kittify/memory/constitution.md`.

{{TOOLS_REFERENCE}}
