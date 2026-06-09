#!/usr/bin/env bash
#
# validate_fork.sh — smoke test the fork-and-pause endpoint against a running
# Helix API. Designed to be run by a human reviewer (or future agent) to
# confirm the wire-up works end-to-end on a real process, not just against
# the in-memory store used by unit tests.
#
# Usage:
#   HELIX_TOKEN=<jwt>  HELIX_PARENT_SESSION_ID=<ses_...>  [HELIX_URL=http://localhost:8080]  ./validate_fork.sh
#
# How to get the inputs:
#   - HELIX_TOKEN: register at $HELIX_URL/auth/register, response includes `token`.
#     Or copy a session JWT from devtools (Application → Cookies on the inner Helix).
#   - HELIX_PARENT_SESSION_ID: from a `zed_external` spec task session in the UI.
#     Look at the spec task → planning_session_id, OR list sessions via the API.
#
# The script:
#   1. Reads the parent's current code_agent_runtime.
#   2. Picks a target runtime different from the parent's (zed_agent → claude_code, or vice versa).
#   3. POST /sessions/{parent}/fork with the target.
#   4. Asserts: 200 + new_session_id, parent now paused with forked_to:<child>,
#      child has parent_session_id pointing back, child has a fork_seed
#      interaction with non-empty response_message.
#   5. POST /sessions/{parent}/messages — asserts 409 with "paused" in body.
#   6. POST /sessions/{parent}/fork — asserts 409 (fork-from-paused).
#
# Exits 0 on all green, non-zero on any failure (printing which assertion).
#
set -euo pipefail

: "${HELIX_TOKEN:?HELIX_TOKEN env var is required (JWT bearer token)}"
: "${HELIX_PARENT_SESSION_ID:?HELIX_PARENT_SESSION_ID env var is required (ses_...)}"
HELIX_URL="${HELIX_URL:-http://localhost:8080}"

PARENT="$HELIX_PARENT_SESSION_ID"
AUTH=(-H "Authorization: Bearer $HELIX_TOKEN" -H "Content-Type: application/json")
PASS=0; FAIL=0

# Small helpers --------------------------------------------------------------
check() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ✓ $label ($actual)"
    PASS=$((PASS+1))
  else
    echo "  ✗ $label — expected: $expected, got: $actual" >&2
    FAIL=$((FAIL+1))
  fi
}

check_contains() {
  local label="$1" needle="$2" haystack="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "  ✓ $label (contains: $needle)"
    PASS=$((PASS+1))
  else
    echo "  ✗ $label — expected substring: $needle, got: $haystack" >&2
    FAIL=$((FAIL+1))
  fi
}

check_nonempty() {
  local label="$1" value="$2"
  if [[ -n "$value" && "$value" != "null" ]]; then
    echo "  ✓ $label (length=${#value})"
    PASS=$((PASS+1))
  else
    echo "  ✗ $label — expected non-empty, got: '$value'" >&2
    FAIL=$((FAIL+1))
  fi
}

# Step 1: read parent runtime ------------------------------------------------
echo "==> Step 1: inspect parent session $PARENT"
PARENT_JSON="$(curl -sS "${AUTH[@]}" "$HELIX_URL/api/v1/sessions/$PARENT")"
PARENT_RUNTIME="$(jq -r '.config.code_agent_runtime' <<< "$PARENT_JSON")"
PARENT_AGENT_TYPE="$(jq -r '.config.agent_type' <<< "$PARENT_JSON")"
PARENT_PAUSED="$(jq -r '.config.paused // false' <<< "$PARENT_JSON")"

check "agent_type is zed_external" "zed_external" "$PARENT_AGENT_TYPE"
check "parent is not paused"       "false"        "$PARENT_PAUSED"
check_nonempty "parent runtime"    "$PARENT_RUNTIME"

# Pick a different target. We support the two most common runtimes; extend as needed.
if [[ "$PARENT_RUNTIME" == "claude_code" ]]; then
  TARGET_RUNTIME="zed_agent"
else
  TARGET_RUNTIME="claude_code"
fi
echo "    fork target runtime: $TARGET_RUNTIME"

# Step 2: fork ---------------------------------------------------------------
echo "==> Step 2: POST /sessions/$PARENT/fork → expect 200 + new_session_id"
FORK_RESP="$(curl -sS -w '\n%{http_code}' "${AUTH[@]}" -X POST \
  -d "{\"code_agent_runtime\":\"$TARGET_RUNTIME\"}" \
  "$HELIX_URL/api/v1/sessions/$PARENT/fork")"
FORK_CODE="${FORK_RESP##*$'\n'}"
FORK_BODY="${FORK_RESP%$'\n'*}"

check "fork HTTP code" "200" "$FORK_CODE"
CHILD="$(jq -r '.new_session_id' <<< "$FORK_BODY")"
check_nonempty "new_session_id" "$CHILD"

# Step 3: parent should now be paused ----------------------------------------
echo "==> Step 3: re-read parent — expect paused=true, paused_reason=forked_to:$CHILD"
PARENT_JSON2="$(curl -sS "${AUTH[@]}" "$HELIX_URL/api/v1/sessions/$PARENT")"
check "parent.paused"        "true"                 "$(jq -r '.config.paused' <<< "$PARENT_JSON2")"
check "parent.paused_reason" "forked_to:$CHILD"     "$(jq -r '.config.paused_reason' <<< "$PARENT_JSON2")"
check_nonempty "parent.paused_at" "$(jq -r '.config.paused_at' <<< "$PARENT_JSON2")"

# Step 4: child should have fork lineage + fork_seed interaction -------------
echo "==> Step 4: read child $CHILD — expect lineage + fork_seed interaction"
CHILD_JSON="$(curl -sS "${AUTH[@]}" "$HELIX_URL/api/v1/sessions/$CHILD")"
check "child.parent_session_id"     "$PARENT"        "$(jq -r '.config.parent_session_id' <<< "$CHILD_JSON")"
check "child.code_agent_runtime"    "$TARGET_RUNTIME" "$(jq -r '.config.code_agent_runtime' <<< "$CHILD_JSON")"
check_nonempty "child.forked_at" "$(jq -r '.config.forked_at' <<< "$CHILD_JSON")"

FORK_SEED_RESPONSE="$(jq -r '.interactions[] | select(.trigger == "fork_seed") | .response_message' <<< "$CHILD_JSON")"
check_nonempty "child has fork_seed interaction with non-empty response_message" "$FORK_SEED_RESPONSE"

FORK_SEED_PROMPT="$(jq -r '.interactions[] | select(.trigger == "fork_seed") | .prompt_message' <<< "$CHILD_JSON")"
check_contains "fork_seed prompt names the parent" "$PARENT" "$FORK_SEED_PROMPT"

# Step 5: send to paused parent should 409 -----------------------------------
echo "==> Step 5: POST /sessions/$PARENT/messages — expect 409 with paused reason"
MSG_RESP="$(curl -sS -w '\n%{http_code}' "${AUTH[@]}" -X POST \
  -d '{"content":"this should be rejected by pause enforcement"}' \
  "$HELIX_URL/api/v1/sessions/$PARENT/messages")"
MSG_CODE="${MSG_RESP##*$'\n'}"
MSG_BODY="${MSG_RESP%$'\n'*}"
check "send-to-paused HTTP code" "409" "$MSG_CODE"
check_contains "send-to-paused body mentions reason" "forked_to:$CHILD" "$MSG_BODY"

# Step 6: fork-from-paused should also 409 -----------------------------------
echo "==> Step 6: POST /sessions/$PARENT/fork again — expect 409 (fork-from-paused)"
RE_FORK_RESP="$(curl -sS -w '\n%{http_code}' "${AUTH[@]}" -X POST \
  -d '{"code_agent_runtime":"qwen_code"}' \
  "$HELIX_URL/api/v1/sessions/$PARENT/fork")"
RE_FORK_CODE="${RE_FORK_RESP##*$'\n'}"
RE_FORK_BODY="${RE_FORK_RESP%$'\n'*}"
check "fork-from-paused HTTP code" "409" "$RE_FORK_CODE"
check_contains "fork-from-paused body suggests descendant" "active descendant" "$RE_FORK_BODY"

# Summary --------------------------------------------------------------------
echo ""
echo "==> Summary"
echo "    PASS: $PASS"
echo "    FAIL: $FAIL"
if [[ "$FAIL" -gt 0 ]]; then
  echo "FAILED" >&2
  exit 1
fi
echo "ALL GREEN"
