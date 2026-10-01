#!/bin/bash
# CPU tiers: the display path keeps the CPU while the agent builds.
#
# Everything in a desktop used to run in one cgroup at nice 0, so the
# compositor competed equally with dozens of compile threads and the stream
# stuttered. Agent work now runs in an idle-class cgroup (cpu.idle=1): it gets
# only the CPU the display path leaves, and a waking compositor preempts it.
# See design/2026-10-01-desktop-cpu-priority.md.
#
#   /desktop                 retro may move processes within this subtree
#   ├── display              every process starts here (PID 1 is moved here)
#   └── agent   cpu.idle=1, cpu.max = container quota minus two cores
#       ├── procs            Zed's shell children (agents, MCP servers,
#       │                    terminals), the setup terminal, inner dockerd
#       └── docker           inner dockerd's cgroup-parent: containers and
#                            BuildKit steps
#
# Processes join the agent tier through /usr/local/libexec/helix/agent-tier/bash.
# retro may write agent/procs but not display, so agent work cannot move itself
# back into the display tier.
#
# The entrypoint sources this file, so skipping must `return`, not `exit`.

CGROUP_ROOT=/sys/fs/cgroup
CPU_TIERS=${CGROUP_ROOT}/desktop

# Docker mounts cgroup2 read-only in unprivileged containers (rootless headless
# agents, bot instances without a container engine). They run no inner engine.
if [ ! -w "${CGROUP_ROOT}/cgroup.subtree_control" ]; then
    echo "[cpu-tiers] cgroup2 is read-only in this unprivileged container; no CPU tiers"
    return 0
fi

mkdir -p "${CPU_TIERS}/display" "${CPU_TIERS}/agent/procs" "${CPU_TIERS}/agent/docker"

# cgroup v2 forbids processes in a cgroup whose subtree has controllers
# enabled, so the root's processes move into a leaf first. mapfile reads
# without forking; a process may still exit before it is moved.
mapfile -t pids < "${CGROUP_ROOT}/cgroup.procs"
for pid in "${pids[@]}"; do
    echo "${pid}" > "${CPU_TIERS}/display/cgroup.procs" 2>/dev/null || [ ! -d "/proc/${pid}" ]
done

# Inner containers (including Kind/systemd ones) need every controller
# delegated down to agent/docker.
controllers=""
for controller in $(cat "${CGROUP_ROOT}/cgroup.controllers"); do
    controllers="${controllers} +${controller}"
done
for cgroup in "${CGROUP_ROOT}" "${CPU_TIERS}" "${CPU_TIERS}/agent"; do
    echo "${controllers}" > "${cgroup}/cgroup.subtree_control"
done

echo 1 > "${CPU_TIERS}/agent/cpu.idle"

# cpu.idle orders work inside the container's CPU quota but still lets agent
# work spend all of it, and then CFS throttles the whole container, compositor
# included, until the next period. Keep two cores of the quota (a quarter of it
# on small sandboxes) out of the agent tier's reach: the display path uses
# 1-2.5 cores while streaming.
read -r quota period < "${CGROUP_ROOT}/cpu.max"
agent_quota=max
if [ "${quota}" != "max" ]; then
    reserve=$(( 2 * period < quota / 4 ? 2 * period : quota / 4 ))
    agent_quota=$(( quota - reserve ))
fi
echo "${agent_quota} ${period}" > "${CPU_TIERS}/agent/cpu.max"

# Moving a process needs write access to the destination and to the common
# ancestor's cgroup.procs.
chown retro:retro "${CPU_TIERS}/cgroup.procs" "${CPU_TIERS}/agent/procs/cgroup.procs"

echo "[cpu-tiers] display: ${CPU_TIERS}/display, agent (cpu.idle=1, cpu.max=${agent_quota} ${period}): ${CPU_TIERS}/agent"
