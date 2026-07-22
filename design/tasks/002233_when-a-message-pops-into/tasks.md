# Implementation Tasks: Keep Spec-Task Chat Pinned to Bottom When the Message Queue Grows

- [ ] Reproduce the bug in the inner Helix: on a spec-task chat, queue a message and confirm (a) the tail is occluded by the grown composer and (b) with a large prompt, auto-scroll flips OFF. Note whether the lock flips without any wheel/trackpad input (resolves Open Question 1).
- [ ] Add a `repinToBottom()` method to `EmbeddedSessionViewHandle` in `EmbeddedSessionView.tsx`, exposed via `useImperativeHandle`. It re-pins to the bottom only when auto-scroll is ON, bypasses the `scrollHeight`-unchanged short-circuit, updates `lastScrolledHeightRef`/`lastScrollTopRef`, clears `hasNewBelow`, and never calls `setAutoScroll`/`triggerUnlock`.
- [ ] Update both composer mount sites in `SpecTaskDetailContent.tsx` (desktop `onHeightChange` ~line 2033 and mobile ~line 2838) to call `sessionViewRef.current?.repinToBottom()` from `onHeightChange` instead of `scrollToBottom()`.
- [ ] Leave `scrollToBottom` and its `scrollHeight` short-circuit intact for the poll/WS-update path (no redundant scroll work on keepalives).
- [ ] If the reproduce step found an automatic (no-user-scroll) disengage path, guard it so queuing/sending never triggers the unlock; otherwise document that pinning removes the reflexive-scroll trigger.
- [ ] `cd frontend && yarn build` and confirm it compiles.
- [ ] Verify end-to-end in the inner Helix per design.md testing steps (AC-1 through AC-7), including the mobile chat view and the auto-scroll-OFF case.
- [ ] Confirm no regression: genuine wheel/touch scroll-up still disengages auto-scroll; initial-open force-scroll and the "Jump to latest" pill still work.
