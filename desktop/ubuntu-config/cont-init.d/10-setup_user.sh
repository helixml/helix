#!/usr/bin/env bash
# GOW init script: Configure default user

set -e

gow_log "**** Configure default user ****"

if [[ "${UNAME}" != "root" ]]; then
    PUID="${PUID:-1000}"
    PGID="${PGID:-1000}"
    UMASK="${UMASK:-000}"

    gow_log "Setting default user uid=${PUID}(${UNAME}) gid=${PGID}(${UNAME})"
    if id -u "${PUID}" &>/dev/null; then
        # Need to delete the old user $PUID then change $UNAME's UID
        # Default ubuntu image comes with user `ubuntu` and UID 1000
        oldname=$(id -nu "${PUID}")
        if [ "$oldname" != "${UNAME}" ]; then
            userdel -r "${oldname}" 2>/dev/null || true
        fi
    fi

    # Create group if it doesn't exist
    if ! getent group "${PGID}" &>/dev/null; then
        groupadd -f -g "${PGID}" ${UNAME}
    fi

    # Create user if it doesn't exist
    if ! id "${UNAME}" &>/dev/null; then
        useradd -m -d ${HOME} -u "${PUID}" -g "${PGID}" -s /bin/bash ${UNAME}
    fi

    gow_log "Setting umask to ${UMASK}"
    umask "${UMASK}"

    gow_log "Ensure ${UNAME} home directory is writable"
    chown "${PUID}:${PGID}" "${HOME}"

    gow_log "Ensure XDG_RUNTIME_DIR is writable"
    mkdir -p "${XDG_RUNTIME_DIR}"
    chown -R "${PUID}:${PGID}" "${XDG_RUNTIME_DIR}"
else
    gow_log "Container running as root. Nothing to do."
fi

gow_log "DONE"
