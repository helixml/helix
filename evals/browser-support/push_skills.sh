#!/bin/bash
# Push (or remove) the eval skills in a bot's project repo under .agents/skills/.
# Every harness reads project-local skills from there; the bot picks them up on
# its next restart (fresh container clone).
#
#   . run/env.sh
#   ./push_skills.sh <bot-id> browser-lookup support-systems   # install these
#   ./push_skills.sh <bot-id>                                  # remove all
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ORG="${EVAL_ORG:-unmanned-org}"
BOT="$1"; shift
BASE="$(cat "$HERE/run/url.txt")/b/$BOT"

PROJECT=$(curl -fsS -H "Authorization: Bearer $HELIX_API_KEY" "$HELIX_URL/api/v1/orgs/$ORG/bots/$BOT" | jq -r .project_id)
REPO=$(curl -fsS -H "Authorization: Bearer $HELIX_API_KEY" "$HELIX_URL/api/v1/projects/$PROJECT" | jq -r .default_repo_id)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
HOSTPORT="${HELIX_URL#http://}"
git clone -q "http://api:$HELIX_API_KEY@$HOSTPORT/git/$REPO" "$WORK/repo"
cd "$WORK/repo"
git rm -rq --ignore-unmatch .agents/skills
rm -rf .agents/skills
for s in "$@"; do
  mkdir -p ".agents/skills/$s"
  sed "s#{{BASE}}#$BASE#g" "$HERE/skills/$s/SKILL.md" > ".agents/skills/$s/SKILL.md"
done
git add -A
if git diff --cached --quiet; then
  echo "$BOT: skills unchanged ($*)"
  exit 0
fi
git -c user.name=eval -c user.email=eval@helix.local commit -qm "chore(skills): set eval skills: ${*:-none}"
git push -q origin HEAD:main
echo "$BOT: skills set to: ${*:-none}"
