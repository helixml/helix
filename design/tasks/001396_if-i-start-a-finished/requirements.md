# Requirements: Show Desktop for Finished Tasks When Container Running

## Problem

When a user opens a finished/merged task, they always see "Task finished - This task has been merged to the default branch. The agent session has ended." even if the container is still running. There's also no way to restart a stopped container on a finished task.

The user expects to see the desktop view if the container is still active, and have the option to restart it if stopped.

## User Stories

1. **As a user**, when I open a finished task that still has a running container, I want to see the desktop view so I can interact with or observe the session.

2. **As a user**, when I open a finished task whose container has stopped, I want to see the "Task finished" message with a play button so I can restart the desktop for follow-up work or deployment.

## Acceptance Criteria

- [ ] Finished tasks (`status === "done"` or `merged_to_main`) show the desktop viewer when the sandbox is running or starting
- [ ] Finished tasks show the "Task finished" message with a play button when the sandbox is stopped
- [ ] Clicking the play button starts the container and shows the desktop viewer
- [ ] Same behavior applies to both desktop (big screen) and mobile views
- [ ] No change to archived/rejected task behavior (those always show the message, no restart option)