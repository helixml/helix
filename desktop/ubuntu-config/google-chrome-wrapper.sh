#!/bin/bash
# Chrome wrapper for container environments
# Adds flags to speed up startup and reduce GPU memory usage

exec /usr/bin/google-chrome-stable \
    --no-first-run \
    --no-default-browser-check \
    --disable-background-networking \
    --disable-client-side-phishing-detection \
    --disable-component-update \
    --disable-default-apps \
    --disable-hang-monitor \
    --disable-popup-blocking \
    --disable-prompt-on-repost \
    --disable-sync \
    --disable-translate \
    --metrics-recording-only \
    --password-store=basic \
    --use-mock-keychain \
    --disable-features=TranslateUI \
    --disable-ipc-flooding-protection \
    --process-per-site \
    --renderer-process-limit=1 \
    --disable-accelerated-2d-canvas \
    "$@"
