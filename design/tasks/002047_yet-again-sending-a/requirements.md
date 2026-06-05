# Requirements: Show Booting State Immediately When Chatting to an Idle Spec-Task Session

## Problem

On the spec task detail page, sending a chat message to an idle / paused
spec-task session is supposed to immediately replace the "Desktop Paused
— Start Desktop" UI with a "Starting Desktop..." spinner, exactly as
clicking the Start Desktop button does. Today the spinner either:

- never appears (the "paused" UI stays put until the desktop is fully
  back online, which can take 10–30s), or
- flashes for ~100–500ms and then snaps back to "paused", before
  eventually flipping to "starting" once a poll catches up.

The user reports this has regressed (or never actually worked) despite
multiple attempts to fix it. They have asked for a review of the prior
attempts and a root-cause explanation.

## Prior attempts

Three commits/specs have touched this exact behaviour. They are all
still in the tree:

1. **`e43acefdb` (2026-04-25) — "always poll session metadata so
   Starting Desktop spinner shows".** Made the 3s `useGetSession` poll
   unconditional inside `useSandboxState`
   (`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:35-86`).
   Bought us "spinner eventually appears within 3s" — did not make the
   transition synchronous with the send action.

2. **`bea5d6ae1` (2026-05-07) — "immediate spinner when waking a
   paused desktop by chat".** Added
   `frontend/src/utils/optimisticSessionStarting.ts` and an
   `onWillSend` callback on `RobustPromptInput`
   (`frontend/src/components/common/RobustPromptInput.tsx:694-700`).
   Both `SpecTaskDetailContent.handleWillSend`
   (`frontend/src/components/tasks/SpecTaskDetailContent.tsx:576-579`)
   and `ExternalAgentDesktopViewer.handleWillSend`
   (`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:312-314`)
   call `optimisticallyMarkSessionStarting(queryClient, sessionId)`
   the moment the user hits send. The helper writes
   `external_agent_status="starting"` into both `'full'` and `'skip'`
   variants of the `["session", id]` React Query cache key, then
   **calls `queryClient.invalidateQueries({ queryKey:
   GET_SESSION_QUERY_KEY(sessionId) })`** to "shorten the time until
   it's confirmed"
   (`frontend/src/utils/optimisticSessionStarting.ts:50-53`).

3. **Spec `001995_httpsgithubcomhelixmlheli` — backend
   `autoStartDevContainerForSession` generalised for exploratory
   `zed_external` sessions and a defensive sweep in
   `auto_wake_stuck_interactions.go`.** Made sure the backend
   *eventually* wakes the desktop on `/messages` for both spec-task
   and exploratory sessions. Pure backend; did not change anything the
   frontend renders during the wake.

## Root cause

The optimistic helper from attempt #2 is undone by its own
`invalidateQueries` call. The send path is fully asynchronous:

```
user clicks send
  └─ RobustPromptInput.handleSend()
       ├─ onWillSend()  ──▶ optimisticallyMarkSessionStarting()
       │                        ├─ setQueryData (full) → "starting"
       │                        ├─ setQueryData (skip) → "starting"
       │                        └─ invalidateQueries(["session", id])  ◀── triggers immediate refetch
       └─ saveToHistory()
            └─ syncEntryImmediately()
                 └─ POST /api/v1/prompt-history/sync  ──▶ returns 200 immediately
                       └─ go processPendingPromptsForIdleSessions(...)         ◀── goroutine
                            └─ processPromptQueue
                                 └─ sendChatMessageToExternalAgent
                                      └─ sendCommandToExternalAgent
                                           └─ go autoStartDevContainerForSession
                                                └─ StartDesktop
                                                     └─ writes external_agent_status="starting" to DB
```

The `invalidateQueries` call kicks off a `GET /api/v1/sessions/{id}`
refetch in the same tick that the user clicks send. The backend has
*not* yet updated the DB — the prompt-history POST has returned, but
the goroutine chain that eventually calls `StartDesktop` and writes
`external_agent_status="starting"` typically hasn't reached the DB
write yet. So the refetch resolves with the still-`stopped` /
still-`absent` row and **overwrites the optimistic "starting"** that
was just placed in the cache.

`useSandboxState` re-derives `isStarting=false`, the spinner
disappears, and the "Desktop Paused" UI snaps back. ~3s later the
regular poll finally sees `external_agent_status="starting"` from the
goroutine's DB write and the spinner returns. The user perceives this
as "the spinner never showed" or "it flickered and went away".

This race is independent of the backend cold-start fix from spec
#001995 — that fix makes the desktop *eventually* boot, but does not
change the race window. The race also pre-dates `bea5d6ae1`: before
the optimistic helper existed there was no immediate spinner at all,
just the 3s polling delay from `e43acefdb`. So the user is correct
that the issue has been around in two distinct forms: "no spinner
until next poll" (pre-`bea5d6ae1`) and "spinner flickers off then
back" (post-`bea5d6ae1`).

## Why the existing optimistic write is dropped (mechanism, in detail)

- `useGetSession` keys its React Query entry with
  `[...GET_SESSION_QUERY_KEY(id), variant]` where variant is `'full'`
  or `'skip'` (`frontend/src/services/sessionService.ts:48-67`).
- `optimisticallyMarkSessionStarting` writes to both variants via
  `setQueryData` (exact-match), which lands correctly.
- `invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(id) })` does a
  **prefix match**, so it marks both variants stale.
- React Query then refires both queries against the API. The first
  response wins and replaces the cache entry wholesale — the
  optimistic patch is gone.
- The backend has no synchronous "I have received a wake intent"
  surface. The send POST is `prompt-history/sync`, which only stores
  the prompt and fires the wake goroutine.

## User stories

### As a user on the spec task detail page
- When I send a chat message to a paused desktop, I want the "Starting
  Desktop..." spinner to appear immediately (within one frame of
  clicking send) and *stay* until either the desktop is running or the
  start has actually failed.
- I never want to see the spinner flicker off and back on again — that
  reads as "my send did nothing" and I will click send a second time.
- The behaviour must match what I see when I click the explicit
  "Start Desktop" button, which already works correctly.

### As an operator / debugger
- I want logs that make it obvious *which* code path declared the
  session "starting": the synchronous send-time mark, the
  `autoStartDevContainerForSession` goroutine, or a real container
  boot.
- I want this to be testable without standing up a full Zed dev
  container — a unit test on the React Query cache and a unit test on
  the synchronous backend mark should cover the race.

## Acceptance criteria

1. **No spinner flicker.** Sending a chat message to a paused spec-task
   session results in the "Starting Desktop..." spinner appearing
   within one frame and remaining visible continuously until the
   desktop is `running` (success) or `absent` with an error message
   (failure). It must never transition `paused → starting → paused →
   starting → running`.
2. **Optimistic state survives the next poll.** Whatever fix is
   chosen, the React Query refetch that fires immediately after
   `onWillSend` must return a session whose
   `external_agent_status` is `starting` (or equivalent), not
   `stopped` / `absent`. The optimistic write must agree with the
   backend within the refetch window.
3. **Existing Start Desktop button behaviour is preserved.** Clicking
   the explicit Start Desktop button still shows the spinner
   immediately and continues to work end-to-end.
4. **Cold-start path from spec #001995 still works.** Sending a
   message to an exploratory `zed_external` session with no live WS
   still wakes the desktop (i.e. we do not break the fix from PR
   fixing issue #2397).
5. **No regression on already-running sessions.** Sending a chat
   message to a session whose desktop is already `running` must not
   flip the cached `external_agent_status` away from `running` even
   transiently. The helper's existing "no-op if running" guard must
   continue to hold.
6. **No regression on non-spec-task chat surfaces.** The chat panel
   inside the `ExternalAgentDesktopViewer` (floating window) and any
   other consumer of `RobustPromptInput.onWillSend` must continue to
   show the spinner correctly (or be a no-op if there is no spinner
   to show).
7. **Tests.** A frontend test exercises the race directly: after
   calling `optimisticallyMarkSessionStarting`, a simulated refetch
   returning `external_agent_status="stopped"` must NOT cause the
   derived `isStarting` to flip to `false`. A backend test covers
   any new synchronous "mark starting" surface introduced on the API
   side, including the no-op-on-running case.
8. **Manual E2E in inner Helix.** Reproducer:
   - Register `test@helix.ml` / `helixtest`, create a project, create
     a spec task, let it run to first stop / "Desktop Paused".
   - From the spec task detail page chat box, send any message.
   - Observe: "Starting Desktop..." spinner appears within one frame,
     stays continuously, eventually transitions to the running
     desktop. No flicker.

## Out of scope

- Changing the prompt-history sync contract or splitting
  `prompt-history/sync` into multiple endpoints.
- Rewiring the goroutine chain
  (`processPendingPromptsForIdleSessions` →
  `sendChatMessageToExternalAgent` → ...) to be synchronous. That
  would alter response-time semantics for many other call sites.
- The Kanban card spinner. The Kanban surface uses
  `initialSandboxState` from the task list and a separate cadence
  (`SpecTaskDetailContent` is the focus here).
- The "Zed accepts a keystroke but never relays it" gap noted in
  `auto_wake_stuck_interactions.go:96-104`.
- Frontend WebSocket session_update handling
  (`streaming.tsx:307-327`) — already preserves config, not part of
  the race.
