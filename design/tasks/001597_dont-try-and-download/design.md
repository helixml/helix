# Design: Don't Download Sessions for Stopped Desktops

## Architecture

### Current Flow

Every task card on the Kanban board renders `LiveAgentScreenshot` → `ExternalAgentDesktopViewer` → `useSandboxState`. That hook polls `GET /api/v1/sessions/{id}` every 3 seconds unconditionally (while the `enabled` flag is true).

The `enabled` flag is set in `TaskCard.tsx` via `showProgress && !!task.planning_session_id`. It does NOT check whether the desktop is stopped.

`LiveAgentScreenshot` is rendered when:
- `task.planning_session_id` is set
- `task.phase !== "completed"`
- `!task.merged_to_main`

So any in-flight task with a session ID gets a polling hook even if the desktop was stopped days ago.

### Key Files

| File | Role |
|------|------|
| `frontend/src/components/tasks/TaskCard.tsx:1044` | Renders `LiveAgentScreenshot` conditionally |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:30` | `useSandboxState` hook — polls every 3s |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:85` | `setInterval(fetchState, 3000)` — the polling loop |

### Fix

The spec task object already contains `agent_work_state` (`"idle" | "working" | "done"`) and `phase`. The Kanban board also fetches task data every 3.1 seconds. We can use this information to gate `useSandboxState` polling.

**Approach**: Pass the task's known status into `useSandboxState` (or `LiveAgentScreenshot`), and set `enabled = false` when the desktop is known to be stopped.

The `useSandboxState` hook already accepts an `enabled` parameter — we just need to compute it correctly in `TaskCard.tsx`.

A desktop is known to be stopped/absent when:
- `task.agent_work_state === "idle"` AND `task.phase` is not `"planning"` or `"implementation"` — the agent finished
- OR: `sandboxState === "absent"` was already observed (once we know it's absent, stop re-polling)

**Simplest correct fix**: Once `useSandboxState` observes `status === "stopped"` or `desired_state === "stopped"` from a session response, set a local `knownStopped` flag and stop the polling interval. The state transitions to `"absent"` and stays there until the user explicitly starts the desktop (which triggers a re-render/remount).

This means:
1. First poll fires to check current state (can't avoid without backend changes)
2. If stopped: polling stops, no further requests
3. If running: polling continues normally at 3s

**Alternative (if we want to avoid even the first poll)**: Use `task.agent_work_state` from the already-polled spec task data to skip the initial poll for tasks where the agent is idle and there's no sign of a running desktop. However, this requires coordinating the task's work state with the sandbox state, which is less reliable.

**Recommended**: The self-stopping approach (option 1) is the most correct and minimal change. Once we get a "stopped" response, we set a ref to prevent further polling.

## Decisions

- **No backend changes needed** — the session endpoint already returns the status correctly; we just need to stop polling it once we know the answer.
- **Don't remove the initial fetch** — we still need to check once to know whether it's running or stopped. The goal is to not keep re-checking after we know it's stopped.
- **Avoid prop drilling** — self-stopping within `useSandboxState` is cleaner than threading task state down through multiple components.

## Discovered Pattern

`useSandboxState` already has a `setSessionUnavailable`-style pattern in `ScreenshotViewer.tsx` for stopping on 503/404. We can apply the same pattern in `useSandboxState` for the stopped state.
