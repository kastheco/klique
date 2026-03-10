---
applyTo: ".agents/skills/**/*.md,internal/initcmd/scaffold/templates/skills/**/*.md,.opencode/agents/**/*.md,.claude/agents/**/*.md,internal/initcmd/scaffold/templates/opencode/agents/**/*.md,internal/initcmd/scaffold/templates/claude/agents/**/*.md,AGENTS.md,.codex/AGENTS.md,internal/initcmd/scaffold/templates/codex/AGENTS.md,.github/copilot-instructions.md,.github/instructions/**/*.instructions.md"
---

These files define kasmos agent behavior, Copilot guidance, or scaffolded prompt content. Review them like a kasmos reviewer/fixer would:

- Preserve role boundaries: planners plan, coders implement, reviewers review, fixers investigate and recover.
- Keep evidence-first language. Do not replace investigation and verification steps with speculative advice.
- Keep tool guidance aligned with repo conventions: prefer `rg`, `fd`, `sd`, `difft`, `typos`, and `scc` over legacy unix tools.
- If a scaffold source exists, update source and mirror together; do not patch only one copy.
- Skills in `.agents/skills/...` must stay in sync with `internal/initcmd/scaffold/templates/skills/...`.
- Agent prompt templates in `internal/initcmd/scaffold/templates/{opencode,claude}/agents/...` must stay in sync with runtime prompt copies such as `.opencode/agents/...` and `.claude/agents/...`.
- Treat scaffold drift as a blocking review issue.
- Keep Copilot review instructions concise and repository-specific so they stay useful within GitHub's code review instruction limits.
