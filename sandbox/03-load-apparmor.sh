#!/bin/bash

load_helix_rootless_docker_profile() {
    local profile=helix-rootless-docker
    local securityfs=/sys/kernel/security

    if [ "$(cat /sys/module/apparmor/parameters/enabled 2>/dev/null || true)" != "Y" ]; then
        echo "AppArmor is disabled; skipping ${profile} profile load"
        return 0
    fi

    if ! mountpoint -q "$securityfs"; then
        mount -t securityfs securityfs "$securityfs"
    fi

    apparmor_parser -r "/etc/apparmor.d/${profile}"
    grep -q "^${profile} (enforce)$" "$securityfs/apparmor/profiles"
    mount --bind "$securityfs" "$securityfs"
    mount -o remount,bind,ro "$securityfs"
    echo "Loaded AppArmor profile ${profile}"
}

load_helix_rootless_docker_profile
unset -f load_helix_rootless_docker_profile
