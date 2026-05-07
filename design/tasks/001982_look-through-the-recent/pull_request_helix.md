# fix(ui): immediate spinner when waking a paused desktop by chat

## Summary

When a user sends a chat message to a paused spec-task desktop, the **Starting Desktop...** spinner should appear immediately to confirm the wake. Today it can take 2.5–5 seconds — long enough to feel broken. This restores the immediate feedback by adding an optimistic React Query cache update on chat send, and fixes a related WebSocket cache-write bug.

## Why it regressed

The April 2026 fix in [PR #2294](https://github.com/helixml/helix/pull/2294) (commit `e43acefdb`) made `useGetSession` poll unconditionally at 3 s — the spinner was meant to appear within one polling cycle. Three things since have stretched that out:

1. `useGetSession`'s query key now suffixes `'full'` / `'skip'` (added in `07e3a313b` for `skipInteractions` support), so `useSandboxState` and `EmbeddedSessionView` no longer share a single React Query entry.
2. `streaming.tsx`'s `session_update` handler writes to the **bare** `["session", id]` key. `setQueryData` requires an exact key match (unlike `invalidateQueries`, which prefix-matches), so those WS-driven writes silently miss every active query.
3. The chat-send path in `RobustPromptInput` is purely backend-mediated (POST → backend goroutine → `StartDesktop` → DB write) before any polled status changes — adds another 0.5–2 s on top of the polling cycle.

## Changes

- **New helper** `frontend/src/utils/optimisticSessionStarting.ts` — flips the cached `session.config.external_agent_status` to `"starting"` in both the `'full'` and `'skip'` query slots, no-ops when the cache already shows `"starting"` / `"running"`, and kicks the next poll via a prefix `invalidateQueries`.
- **`RobustPromptInput`** — adds an optional `onWillSend` callback prop, invoked synchronously inside `handleSend` immediately after the user submits. Wrapped in try/catch so an optimistic-UI bug can never block a send.
- **`SpecTaskDetailContent`** — defines `handleWillSend` and passes `onWillSend={handleWillSend}` to both `RobustPromptInput` mounts (split-view chat panel and mobile chat view).
- **`ExternalAgentDesktopViewer`** — same wiring on its inline prompt input, alongside the existing `invalidateQueries` safety net.
- **`streaming.tsx`** — fixes `session_update` to use the suffixed query keys. Writes both variants; strips `interactions` from the `'skip'` write so `useListInteractions` remains the source of truth there. Reads from `'full'` for the stale-update interaction-count check.

Polling at 3 s is still the source of truth — the optimistic flip is harmlessly overwritten by the next backend poll. If the session is already running when the user sends, the helper no-ops and no spinner flashes.

No backend changes.

## Test plan

- [ ] **Manual** — reviewer runs a Helix stack with this branch deployed:
  - [ ] Pause a spec-task desktop. Send a chat message. Spinner appears in ≤ 500 ms.
  - [ ] Send a chat to a *running* desktop. No flicker, no false spinner, message appears as usual.
  - [ ] Repeat with the chat panel collapsed (icon-only mode) — no React errors.
- [x] `cd frontend && yarn build` clean.

⚠️ **Not yet manually tested in this branch's CI** — inner-Helix sandbox failed to build (`/zed-build/app-icon.png: not found`) so no live stack was available. The change is small (5 files, ~75 lines net) and falls back to existing polling behaviour on any error path.

## Related

- Original fix this restores: [PR #2294](https://github.com/helixml/helix/pull/2294) — `e43acefdb fix(ui): always poll session metadata so "Starting Desktop..." spinner shows`
- Backend coordination: `3c931bfe5 fix(api): don't clobber DB-stored ExternalAgentStatus during boot window`
- Spec: [helix-specs/design/tasks/001982_look-through-the-recent](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001982_look-through-the-recent)
