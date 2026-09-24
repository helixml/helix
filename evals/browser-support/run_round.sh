#!/bin/bash
# Run one eval round across all variants.
#   ./run_round.sh skills     # skills pushed into each bot repo, baseline prompt
#   ./run_round.sh playbook   # no skills, playbook inlined in the prompt
set -euo pipefail
cd "$(dirname "$0")"
. run/env.sh
ROUND="$1"; shift
BOTS=$(jq -r '.[].id' variants.json)
case "$ROUND" in
  skills)   for b in $BOTS; do ./push_skills.sh "$b" browser-lookup support-systems; done; VARIANTS=variants.json ;;
  playbook) for b in $BOTS; do ./push_skills.sh "$b"; done; VARIANTS=variants_playbook.json ;;
  *) echo "unknown round $ROUND"; exit 1 ;;
esac
python3 run_eval.py --variants "$VARIANTS" --questions questions_all.json --tag "$ROUND" --parallel 3 --timeout 1200 "$@"
