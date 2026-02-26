#!/bin/bash
# PreToolUse hook: block legacy CLI tools, enforce modern replacements.
# Installed by kasmos setup. Source of truth: cli-tools skill.
# Reads Bash tool_input.command from stdin JSON and rejects banned commands.

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

[ -z "$COMMAND" ] && exit 0

# grep -> rg (ripgrep)
# Word-boundary match avoids false positives (e.g. ast-grep)
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bgrep\b'; then
  echo "BLOCKED: 'grep' is banned. Use 'rg' (ripgrep) instead. rg is faster, respects .gitignore, and has better defaults." >&2
  exit 2
fi

# sed -> sd or comby
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bsed\b'; then
  echo "BLOCKED: 'sed' is banned. Use 'sd' for simple replacements or 'comby' for structural/multi-line rewrites." >&2
  exit 2
fi

# awk -> yq/jq, sd, or comby
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bawk\b'; then
  echo "BLOCKED: 'awk' is banned. Use 'yq'/'jq' for structured data, 'sd' for text, or 'comby' for code patterns." >&2
  exit 2
fi

# standalone diff (not git diff) -> difft
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bdiff\b' && \
   ! echo "$COMMAND" | grep -qP '\bgit\s+diff\b'; then
  echo "BLOCKED: standalone 'diff' is banned. Use 'difft' (difftastic) for syntax-aware structural diffs. 'git diff' is allowed." >&2
  exit 2
fi

# wc -l -> scc
if echo "$COMMAND" | grep -qP '\bwc\s+(-\w*l|--lines)\b|\bwc\b.*\s-l\b'; then
  echo "BLOCKED: 'wc -l' is banned. Use 'scc' for language-aware line counts with complexity estimates." >&2
  exit 2
fi

exit 0
