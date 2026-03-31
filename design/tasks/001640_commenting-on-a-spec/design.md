# Design: Auto-Start Desktop When Sending a Message

## Architecture Overview

The change is **frontend-only**. The backend message/comment queue is already restart-resilient — messages persist in the DB and are processed when the agent connects. We need the frontend to trigger a desktop resume whenever any message is sent and the session is not running.

This applies to two entry points:
1. **Spec design review comments** (`DesignReviewContent.tsx`)
2. **Session chat messages** (the session chat input in the session view)

## Key Files

| File | Role |
|------|------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Spec review UI; comment submission logic |
| `frontend/src/services/designReviewService.ts` | `useCreateComment` mutation hook |
| `frontend/src/services/sessionService.ts` | Session hooks, `useStopExternalAgent` |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Session start/stop handlers; `useSandboxState` via `ExternalAgentDesktopViewer` |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` | `useSandboxState` hook — detects desktop running/stopped/absent |
| Session chat input component | Where free-form session messages are submitted |

## Proposed Flow (both entry points)

1. User types a message (comment or chat) → clicks submit
2. **Check desktop state** via `sandboxState`:
   - If `running` or `resumable`: no change, submit as normal
   - If `absent` + `session_id` exists → call `v1SessionsResumeCreate(sessionId)` **in parallel** with message submission
   - If no `session_id`: no auto-start (nothing to resume)
3. Message is submitted to the DB queue regardless
4. UI shows "Starting desktop..." indicator alongside submission feedback
5. Agent connects when desktop is ready → picks up queued message

### Why Parallel (Not Sequential)?

- Message submission is a fast DB write; desktop startup takes 30–60+ seconds
- The queue is restart-resilient — it processes once the agent connects
- Blocking submission until the desktop is up would be poor UX

## Desktop State Detection

Extract (or reuse) `useSandboxState` from `ExternalAgentDesktopViewer.tsx` into a shared hook or `sessionService.ts` so both entry points can access it without prop-drilling.

The preferred pattern: a shared `useEnsureDesktopRunning(sessionId)` hook that encapsulates the resume logic and can be called from any message-send handler.

## Shared Hook Logic

```typescript
const useEnsureDesktopRunning = (sessionId: string | undefined) => {
  return async () => {
    if (!sessionId) return; // no session to resume
    if (sandboxState === "running" || sandboxState === "resumable") return;

    await api.getApiClient().v1SessionsResumeCreate(sessionId);
    queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) });
  };
};
```

Call this in parallel with each message-send mutation.

## Error Handling

- Desktop start errors are surfaced via a toast/snackbar (non-blocking)
- Message is still submitted regardless of desktop start outcome
- Existing 2-minute agent timeout still applies as a backstop

## Notes for Implementors

- `useSandboxState` lives in `ExternalAgentDesktopViewer.tsx` — extract to a shared hook so both spec comments and session chat can use it
- `DesignReviewContent.tsx` is large (57.8KB); make surgical changes only
- `handleStartSession` in `SpecTaskDetailContent.tsx` (~line 687) already has resume logic — reuse or extract it rather than duplicating
- `SpecTaskReviewPage.tsx` (standalone review page) also needs the session state wired in
- The session chat input component needs the same `onSend` → `ensureDesktopRunning` pattern applied
