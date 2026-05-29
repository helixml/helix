# Requirements: Re-enable Auto-Scroll When User Scrolls Back to Bottom

## Background

The chat viewport in `EmbeddedSessionView` (`frontend/src/components/session/EmbeddedSessionView.tsx`)
already implements the auto-scroll model documented in the comment block
at lines ~66-86: a single global preference (`helix.autoScroll`,
default ON) controls whether new content scrolls to the bottom. The
preference is flipped OFF in two ways:

1. The toggle button (bottom-right).
2. The user wheels / finger-drags upward by ≥ `USER_SCROLL_UNLOCK_PX`
   (100px) within a single gesture.

And it is flipped back ON in two ways:

1. The toggle button.
2. Clicking the "Jump to latest" pill that appears bottom-center.

**What's missing:** if the user manually scrolls back to the bottom of
the conversation — via wheel, finger drag, scrollbar drag, Page Down /
End keys, or iOS momentum — auto-scroll **stays OFF**. The user has
clearly indicated they want to be at the bottom (and presumably want
new content as it arrives), but new agent output continues to land
below the viewport with only the pill as the recovery affordance.

The user's expectation, stated verbatim: *"Scrolling back to the bottom
should explicitly re-enable the auto scroll."*

## User Stories

### US-1: Wheel/touch back to bottom re-enables auto-scroll
**As a** user reading older messages with auto-scroll paused,
**I want** scrolling all the way back down to the latest message to
automatically resume auto-scroll,
**so that** I don't have to also click the pill or the toggle button
just to confirm "yes, follow new content again".

### US-2: Scrollbar drag / keyboard nav behaves the same way
**As a** user using the scrollbar handle, Page Down, or End to return
to the bottom,
**I want** the same re-enable behaviour,
**so that** auto-scroll resumption isn't gated on a specific input
device.

### US-3: No false-positive re-enable from content reflow
**As a** user with auto-scroll explicitly paused, sitting somewhere
mid-conversation,
**I do NOT want** auto-scroll to silently re-enable just because
content above or below the viewport collapsed, shrank, or otherwise
re-laid out underneath me.

## Acceptance Criteria

### AC-1: Manual scroll-down reaching the bottom re-enables auto-scroll
- **Given** auto-scroll is currently OFF (because the user scrolled up
  past `USER_SCROLL_UNLOCK_PX`, or toggled it off, or because
  `helix.autoScroll = "false"` is in localStorage from a prior session)
- **And** the user scrolls the chat viewport downward via any input
  (wheel, touch-drag, scrollbar drag, Page Down, End, iOS momentum
  flick)
- **When** the resulting scroll position lands the viewport within
  `AUTO_SCROLL_NEAR_BOTTOM_PX` (80px) of the bottom
- **Then** the `helix.autoScroll` preference flips back to **ON**
- **And** the toggle button visual state updates to ON (filled, primary)
- **And** the "Jump to latest" pill dismisses (it was already dismissing
  at this threshold; this behaviour is preserved)
- **And** subsequent new content auto-scrolls into view

### AC-2: Persisted preference is updated, not just in-memory state
- **Given** AC-1 has just triggered
- **When** the user reloads the page
- **Then** `localStorage.helix.autoScroll === "true"` (persisted via
  `useAutoScrollPreference`)
- **And** other open Helix tabs receive the change via the `storage`
  event broadcast (existing mechanism in `useAutoScrollPreference.ts`)

### AC-3: Content shrinking does NOT re-enable auto-scroll
- **Given** auto-scroll is OFF and the user is scrolled to some
  mid-conversation position (NOT at the bottom)
- **When** content below the viewport shrinks (e.g. a tool-call
  collapses, an image stops loading and reserves less space) such that
  the viewport now happens to fall within `AUTO_SCROLL_NEAR_BOTTOM_PX`
  of the new bottom **without the user moving the scrollbar**
- **Then** auto-scroll remains OFF
- **And** the pill behaviour follows whatever the current logic dictates
  (we are not changing pill semantics)

### AC-4: Initial session mount does NOT spuriously re-enable
- **Given** `helix.autoScroll` is persisted as `"false"`
- **And** the user opens a session for the first time
- **When** the initial `scrollToBottom(true)` effect runs (lines ~278-291
  in `EmbeddedSessionView.tsx`) to land the user on the latest message
- **Then** auto-scroll preference stays OFF (this is a programmatic
  scroll, not a user-initiated one — the user did not ask to follow new
  content)
- **And** subsequent new content stops landing in view; the pill appears
  when new content arrives below the viewport (existing behaviour)

### AC-5: Loading older messages preserves the OFF state
- **Given** auto-scroll is OFF and the user clicks "Show N older
  messages" (`handleLoadOlder`)
- **When** the pagination effect adjusts `scrollTop` to preserve viewport
  position after the older content is prepended (lines ~459-463)
- **Then** auto-scroll preference stays OFF (no spurious re-enable)

### AC-6: Upward-scroll unlock still works after a re-enable cycle
- **Given** auto-scroll was re-enabled via AC-1
- **When** the user immediately scrolls back up by ≥
  `USER_SCROLL_UNLOCK_PX`
- **Then** auto-scroll flips OFF as before
- **And** the `upwardAccumRef` cumulative-gesture state is consistent
  (no stale value from before the re-enable)

## Out of Scope

- Changing the 80px near-bottom threshold or the 100px unlock threshold.
- Changing pill or toggle-button visuals.
- Changing behaviour in surfaces that don't use `EmbeddedSessionView`
  (e.g. `RunnerLogs.tsx`, `LogViewerModal.tsx`, `PreviewPanel.tsx`,
  `DataGridWithFilters.tsx`). The auto-scroll model documented above
  is `EmbeddedSessionView` only.
- Adding new affordances (e.g. a "snap to bottom" gesture, a keyboard
  shortcut to toggle auto-scroll).
