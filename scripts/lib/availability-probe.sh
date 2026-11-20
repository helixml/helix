#!/bin/bash
# availability-probe.sh URL [CURL_ARGS...]
# Requests URL every PROBE_INTERVAL seconds (default 0.25) until SIGTERM or
# SIGINT, then prints one line:
#   requests=N failed=N max_gap_s=S
# A request fails on a connection error, a timeout (PROBE_TIMEOUT, default 5s)
# or a status other than 2xx; max_gap_s is the longest stretch with no
# successful response. deploy-prod.sh runs it against the public API for the
# whole control-plane deploy to measure the downtime a deploy causes.
set -uo pipefail
url=$1
shift
interval=${PROBE_INTERVAL:-0.25}
timeout=${PROBE_TIMEOUT:-5}

now() { date +%s.%N; }
requests=0 failed=0 max_gap=0
last_ok=$(now)
stop=false
trap 'stop=true' TERM INT

while [ "$stop" = false ]; do
  requests=$((requests + 1))
  if curl -fsS -o /dev/null --max-time "$timeout" "$@" "$url" 2> /dev/null; then
    last_ok=$(now)
  else
    failed=$((failed + 1))
    max_gap=$(echo "$(now) $last_ok $max_gap" | awk '{ g = $1 - $2; print (g > $3) ? g : $3 }')
  fi
  sleep "$interval" &
  wait $! 2> /dev/null
done
printf 'requests=%d failed=%d max_gap_s=%.1f\n' "$requests" "$failed" "$max_gap"
