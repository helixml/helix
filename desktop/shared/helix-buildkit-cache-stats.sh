#!/bin/bash
# Print a one-line JSON summary of the local dockerd's BuildKit cache: record
# count, total bytes, and the RUN --mount=type=cache (exec.cachemount) records
# with their sizes. Golden builds record it before the snapshot and sessions
# log it after boot, so cache lost across golden promotion shows up in logs.
#
# Uses the Engine API rather than `docker buildx du`, whose sizes are
# human-formatted strings.
set -euo pipefail

curl -sf --unix-socket /var/run/docker.sock 'http://localhost/system/df?type=build-cache' | jq -c '
    (.BuildCache // []) as $records
    | ($records | map(select(.Type == "exec.cachemount"))) as $mounts
    | {
        records: ($records | length),
        total_bytes: ($records | map(.Size) | add // 0),
        cache_mount_count: ($mounts | length),
        cache_mount_bytes: ($mounts | map(.Size) | add // 0),
        cache_mounts: ($mounts | sort_by(-.Size) | .[:10]
            | map({mount: ((.Description | capture("^cached mount (?<m>\\S+)") | .m) // .Description), bytes: .Size}))
    }'
