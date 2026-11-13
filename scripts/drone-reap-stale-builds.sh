#!/usr/bin/env bash
# Cancel Drone builds stuck in running/pending after their runner died
# (for example a runner host reboot). Drone never notices a vanished runner,
# so those builds stay "running" until someone cancels them.
#
# Dry run by default: lists what it would cancel. Pass --apply to cancel.
#
#   DRONE_SERVER=https://drone.example DRONE_TOKEN=... \
#     scripts/drone-reap-stale-builds.sh [--apply] [--max-age-minutes N] [--repo owner/name]
#
# Needs a Drone admin token: it reads /api/builds/incomplete, which spans all repos.
# Age is measured from the build's start (or creation, if it never started).
# The whole response is validated before any cancel, so a malformed reply cancels nothing.
# --max-age-minutes defaults to 240; keep it above the longest legitimate build.
set -euo pipefail

apply=false
max_age_minutes=240
repo_filter=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) apply=true; shift ;;
    --max-age-minutes) [[ $# -ge 2 ]] || { echo "--max-age-minutes needs a value" >&2; exit 2; }; max_age_minutes="$2"; shift 2 ;;
    --repo) [[ $# -ge 2 ]] || { echo "--repo needs a value" >&2; exit 2; }; repo_filter="$2"; shift 2 ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

: "${DRONE_SERVER:?DRONE_SERVER must be set}"
: "${DRONE_TOKEN:?DRONE_TOKEN must be set}"
[[ "$max_age_minutes" =~ ^[0-9]{1,6}$ ]] || { echo "--max-age-minutes must be an integer below 1000000" >&2; exit 2; }

auth_header="Authorization: Bearer ${DRONE_TOKEN}"
now=$(date +%s)
cutoff=$((now - max_age_minutes * 60))

stale=$(curl -fsS -H "$auth_header" "${DRONE_SERVER%/}/api/builds/incomplete" | jq -r \
  --argjson cutoff "$cutoff" --arg repo "$repo_filter" '
  if type != "array" then error("expected a JSON array from /api/builds/incomplete") else . end
  | .[]
  | if (.slug | type) == "string" and (.slug | test("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"))
       and (.build.number | type) == "number" and .build.number > 0
       and (.build.status == "running" or .build.status == "pending")
       and ((.build.started | type) == "number") and ((.build.created | type) == "number")
    then . else error("unexpected build entry: \(tojson)") end
  | select($repo == "" or .slug == $repo)
  | (if .build.started > 0 then .build.started else .build.created end) as $since
  | select($since > 0 and $since < $cutoff)
  | [.slug, .build.number, .build.status, (.build.ref // ""), $since] | @tsv')

if [[ -z "$stale" ]]; then
  echo "no builds older than ${max_age_minutes}m in running/pending"
  exit 0
fi

failed=0
while IFS=$'\t' read -r slug number status ref since; do
  age_minutes=$(((now - since) / 60))
  if [[ "$apply" != true ]]; then
    echo "would cancel ${slug}#${number} (${status}, ${ref}, ${age_minutes}m old)"
    continue
  fi
  if curl -fsS -X DELETE -H "$auth_header" "${DRONE_SERVER%/}/api/repos/${slug}/builds/${number}" >/dev/null; then
    echo "cancelled ${slug}#${number} (${status}, ${ref}, ${age_minutes}m old)"
  else
    echo "FAILED to cancel ${slug}#${number}" >&2
    failed=1
  fi
done <<<"$stale"

[[ "$apply" == true ]] || echo "dry run: pass --apply to cancel"
exit "$failed"
