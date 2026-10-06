#!/bin/bash
# Bash in the desktop's agent CPU tier (see /etc/cont-init.d/16-cpu-tiers.sh).
#
# Zed runs with SHELL set to this file. Zed starts ACP agents, MCP servers,
# terminals and tasks through $SHELL, so all of them, and everything they
# spawn, run below the display path. It is installed as ".../bash" because
# Zed and Claude Code pick shell syntax from the shell's file name.
#
# Unprivileged containers have no CPU tiers; this is then plain bash.

AGENT_TIER=/sys/fs/cgroup/desktop/agent/procs/cgroup.procs

if [ -e "${AGENT_TIER}" ] && ! echo 0 > "${AGENT_TIER}"; then
    echo "agent-tier: cannot join ${AGENT_TIER}" >&2
    exit 1
fi
exec /bin/bash "$@"
