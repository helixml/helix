# Requirements

## Context

The embedded session view in the spec task detail page (`EmbeddedSessionView`, used in `SpecTaskDetailContent.tsx`) currently has a global on/off auto-scroll preference (`helix.autoScroll`, default ON), persisted to localStorage. While ON, every content growth scrolls the chat to the bottom. The only way to disable auto-scroll today is to click the toggle button in the bottom-right corner.

The current behavior is intentionally simple — see commit `42c3a5112` — because earlier "sticky-scroll" detection had three race-condition surfaces around content reflow, RAF guards, and uncoordinated triggers. The maintainer explicitly removed wheel/touch listeners in that pivot.

This task reintroduces user-scroll detection, but in a way that does **not** reintroduce the prior races.

## User Story

**As a** user reading a long, actively-streaming agent session in the spec task detail page,
**I want** auto-scroll to disengage when I explicitly scroll up to read history,
**so that** I don't have to find and click the toggle button before content yanks me back to the bottom.

## Acceptance Criteria

1. While `autoScroll` is ON, if the user explicitly scrolls upward by a cumulative ≥ 100px (within a single gesture), `autoScroll` is set to OFF (the localStorage preference is updated, the toggle button updates, and the "Jump to latest" pill becomes available).
2. Programmatic scrolls performed by the auto-scroll machinery itself MUST NOT trip the threshold. (The component's own `container.scrollTop = container.scrollHeight` writes do not count as user scrolls.)
3. Content reflow above the viewport (image loads, syntax-highlight passes, polling re-renders) MUST NOT trip the threshold. Only direct user input counts.
4. Wheel scrolling (mouse wheel, trackpad), touch scrolling (finger drag on mobile/iPad), and scrollbar drag all trip the detection. Keyboard-driven scroll (PageUp / ArrowUp / Home) is a nice-to-have, not required.
5. Scrolling **down** does not affect the preference. Re-enabling auto-scroll continues to happen via the existing toggle button or the "Jump to latest" pill.
6. The 100px threshold is a single named constant in code (e.g. `USER_SCROLL_UNLOCK_PX`), tunable in one place.
7. When `autoScroll` is already OFF, the new logic is a no-op (does no extra work, fires no state updates).
8. Behavior is reliable on Chromium desktop **and** iOS Safari (the prior implementation specifically broke on iOS momentum scrolling — the new one must not).

## Non-Goals

- Re-implementing "scroll down to re-enable auto-scroll." That decision was deliberately removed in `42c3a5112`; users re-enable via the pill or toggle.
- Changing the default preference value, the toggle button UI, or the "Jump to latest" pill.
- Changing the auto-scroll mechanism itself (still ResizeObserver-driven on `contentRef`).
