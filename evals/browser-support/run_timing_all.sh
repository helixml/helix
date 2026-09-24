#!/bin/bash
# Launch + turn timing: spec tasks vs org bots, desktop vs headless.
set -uo pipefail
cd "$(dirname "$0")"
. run/env.sh
Q="q1_crm_owner q3_billing_balance q5_helpdesk_history q6_cross_crm_billing h1_aggregate_platinum h3_not_found"
python3 run_spectasks.py --qids $Q --runtime ubuntu-desktop --tag st-desktop
python3 run_spectasks.py --qids $Q --runtime headless-ubuntu --tag st-headless
python3 run_bots_timing.py --qids $Q --sandbox-runtime ubuntu-desktop --tag bot-desktop
python3 run_bots_timing.py --qids $Q --sandbox-runtime headless-ubuntu --tag bot-headless
. run/env.sh; for b in sup-opencode-glm sup-dsh-glm sup-dsh-qwen; do curl -s -o /dev/null -X PATCH -H "Authorization: Bearer $HELIX_API_KEY" -H 'Content-Type: application/json' -d '{"sandbox_runtime":"ubuntu-desktop"}' "$HELIX_URL/api/v1/orgs/unmanned-org/bots/$b"; done
echo ALL-DONE
