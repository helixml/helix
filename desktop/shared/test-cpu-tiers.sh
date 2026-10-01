#!/bin/bash
#
# Test the desktop CPU tiers (16-cpu-tiers.sh + agent-tier-bash.sh) in
# throwaway containers configured like Hydra's desktops: privileged with a
# private cgroup namespace and a CPU quota, plus one unprivileged container.
#
# Needs Docker with privileged containers.
#
# Usage: ./test-cpu-tiers.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE="${IMAGE:-ubuntu:24.04}"

run() {
    docker run --rm --entrypoint bash -v "${SCRIPT_DIR}:/s:ro" "$@"
}

check() {
    local name="$1" expected="$2" actual="$3"
    if [ "${actual}" != "${expected}" ]; then
        echo "FAIL: ${name}: expected '${expected}', got '${actual}'"
        exit 1
    fi
    echo "ok: ${name}"
}

# Prints: agent cpu.max | PID 1's cgroup | the wrapper's cgroup | escape result
probe='
set -e
id retro >/dev/null 2>&1 || useradd -m retro
source /s/16-cpu-tiers.sh >/dev/null
echo "$(cat /sys/fs/cgroup/desktop/agent/cpu.idle) $(cat /sys/fs/cgroup/desktop/agent/cpu.max)"
cut -d: -f3 /proc/1/cgroup
su retro -c "bash /s/agent-tier-bash.sh -c \"cut -d: -f3 /proc/self/cgroup\""
su retro -c "bash /s/agent-tier-bash.sh -c \"echo \\\$\\\$ > /sys/fs/cgroup/desktop/display/cgroup.procs\"" 2>/dev/null && echo escaped || echo denied
'

out=$(run --privileged --cgroupns=private --cpus 12 "${IMAGE}" -c "${probe}")
check "agent tier is idle and capped two cores below a 12-CPU quota" "1 1000000 100000" "$(sed -n 1p <<<"${out}")"
check "PID 1 starts in the display tier" "/desktop/display" "$(sed -n 2p <<<"${out}")"
check "the agent-tier bash joins the agent tier" "/desktop/agent/procs" "$(sed -n 3p <<<"${out}")"
check "agent work cannot move itself back to the display tier" "denied" "$(sed -n 4p <<<"${out}")"

out=$(run --privileged --cgroupns=private --cpus 4 "${IMAGE}" -c "${probe}")
check "small sandbox keeps a quarter of the quota for the display tier" "1 300000 100000" "$(sed -n 1p <<<"${out}")"

out=$(run --privileged --cgroupns=private "${IMAGE}" -c "${probe}")
check "no container quota leaves the agent tier uncapped" "1 max 100000" "$(sed -n 1p <<<"${out}")"

out=$(run --cgroupns=private "${IMAGE}" -c 'set -e
f() { source /s/16-cpu-tiers.sh; }; f
bash /s/agent-tier-bash.sh -c "cut -d: -f3 /proc/self/cgroup"')
check "unprivileged containers skip the tiers and the wrapper is plain bash" "/" "$(tail -1 <<<"${out}")"

echo "PASS"
