# Implementation Tasks: Add Desktop View to the Bot Detail Page

Frontend-only. Reuses `ExternalAgentDesktopViewer` and existing external-agent
WS endpoints — no backend changes. Coordinate with task 002185 (Role/Worker →
Bot merge): prefer landing on the merged `HelixOrgBotDetail.tsx`; keep changes
additive and localized to the session panel.

## 1. Shared display-settings helper
- [ ] Add `frontend/src/services/externalAgentDisplay.ts` exporting `deriveDisplaySettings(app?)` → `{ width, height, fps }` (resolution presets 1080p/4k/5k, `display_width`/`display_height`/`display_refresh_rate`, fallback 1920×1080×60), extracted from `SpecTaskDetailContent.tsx` (~lines 260–290).
- [ ] Refactor `SpecTaskDetailContent.tsx` to use the helper (behaviour unchanged).
- [ ] Add a unit test for `deriveDisplaySettings` (presets + fallback).

## 2. Desktop view on the Bot detail page
- [ ] In `HelixOrgBotDetail.tsx` (post-002185; else `HelixOrgWorkerDetail.tsx`), add a `ToggleButtonGroup` (Chat | Desktop) at the top of the session panel.
- [ ] Look up the bot's agent app via `agent_app_id` and compute display settings with `deriveDisplaySettings`.
- [ ] Render `ExternalAgentDesktopViewer` (`mode="stream"`, `sessionId`/`sandboxId` = the already-resolved `chatSessionId`, display width/height/fps) when Desktop is selected, inside the existing bounded (~520px) container.
- [ ] Keep the existing Chat panel (`EmbeddedSessionView` + `RobustPromptInput`) on the Chat toggle, unchanged.
- [ ] Show an empty/idle state ("No desktop yet") when `chatSessionId` is null; do not mount the viewer. Hide the Desktop view for a human-kind participant with no agent desktop (if applicable post-002185).

## 3. Verify
- [ ] `yarn build` + `yarn test` green; existing `HelixOrgWorkerDetail`/bot detail tests still pass (update for the toggle).
- [ ] Manual UI check at a bot detail page (e.g. `/orgs/.../helix-org/bots/<id>`): Chat still works, Desktop streams the bot's screen, lifecycle states render, and a bot with no session shows the empty state.
