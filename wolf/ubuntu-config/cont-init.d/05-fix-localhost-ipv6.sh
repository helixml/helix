#!/usr/bin/env bash
# Fix localhost resolution to prefer IPv6
# Node.js/Vite binds only to IPv6, so Firefox needs to try ::1 first

# Reorder /etc/hosts to put ::1 localhost before 127.0.0.1 localhost
if grep -q "^127.0.0.1.*localhost" /etc/hosts; then
    # Create temp file with IPv6 localhost first
    {
        grep "^::1" /etc/hosts
        grep -v "^::1" /etc/hosts
    } > /tmp/hosts.new
    cp /tmp/hosts.new /etc/hosts
    rm /tmp/hosts.new
    echo "Fixed /etc/hosts: IPv6 localhost now preferred"
fi
