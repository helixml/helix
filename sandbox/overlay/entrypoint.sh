#!/usr/bin/env bash
# Based on GOW base image entrypoint
# Runs cont-init.d scripts then launches startup-app.sh

set -e

# Source functions from GOW utils
source /opt/gow/bash-lib/utils.sh

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
exec gosu "${UNAME}" /opt/gow/startup-app.sh
