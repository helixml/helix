#!/bin/bash

load_helix_rootless_docker_profile() {
    local profile=helix-rootless-docker
    local securityfs=/sys/kernel/security

    if [ "$(cat /sys/module/apparmor/parameters/enabled 2>/dev/null || true)" != "Y" ]; then
        echo "AppArmor is disabled; skipping ${profile} profile load"
        return 0
    fi

    if ! mountpoint -q "$securityfs"; then
        mount -t securityfs securityfs "$securityfs" || return 1
    fi

    apparmor_parser -r "/etc/apparmor.d/${profile}" || return 1
    if ! grep -q "^${profile} (enforce)$" "$securityfs/apparmor/profiles"; then
        echo "AppArmor profile ${profile} is not enforced" >&2
        return 1
    fi
    mount --bind "$securityfs" "$securityfs" || return 1
    mount -o remount,bind,ro "$securityfs" || return 1
    echo "Loaded AppArmor profile ${profile}"
}

load_helix_rootless_docker_profile || exit 1
unset -f load_helix_rootless_docker_profile
