#!/usr/bin/env bash
# import-plans.sh — import plans and topics from plan-state.json into the plan store.
#
# Insert-only: skips plans/topics that already exist in the store (HTTP 409).
# Requires: jq, curl
#
# Usage:
#   ./contrib/import-plans.sh [plan-state.json] [store-url] [project]
#
# Defaults:
#   plan-state.json  docs/plans/plan-state.json
#   store-url        http://localhost:7433
#   project          basename of current git repo (or "default")

set -euo pipefail

JSON_FILE="${1:-docs/plans/plan-state.json}"
STORE_URL="${2:-http://localhost:7433}"
PROJECT="${3:-$(basename "$(git rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || echo default)}"

STORE_URL="${STORE_URL%/}"

for cmd in jq curl; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "error: $cmd is required but not installed" >&2
    exit 1
  fi
done

if [[ ! -f "$JSON_FILE" ]]; then
  echo "error: $JSON_FILE not found" >&2
  exit 1
fi

# Health check
if ! curl -sf "${STORE_URL}/v1/ping" >/dev/null 2>&1; then
  echo "error: plan store unreachable at ${STORE_URL}" >&2
  echo "       start it with: kas serve" >&2
  exit 1
fi

echo "importing from ${JSON_FILE} → ${STORE_URL} (project: ${PROJECT})"
echo

# --- Topics ---
topic_ok=0
topic_skip=0
topic_fail=0

while IFS= read -r topic_json; do
  name=$(echo "$topic_json" | jq -r '.key')
  payload=$(echo "$topic_json" | jq '{name: .key, created_at: .value.created_at}')

  status=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${STORE_URL}/v1/projects/${PROJECT}/topics" \
    -H 'Content-Type: application/json' \
    -d "$payload")

  case "$status" in
    201) echo "  + topic: ${name}"; ((topic_ok++)) || true ;;
    409) echo "  ~ topic: ${name} (exists)"; ((topic_skip++)) || true ;;
    *)   echo "  ! topic: ${name} (HTTP ${status})"; ((topic_fail++)) || true ;;
  esac
done < <(jq -c '.topics // {} | to_entries[]' "$JSON_FILE")

echo
echo "topics: ${topic_ok} imported, ${topic_skip} skipped, ${topic_fail} failed"
echo

# --- Plans ---
plan_ok=0
plan_skip=0
plan_fail=0

while IFS= read -r plan_json; do
  filename=$(echo "$plan_json" | jq -r '.key')
  payload=$(echo "$plan_json" | jq '{filename: .key} + .value')

  status=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${STORE_URL}/v1/projects/${PROJECT}/plans" \
    -H 'Content-Type: application/json' \
    -d "$payload")

  case "$status" in
    201) echo "  + plan: ${filename}"; ((plan_ok++)) || true ;;
    409) echo "  ~ plan: ${filename} (exists)"; ((plan_skip++)) || true ;;
    *)   echo "  ! plan: ${filename} (HTTP ${status})"; ((plan_fail++)) || true ;;
  esac
done < <(jq -c '.plans // {} | to_entries[]' "$JSON_FILE")

echo
echo "plans: ${plan_ok} imported, ${plan_skip} skipped, ${plan_fail} failed"
echo

total_fail=$((topic_fail + plan_fail))
if [[ "$total_fail" -gt 0 ]]; then
  echo "warning: ${total_fail} item(s) failed — check the store server logs"
  exit 1
fi

echo "done. add this to ~/.config/kasmos/config.toml to use the store:"
echo
echo "  plan_store = \"${STORE_URL}\""
