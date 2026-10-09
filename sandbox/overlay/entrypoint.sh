#!/usr/bin/env bash
# Based on GOW base image entrypoint
# Runs cont-init.d scripts then launches startup-app.sh

set -e

# Source functions from GOW utils
source /opt/gow/bash-lib/utils.sh

# This script is PID 1, and the kernel drops any signal PID 1 has no handler
# for. Trap SIGTERM (docker stop) so the sandbox tears its desktops and nested
# dockerd down in order instead of being SIGKILLed wholesale after the grace
# period, which leaves the kernel minutes of zvol/overlay teardown and makes
# `docker stop` fail with "did not receive an exit event". See shutdown.sh.
STARTUP_PID=""
shutdown_sandbox() {
    trap '' TERM INT
    /usr/local/bin/helix-sandbox-shutdown || true
    if [ -n "$STARTUP_PID" ]; then
        kill -TERM "$STARTUP_PID" 2>/dev/null || true
    fi
    exit 0
}
trap shutdown_sandbox TERM INT
# Left behind by the previous shutdown when the same container is restarted;
# the dockerd/hydra supervisors refuse to run while it exists.
rm -f /run/helix-sandbox-stopping

# Execute all container init scripts. Only run this if the container is started as the root user
if [ "$(id -u)" = "0" ]; then
    for init_script in /etc/cont-init.d/*.sh ; do
        gow_log
        gow_log "[ ${init_script}: executing... ]"
        # shellcheck source=/dev/null
        source "${init_script}"
    done
fi

# If a command was passed, run that instead of the usual init startup script
# shellcheck disable=SC2198
if [ -n "${@:-}" ]; then
    /bin/bash -c "$@"
    exit $?
fi

# Launch startup script as 'UNAME' user (some services will run as root)
gow_log "Launching the container's startup script as user '${UNAME}'"
chmod +x /opt/gow/startup-app.sh
# Not exec'd: this shell must stay PID 1 to run the SIGTERM trap above. `wait`
# returns as soon as a trapped signal arrives.
gosu "${UNAME}" /opt/gow/startup-app.sh &
STARTUP_PID=$!
wait "$STARTUP_PID"
