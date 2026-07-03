# Requirements: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

## Background

When a spec task's remote desktop goes to sleep (paused) and the user returns
later, the frontend's reconnection retry counters have already been exhausted
from earlier failed attempts. When the user wakes the desktop back up — either by
**sending a message** or by clicking the **Start Desktop** button — the viewer
attempts one reconnect, immediately hits its already-maxed-out retry limit, and
shows a "giving up" error (e.g. *"Connection lost - max reconnection attempts
reached"* or *"Connection failed repeatedly. Please refresh the page to try
again."*) instead of reconnecting to the freshly-woken desktop.

The retry counters live at three independent layers, none of which is tied to the
session wake lifecycle:

| Counter | Location | Cap |
|---|---|---|
| `reconnectAttempts` | `websocket-stream.ts` | 10 |
| `retryAttemptRef` (AlreadyStreaming) | `DesktopStreamViewer.tsx` | uncapped |
| `manualReconnectAttemptsRef` | `DesktopStreamViewer.tsx` | 3 |

All three only reset on a *successful* connection (`connectionComplete`). A wake
never gets that far because the counters are already spent.

## User Stories

**US-1 — Wake by sending a message**
As a user returning to a slept spec task, when I send a message to wake the
desktop, I want the connection to retry from a clean state so it reconnects
successfully instead of failing immediately with an error.

**US-2 — Wake by clicking Start Desktop**
As a user returning to a slept spec task, when I click the Start Desktop button,
I want the reconnect retry count reset so the viewer connects to the newly-woken
desktop instead of showing a "max attempts reached" error.

**US-3 — Clear stale error on wake**
As a user, when I trigger a wake, any previous "giving up" error message should
be cleared so I see the normal connecting/starting state rather than a leftover
error.

## Acceptance Criteria

- [ ] When the session transitions to `starting` (wake triggered via message send
  **or** Start Desktop), all three retry counters are reset to zero:
  `reconnectAttempts` (transport), `retryAttemptRef`, `manualReconnectAttemptsRef`.
- [ ] Any existing connection error state (`error`, "Connection failed" status) is
  cleared when a wake is triggered.
- [ ] After a wake, the viewer performs a fresh reconnect attempt with the full
  retry budget available (up to the transport's 10 attempts), rather than
  immediately giving up.
- [ ] Sending a message to a slept task reconnects the desktop without a manual
  page refresh.
- [ ] Clicking Start Desktop on a slept task reconnects the desktop without a
  manual page refresh.
- [ ] Normal (non-wake) reconnect behaviour and the existing give-up-after-N logic
  are unchanged when there has been no wake trigger.

## Out of Scope

- Backend wake/resume logic (the backend already wakes the session on prompt or
  resume; this is purely a frontend retry-state fix).
- Changing the retry caps or backoff timing.
