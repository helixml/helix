#!/bin/bash
# Increase inotify limits for applications that watch many files (e.g., Zed IDE)
# This runs as root before the main startup script

source /opt/gow/bash-lib/utils.sh

# Default limits are often too low for IDEs that watch entire project directories
# Zed and other file-watching tools need higher limits

# max_user_watches: Maximum number of inotify watches per user (default: 8192)
# Zed/VS Code can easily exceed this with large projects
INOTIFY_WATCHES=524288

# max_user_instances: Maximum number of inotify instances per user (default: 128)
INOTIFY_INSTANCES=1024

# max_queued_events: Maximum number of events that can be queued (default: 16384)
INOTIFY_EVENTS=32768

gow_log "[inotify] Setting inotify limits..."

# These settings require privileged mode (which sandbox has for Docker-in-Docker)
if sysctl -w fs.inotify.max_user_watches=$INOTIFY_WATCHES 2>/dev/null; then
    gow_log "[inotify] max_user_watches=$INOTIFY_WATCHES"
else
    gow_log "[inotify] Warning: Failed to set max_user_watches (not privileged?)"
fi

if sysctl -w fs.inotify.max_user_instances=$INOTIFY_INSTANCES 2>/dev/null; then
    gow_log "[inotify] max_user_instances=$INOTIFY_INSTANCES"
else
    gow_log "[inotify] Warning: Failed to set max_user_instances"
fi

if sysctl -w fs.inotify.max_queued_events=$INOTIFY_EVENTS 2>/dev/null; then
    gow_log "[inotify] max_queued_events=$INOTIFY_EVENTS"
else
    gow_log "[inotify] Warning: Failed to set max_queued_events"
fi

gow_log "[inotify] Limits configured"
