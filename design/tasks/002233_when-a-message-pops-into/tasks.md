# Implementation Tasks: Keep Spec-Task Chat Pinned to Bottom When the Message Queue Grows

- [x] Add a `repinToBottom()` method to `EmbeddedSessionViewHandle` in `EmbeddedSessionView.tsx`, exposed via `useImperativeHandle`. It re-pins to the bottom only when auto-scroll is ON, bypasses the `scrollHeight`-unchanged short-circuit, updates `lastScrolledHeightRef`/`lastScrollTopRef`, clears `hasNewBelow`, and never calls `setAutoScroll`/`triggerUnlock`.
- [x] Update both composer mount sites in `SpecTaskDetailContent.tsx` (desktop `onHeightChange` ~line 2033 and mobile ~line 2838) to call `sessionViewRef.current?.repinToBottom()` from `onHeightChange` instead of `scrollToBottom()`.
- [x] Also switch the two other identical `onHeightChange → scrollToBottom()` call sites to `repinToBottom()` (`HelixOrgBotDetail.tsx:581`, `HelixOrgChatPanel.tsx:436`) — same occlusion bug, same one-line fix. (Discovered during implementation.)
- [x] Leave `scrollToBottom` and its `scrollHeight` short-circuit intact for the poll/WS-update path (no redundant scroll work on keepalives).
- [x] `cd frontend && yarn build` and confirm it compiles. (Passed: `✓ built in 3m 28s`. Note: `dist/` was a root-owned bind-mount, `sudo chown -R retro:retro dist` first.)
- [ ] Reproduce + verify end-to-end in the inner Helix per design.md testing steps (AC-1 through AC-7), including the mobile chat view and the auto-scroll-OFF case. Note whether the lock flips without any wheel/trackpad input (resolves Open Question 1); if an automatic disengage path exists, guard it so queuing/sending never triggers the unlock.
- [ ] Confirm no regression: genuine wheel/touch scroll-up still disengages auto-scroll; initial-open force-scroll and the "Jump to latest" pill still work.
