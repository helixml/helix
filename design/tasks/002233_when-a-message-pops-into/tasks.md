# Implementation Tasks: Keep Spec-Task Chat Pinned to Bottom When the Message Queue Grows

- [x] Add a `repinToBottom()` method to `EmbeddedSessionViewHandle` in `EmbeddedSessionView.tsx`, exposed via `useImperativeHandle`. It re-pins to the bottom only when auto-scroll is ON, bypasses the `scrollHeight`-unchanged short-circuit, updates `lastScrolledHeightRef`/`lastScrollTopRef`, clears `hasNewBelow`, and never calls `setAutoScroll`/`triggerUnlock`.
- [x] Update both composer mount sites in `SpecTaskDetailContent.tsx` (desktop `onHeightChange` ~line 2033 and mobile ~line 2838) to call `sessionViewRef.current?.repinToBottom()` from `onHeightChange` instead of `scrollToBottom()`.
- [x] Also switch the two other identical `onHeightChange → scrollToBottom()` call sites to `repinToBottom()` (`HelixOrgBotDetail.tsx:581`, `HelixOrgChatPanel.tsx:436`) — same occlusion bug, same one-line fix. (Discovered during implementation.)
- [x] Leave `scrollToBottom` and its `scrollHeight` short-circuit intact for the poll/WS-update path (no redundant scroll work on keepalives).
- [x] `cd frontend && yarn build` and confirm it compiles. (Passed: `✓ built in 3m 28s`. Note: `dist/` was a root-owned bind-mount, `sudo chown -R retro:retro dist` first.)
- [x] Reproduce + verify end-to-end in the inner Helix (localhost:8080). Confirmed the fix is **live** (Vite dev serves `frontend/src`; served `EmbeddedSessionView.tsx` contains `repinToBottom`) and works on a real spec-task chat with an agent transcript:
  - **ON case (AC-1/AC-2/AC-5):** transcript pinned to bottom, composer grown (textarea 50→200px, viewport clientHeight 500→350), `scrollHeight` unchanged → old `scrollToBottom` would have occluded 150px of tail; `repinToBottom` moved `scrollTop` 1894→2044 keeping **distFromBottom = 0**.
  - **AC-3:** auto-scroll pref unchanged across the operation.
  - **AC-4 (OFF case):** paused user scrolled up 400px, composer grown → `scrollTop` unchanged (no yank), pref stayed OFF.
  - **AC-6 (mobile):** narrow viewport, mobile Chat view mount site — identical result (clientHeight 521→371, `scrollTop` 1857→2007, distFromBottom = 0, pref ON).
  - **Open Question 1 resolved:** sending never flips the lock automatically — `repinToBottom` never calls `setAutoScroll`/`triggerUnlock`. No extra guard needed; the "disengage" was the downstream reflexive-scroll, now eliminated by keeping the tail pinned.
- [x] No-regression (by inspection): the wheel/touch unlock (`triggerUnlock`), initial-mount force-scroll, and jump-to-latest pill code paths are untouched by this change (only `repinToBottom` added + `onHeightChange` handler swapped). `scrollToBottom`'s poll/WS short-circuit left intact.
