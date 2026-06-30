# Implementation Tasks: Add Desktop View to the Bot Detail Page

Frontend-only. Reuses `ExternalAgentDesktopViewer` and existing external-agent
WS endpoints — no backend changes. Task 002185 (Role/Worker → Bot merge) has
**landed in main**, so this builds directly on `HelixOrgBotDetail.tsx`
(`useHelixOrgBot`/`BotDetailDTO` already expose `project_id`/`agent_app_id`).
Bots have no `kind`, so there is no AI/human gating.

## 1. Shared display-settings helper [x]
- [x] Add `frontend/src/services/externalAgentDisplay.ts` exporting `deriveDisplaySettings(app?)` → `{ width, height, fps }` (resolution presets 1080p/4k/5k, `display_width`/`display_height`/`display_refresh_rate`, fallback 1920×1080×60), extracted from `SpecTaskDetailContent.tsx` (~lines 260–290).
- [x] Refactor `SpecTaskDetailContent.tsx` to use the helper (behaviour unchanged).
- [x] Add a unit test for `deriveDisplaySettings` (presets + fallback) — 6 cases, all green.

## 2. Desktop view on the Bot detail page (`HelixOrgBotDetail.tsx`)
- [ ] Add a `ToggleButtonGroup` (Chat | Desktop) at the top of the existing session panel.
- [ ] Look up the bot's agent app via `agentAppID` (`data.agent_app_id`) and compute display settings with `deriveDisplaySettings`.
- [ ] Render `ExternalAgentDesktopViewer` (`mode="stream"`, `sessionId`/`sandboxId` = the already-resolved `chatSessionId`, display width/height/fps) when Desktop is selected, inside the existing bounded (~520px) container.
- [ ] Keep the existing Chat panel (`EmbeddedSessionView` + `RobustPromptInput`) on the Chat toggle, unchanged.
- [ ] Show an empty/idle state ("No desktop yet") when `chatSessionId` is null; do not mount the viewer. (No `kind` gating — bots are singular.)

## 3. Verify
- [ ] `yarn build` + `yarn test` green; update/extend the bot detail test for the toggle.
- [ ] Manual UI check at a bot detail page (e.g. `/orgs/.../helix-org/bots/<id>`): Chat still works, Desktop streams the bot's screen, lifecycle states render, and a bot with no session shows the empty state.
