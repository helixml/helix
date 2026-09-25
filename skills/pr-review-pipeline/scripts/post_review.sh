#!/usr/bin/env bash
# post_review.sh — post a finished review to GitHub and PROVE it landed.
#
# Encodes the pipeline rule: "never claim a review you cannot verify".
# Exactly one POST attempt (no retry loops — see auth-ops.md), then a GET
# on the reviews API. Exit 0 ONLY when that GET shows exactly one review
# by us at the target commit.
#
# usage: post_review.sh <owner/repo> <pr-number> [review.json]   # default: ./review.json
# env:   GH_TOKEN or GITHUB_TOKEN  (fetch with get_secret right before running this)
#        GH_API                     (API base, default https://api.github.com; GHE override)
#
# review.json shape:
#   { "event": "APPROVE" | "COMMENT",
#     "commit_id": "<head sha the review was written against>",
#     "body_file": "<path to review body markdown>" }
#
# Failure mode on any non-2xx: the raw status, response body, and
# x-github-request-id / x-accepted-github-permissions headers are pasted to
# stderr verbatim for the report. The script does not guess and does not retry.
set -euo pipefail

die() { printf 'post_review: %s\n' "$*" >&2; exit 1; }

REPO="${1:?usage: post_review.sh <owner/repo> <pr-number> [review.json]}"
PR="${2:?usage: post_review.sh <owner/repo> <pr-number> [review.json]}"
REVIEW_JSON="${3:-review.json}"
API="${GH_API:-https://api.github.com}"
TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"

[ -n "$TOKEN" ] || die "no token in GH_TOKEN/GITHUB_TOKEN — get_secret immediately before running this (see auth-ops.md §1)"
command -v curl >/dev/null || die "curl required"
command -v jq >/dev/null || die "jq required"
[ -f "$REVIEW_JSON" ] || die "review manifest not found: $REVIEW_JSON"

REVIEW_DIR="$(cd "$(dirname "$REVIEW_JSON")" && pwd)"
EVENT="$(jq -re '.event' "$REVIEW_JSON")" || die "review.json missing .event"
COMMIT="$(jq -re '.commit_id' "$REVIEW_JSON")" || die "review.json missing .commit_id"
BODY_REL="$(jq -re '.body_file' "$REVIEW_JSON")" || die "review.json missing .body_file"
# Optional inline findings: [{"path":...,"line":...,"body":...}], ranges via start_line,
# subject_type:"FILE" only when line-anchoring is impossible (one short sentence each, max 5).
COMMENTS="$(jq -c '.comments // []' "$REVIEW_JSON")"
if [ "$COMMENTS" != "[]" ]; then
  jq -e 'all(.[]; .path and .body and (.line != null or .subject_type == "FILE"))' <<<"$COMMENTS" >/dev/null \
    || die "each review.json .comments entry needs path, body, and line (or subject_type==\"FILE\")"
  [ "$(jq 'length' <<<"$COMMENTS")" -le 5 ] || die "max 5 inline comments (operator policy)"
fi
case "$EVENT" in APPROVE|COMMENT) ;; *) die "invalid event '$EVENT' — APPROVE|COMMENT only; REQUEST_CHANGES is rejected by policy: a bot review must never block a merge" ;; esac
[[ "$COMMIT" =~ ^[0-9a-fA-F]{40}$ ]] || die "commit_id must be a full 40-char sha, got '$COMMIT'"

BODY_FILE="$BODY_REL"
[ -f "$BODY_FILE" ] || BODY_FILE="$REVIEW_DIR/$BODY_REL"
[ -f "$BODY_FILE" ] || die "body file not found: $BODY_REL"

PAYLOAD="$(mktemp)"; RESP="$(mktemp)"; HEADERS="$(mktemp)"
trap 'rm -f "$PAYLOAD" "$RESP" "$HEADERS"' EXIT
jq -n --arg event "$EVENT" --arg sha "$COMMIT" --rawfile body "$BODY_FILE" --argjson comments "$COMMENTS" \
  '{event:$event, commit_id:$sha, body:$body} + (if $comments == [] then {} else {comments:$comments} end)' > "$PAYLOAD"

API_URL="$API/repos/$REPO/pulls/$PR/reviews"

# --- one POST attempt, no retries (auth-ops.md §2) ------------------------
STATUS="$(curl -sS -o "$RESP" -D "$HEADERS" -w '%{http_code}' \
  -X POST "$API_URL" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  --data @"$PAYLOAD")" || die "POST transport failure (no retry — check network, then re-mint once per auth-ops.md §2)"

dump_failure() {
  {
    printf '== verbatim GitHub response ==\n'
    printf 'status: %s\n' "$STATUS"
    printf 'x-github-request-id: %s\n' "$(grep -im1 '^x-github-request-id:' "$HEADERS" | tr -d '\r' | cut -d' ' -f2-)"
    printf 'x-accepted-github-permissions: %s\n' "$(grep -im1 '^x-accepted-github-permissions:' "$HEADERS" | tr -d '\r' | cut -d' ' -f2-)"
    printf 'message: %s\n' "$(jq -r '.message // "?"' "$RESP" 2>/dev/null || true)"
    printf -- '-- body --\n'; cat "$RESP"; printf '\n'
  } >&2
}

case "$STATUS" in
  2??) ;;
  *)
    dump_failure
    if grep -qim1 'Resource not accessible by integration' "$RESP"; then
      die "POST refused with '$STATUS'. This is an installation/connection fault, NOT GitHub UI permissions (auth-ops.md §4). Salvage: keep review.json on disk, report path (auth-ops.md §6)."
    else
      die "POST refused with '$STATUS'. Headers dumped above; one re-mint then stop (auth-ops.md §2/§3). Do NOT claim the review was posted."
    fi
    ;;
esac

REVIEW_URL="$(jq -r '.html_url // "?"' "$RESP")"
REVIEWER="$(jq -r '.user.login // "?"' "$RESP")"
[ "$REVIEWER" != "?" ] || die "POST returned 2xx without user.login — cannot verify authorship, treating as unverified"

# --- verify: exactly one review by us at the target commit -----------------
PAGE=1; MATCHES=0
while [ "$PAGE" -le 5 ]; do
  LIST="$(curl -sS -f -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" \
    "$API_URL?per_page=100&page=$PAGE")" || die "verification GET failed — review may have posted, but UNVERIFIED, do not claim success"
  [ -n "$LIST" ] && [ "$LIST" != "[]" ] || break
  MATCHES=$(( MATCHES + $(jq --arg u "$REVIEWER" --arg s "$COMMIT" \
    '[.[] | select((.user.login // "" | ascii_downcase) == ($u | ascii_downcase) and (.commit_id // "") == $s)] | length' <<<"$LIST") ))
  PAGE=$((PAGE + 1))
done

if [ "$MATCHES" -eq 1 ]; then
  printf 'VERIFIED: exactly 1 review by %s at %s — %s\n' "$REVIEWER" "$COMMIT" "$REVIEW_URL"
  exit 0
elif [ "$MATCHES" -eq 0 ]; then
  printf 'post_review: UNVERIFIED — POST returned %s but reviews API shows no review by %s at %s. Treat as not posted; inspect before any re-dispatch.\n' "$STATUS" "$REVIEWER" "$COMMIT" >&2
  exit 1
else
  printf 'post_review: UNVERIFIED — %s reviews by %s at %s (>1 = double post, escalate; do not re-post).\n' "$MATCHES" "$REVIEWER" "$COMMIT" >&2
  exit 1
fi
