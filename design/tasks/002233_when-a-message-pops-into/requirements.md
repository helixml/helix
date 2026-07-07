# Requirements: Keep Spec-Task Chat Pinned to Bottom When the Message Queue Grows

## Background

In the Spec Task detail page (`SpecTaskDetailContent.tsx`), the chat panel is
`EmbeddedSessionView` (the scrollable transcript) stacked above the composer
`RobustPromptInput`. Sending a message adds it to a **local message queue**,
which syncs to the backend queue and is rendered as an expandable queue panel
inside the composer.

When a message enters that queue, the composer grows (the queue `Collapse`
expands, and the textarea auto-resizes for larger prompts). Because the composer
is a `flexShrink: 0` sibling *below* the transcript, its growth shrinks the
transcript's scroll **viewport** — pushing the tail of the conversation below
the fold. The transcript does not re-pin to the bottom, so the newest messages
become hidden, and with larger prompts the disruption is large enough that the
auto-scroll ("sticky scroll") lock inadvertently disengages.

## User Stories

**US-1 — Tail stays visible after sending.**
As a user of the Spec Task chat panel, when I send/queue a message, I want the
transcript to remain scrolled to the bottom so the newest content (my queued
message and the agent's reply) is never occluded by the grown composer.

**US-2 — Sending never changes the sticky-scroll lock.**
As a user, when I simply send a message, I want the auto-scroll on/off state to
stay exactly as it was — sending should never turn sticky scroll off (or on).

**US-3 — Larger prompts behave the same as small ones.**
As a user sending a large multi-line prompt, I want the same pinned-to-bottom
behaviour as a short prompt; a bigger composer must not disengage auto-scroll.

## Acceptance Criteria

**AC-1 (US-1):** With auto-scroll ON, queuing a message keeps the transcript
pinned to the bottom. After the composer grows, the last interaction (and any
live/waiting bubble) remains fully visible — not cut off below the viewport.

**AC-2 (US-1):** When the composer grows purely from a viewport change (queue
panel expanding, textarea auto-resizing) with no new transcript content, the
transcript still re-pins to the bottom. The existing short-circuit that skips a
scroll when transcript `scrollHeight` is unchanged must not prevent this.

**AC-3 (US-2):** Queuing/sending a message never calls the auto-scroll unlock and
never toggles the `helix.autoScroll` preference. If auto-scroll was ON before
sending, it is ON after; if it was OFF, it stays OFF.

**AC-4 (US-2, OFF case):** If auto-scroll is OFF, sending a message must not
force-scroll or re-pin the transcript (the user chose to stay put); the existing
"Jump to latest" affordance continues to govern catching up.

**AC-5 (US-3):** Sending a large prompt (composer grows by a large amount)
produces the same outcome as AC-1/AC-3 — pinned to bottom, lock unchanged — with
no user reflexive scroll needed to see the tail.

**AC-6:** Behaviour is identical on both composer mount sites in
`SpecTaskDetailContent.tsx` (the desktop split-panel layout and the mobile chat
view), since both wire the same `onHeightChange` handler and `sessionViewRef`.

**AC-7 (no regression):** The 3s poll / WebSocket keepalive path still does no
redundant scroll work, the wheel/touch scroll-up unlock still works for genuine
user scroll-ups, and the initial-mount / session-change force-scroll and
jump-to-latest pill are unaffected.

## Out of Scope

- The general Session page chat (`Session.tsx`) — this task targets the Spec Task
  detail chat panel specifically. (Apply there too only if the fix lands in
  shared `EmbeddedSessionView` behaviour, which it does; no extra work required.)
- Redesigning the message queue UI or the auto-scroll toggle/pill.
- Backend queue sync behaviour.

## Open Questions

1. **Exact disengage mechanism.** Static analysis indicates auto-scroll only
   flips OFF via the toggle button or the wheel/touch upward unlock
   (`USER_SCROLL_UNLOCK_PX = 100`). The most likely path for "sending disengages
   it" is downstream of the occlusion: the view jumps, the user reflexively
   wheels up to reorient, and crosses the 100px threshold. Fixing the occlusion
   (keep pinned) should remove that trigger. **Did you observe the lock flipping
   even when you did *not* touch the wheel/trackpad after pressing send?** If so
   there is a second, automatic path we must reproduce and guard directly.
2. **OFF-state expectation.** AC-4 assumes that when auto-scroll is already OFF,
   sending should *not* yank you to the bottom (respecting your paused state).
   Is that correct, or do you expect your own just-sent message to always pull
   you down even when sticky scroll is off?
