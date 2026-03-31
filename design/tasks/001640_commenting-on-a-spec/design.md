# Design: Auto-Start Desktop When Commenting on a Spec

## Architecture Overview

The change is **frontend-only**. The backend comment queue is already designed to be restart-resilient — comments persist in the DB and are processed when the agent connects. We just need the frontend to trigger a desktop start when a comment is submitted and the session is not running.

## Key Files

| File | Role |
|------|------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Main review UI; contains comment submission logic |
| `frontend/src/services/designReviewService.ts` | `useCreateComment` mutation hook |
| `frontend/src/services/sessionService.ts` | `useStopExternalAgent`, session hooks |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Session start/stop handlers (`handleStartPlanning`, `handleStartSession`) |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` | `useSandboxState` hook — detects desktop running/stopped/absent |

## Current Flow

1. User selects text → comment form appears
2. User types comment → clicks submit
3. `useCreateComment` mutation fires → comment stored in DB queue
4. Agent (if connected) picks up comment → responds

## Proposed Flow

1. User selects text → comment form appears
2. User types comment → clicks submit
3. **Check desktop state** via `sandboxState` from `useSandboxState` (or session config):
   - If `running`: no change (step 4 as normal)
   - If `absent` + `planning_session_id` exists → call `v1SessionsResumeCreate(sessionId)` in parallel with comment creation
   - If no `planning_session_id`: no auto-start (a session must already exist to have a plan)
4. `useCreateComment` mutation fires → comment stored in DB queue
5. UI shows "Starting desktop..." toast/indicator alongside comment submission feedback
6. Agent connects when desktop is ready → picks up queued comment

### Why Parallel (Not Sequential)?

- Comment creation is a fast DB write; desktop startup can take 30–60+ seconds
- The comment queue is restart-resilient — it will process once the agent connects
- Blocking submission until the desktop is fully started would be a poor UX
- Sequential startup → submit would not meaningfully improve correctness

## Desktop State Detection

The `DesignReviewContent.tsx` component receives the `specTask` prop which includes `planning_session_id`. The sandbox state can be obtained by:

1. **Option A (preferred):** Accept a `sandboxState` prop from the parent (`SpecTaskDetailContent.tsx` already computes this via `useSandboxState`). Pass it down to `DesignReviewContent` or expose a `onStartDesktop` callback.
2. **Option B:** Read the session config directly inside `DesignReviewContent` using `useGetSession(task.planning_session_id)`.

Option A is cleaner since the parent already owns this state. The parent passes an `onStartDesktop` callback that `DesignReviewContent` calls when a comment is submitted and the desktop is not running. On the standalone `SpecTaskReviewPage`, a similar pattern applies.

## Session Start Logic (to be placed in parent or shared hook)

```typescript
const handleEnsureDesktopRunning = async () => {
  if (sandboxState === "running" || sandboxState === "resumable") return; // already up

  if (!task.planning_session_id) return; // no session to resume

  // Resume stopped session
  await api.getApiClient().v1SessionsResumeCreate(task.planning_session_id);
  queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(task.planning_session_id) });
};
```

## Error Handling

- Desktop start errors are surfaced via a toast/snackbar (non-blocking)
- Comment is still submitted regardless of desktop start outcome
- Existing 2-minute agent timeout still applies as a backstop

## Notes for Implementors

- `useSandboxState` lives in `ExternalAgentDesktopViewer.tsx` — consider extracting it to a shared hook or `sessionService.ts` if it needs to be used in more places
- `SpecTaskReviewPage.tsx` is the standalone review page — it also needs access to the spec task and session state to enable this feature there
- The `DesignReviewContent` component is large (57.8KB); make surgical changes rather than refactoring the whole file
- The `handleStartSession` pattern in `SpecTaskDetailContent.tsx` (line 687) already has the resume logic — reuse or extract it
