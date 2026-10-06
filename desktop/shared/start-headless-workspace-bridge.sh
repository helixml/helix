#!/bin/bash

SERVICE_NAME="headless-workspace-bridge"

while true; do
    /usr/local/bin/desktop-bridge 2>&1 | sed -u "s/^/[${SERVICE_NAME}] /"
    exit_code=$?
    echo "[${SERVICE_NAME}] Process exited with code ${exit_code}, restarting in 2s..."
    sleep 2
done
