# Requirements: Restore Immediate Loading State When Waking a Paused Desktop by Chat

## Problem Statement

When a user types a message into the chat panel of a spec-task whose desktop is paused, the desktop is supposed to start booting immediately and the **Starting Desktop...** spinner should appear straight away. A recent fix (PR adding the polling fix in late April 2026, see Background) made this work via 3‑second polling. The user reports this has regressed: after sending a chat message to a paused desktop, the UI now sits on the **Desktop Paused / Start Desktop** screen for an uncomfortably long time before the spinner appears (or never visibly transitions for short boots).

## Background — the original fix

Two coordinated commits on 2026‑04‑25 made the spinner reliable:

- Frontend: `e43acefdb fix(ui): always poll session metadata so "Starting Desktop..." spinner shows`
  - Removed `refetchInterval: wsConnected ? false : 3000` gating in `EmbeddedSessionView.tsx`
  - Made `useGetSession` poll unconditionally at 3 s
- Backend: `3c931bfe5 fix(api): don't clobber DB-stored ExternalAgentStatus during boot window`
  - `getSession` handler used to overwrite `Metadata.ExternalAgentStatus` with a runtime check that returns `""` when no container exists yet
  - Now respects DB‑stored `"starting"` while `ContainerName == ""`

The premise of the frontend fix was that `useSandboxState` and `EmbeddedSessionView` shared a single React Query entry (same key), so making one consumer poll fixed both. That premise no longer holds — see Design.

## User Stories

### US1: Immediate feedback when waking a paused desktop by chat
**As a** user with a paused spec‑task desktop
**I want** the **Starting Desktop...** spinner to replace the **Desktop Paused** UI within ~1 second of sending a chat message
**So that** I know my message was received and the desktop is booting

### US2: No false "Paused" UI during boot
**As a** user who just woke a desktop
**I want** the UI to never flip back to **Desktop Paused** while the container is booting
**So that** I don't think my action failed and click "Start Desktop" again

## Acceptance Criteria

### AC1: Optimistic spinner on chat send
- [ ] When the user sends a chat message and the session's current `external_agent_status` is anything other than `"starting"` or `"running"`, the chat send path immediately sets the cached session metadata to `external_agent_status = "starting"`
- [ ] The **Starting Desktop...** spinner is visible within 500 ms of the user submitting a chat message to a paused session
- [ ] If the backend reports the session was already running (no boot needed), the optimistic change is harmless (next poll overrides)

### AC2: Polling continues to work as the source of truth
- [ ] Once the backend reports the real status (`"starting"` → `"running"` or back to `"absent"`), the cached value is updated by the next poll within 3 s
- [ ] The optimistic state never persists beyond the next successful poll

### AC3: WebSocket‑driven session_update no longer silently drops
- [ ] `setQueryData` and `getQueryData` calls in `streaming.tsx` for the session use the same React Query key shape as `useGetSession` (i.e. include the `'full'` / `'skip'` segment)
- [ ] Verified by adding a console log or test that confirms the cache write is observed by an active `useGetSession` consumer

### AC4: Manual end‑to‑end verification
- [ ] On a Helix‑in‑Helix instance, pause a spec‑task desktop, send a chat message, and observe the spinner appear immediately
- [ ] Repeat with the chat panel on mobile width — the chat panel itself indicates the queued prompt while the desktop view shows the spinner

## Out of Scope
- Changing the 3 s poll interval
- Reverting the queue/prompt-history flow back to direct `NewInference`
- Re‑introducing `wsConnected`-based polling gates
- Sandbox‑mode (non‑helix-session) desktop UI behaviour
