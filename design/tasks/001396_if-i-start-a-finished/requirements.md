# Requirements: Show Desktop for Finished Tasks When Container Running

## Problem

When a user opens a finished/merged task, they always see "Task finished - This task has been merged to the default branch. The agent session has ended." even if the container is still running.

The user expects to see the desktop view if the container is still active. The "task finished" message should only appear when the container is actually stopped.

## User Stories

1. **As a user**, when I open a finished task that still has a running container, I want to see the desktop view so I can interact with or observe the session.

2. **As a user**, when I click the play button to restart a stopped container on a finished task, I want to see the desktop view so I can do follow-up work or deployment after the branch has merged.

3. **As a user**, when I open a finished task whose container has stopped (and I haven't restarted it), I want to see the "Task finished" message with an option to start the desktop.

## Acceptance Criteria

- [ ] Finished tasks (`status === "done"` or `merged_to_main`) show the desktop viewer when the sandbox is running or starting
- [ ] Finished tasks show the "Task finished" message only when the sandbox is stopped/absent
- [ ] User can click play button to restart a stopped container on a finished task, then see the desktop
- [ ] Same behavior applies to both desktop (big screen) and mobile views
- [ ] No change to archived/rejected task behavior (those always show the message)