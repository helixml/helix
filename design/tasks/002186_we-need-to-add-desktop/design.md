# Design: Add Desktop View to the Bot Detail Page

## Core decision: reuse `ExternalAgentDesktopViewer`, don't build a new widget

`ExternalAgentDesktopViewer`
(`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx`) is
already the shared remote-desktop widget across the app (~12 call sites:
`SpecTaskDetailContent`, `TeamDesktopPage`, `SandboxDesktopTab`,
`ProjectSettings`, `Jobs`, …). It wraps `DesktopStreamViewer` and handles the
full sandbox lifecycle (starting/running/paused), H.264 WS streaming,
input, and screenshot fallback.

It takes a `sessionId`/`sandboxId` plus display dimensions and is fully
self-contained. Therefore: **reuse it directly** on the Bot detail page. A
custom component would re-implement lifecycle + streaming for no benefit. The
only new work is *wiring* — resolving the session id and display settings the
widget already expects, and giving it a place in the page.

Reference call (from `SpecTaskDetailContent.tsx` ~line 2429):
```tsx
<ExternalAgentDesktopViewer
  sessionId={activeSessionId}
  sandboxId={activeSessionId}
  mode="stream"
  displayWidth={displaySettings.width}
  displayHeight={displaySettings.height}
  displayFps={displaySettings.fps}
/>
```

## Where it goes

The Bot detail page (`frontend/src/pages/HelixOrgBotDetail.tsx`, the merged
page from landed task 002185) already resolves the bot's session and renders
an inline **chat** panel (verified in main):
- `const { data } = useHelixOrgBot(botId)` (returns `BotDetailDTO`);
  `projectID = data.project_id`, `agentAppID = data.agent_app_id`
- `chatSessionId` resolved by `fetchExistingWorkerSession(projectID, chatApi)`
  → `v1ProjectsExploratorySessionDetail(projectID)` (GET-only; 204 → null) —
  the bot's long-lived "Project Desktop" exploratory session
- the resolved session also feeds `streaming.setCurrentSessionId(...)`
- project/agent links + `useRestartBotAgent` already live in the right rail

Add a **Chat | Desktop** toggle (MUI `ToggleButtonGroup`) at the top of that
session panel. Both views share the **same `chatSessionId`**:
- **Chat** → existing `EmbeddedSessionView` + `RobustPromptInput` (unchanged).
- **Desktop** → `ExternalAgentDesktopViewer mode="stream"` with
  `sessionId={chatSessionId}` / `sandboxId={chatSessionId}` and the derived
  display settings, inside the same bounded (~520px) flex container the chat
  panel uses.

When `chatSessionId` is null, both views show the existing empty state
("No conversation yet" / "No desktop yet") rather than mounting the viewer.

(Alternative considered: a separate full-page route like `TeamDesktopPage`.
Rejected — the bot already has an inline session panel and the request is to
add the *widget* to the bot view, so an in-page toggle is the smaller, more
cohesive change. Implementer may use discretion if a dedicated sub-route
proves cleaner.)

## Display settings helper

The spec-task page derives display settings from its app config
(`SpecTaskDetailContent.tsx` ~lines 260–290): read
`app.config.helix.external_agent_config`, honour `resolution` presets
(`1080p`/`4k`/`5k`) and `display_width`/`display_height`/`display_refresh_rate`,
default to 1920×1080×60.

To avoid copy-paste, **extract that logic into a small shared helper**, e.g.
`frontend/src/services/externalAgentDisplay.ts`:
```ts
export function deriveDisplaySettings(app?: TypesApp):
  { width: number; height: number; fps: number }
```
Call it from the Bot detail page using the app looked up by `agentAppID`
(via the existing apps list / `useApp`), and refactor `SpecTaskDetailContent`
to use the same helper. Keep the fallback identical so behaviour is unchanged.

## Data flow (no backend changes)

```
BotDetail page
  ├─ useHelixOrgBot(botId)  →  project_id, agent_app_id
  ├─ fetchExistingWorkerSession(project_id)  →  sessionId   (GET-only)
  ├─ deriveDisplaySettings(app[agent_app_id])  →  w/h/fps
  └─ <ExternalAgentDesktopViewer sessionId=… mode="stream" …/>
        └─ existing WS endpoints: /api/v1/external-agents/{sessionID}/ws/stream
                                  /api/v1/external-agents/{sessionID}/ws/input
```

All streaming/input endpoints, RevDial proxying, and authorization already
exist and are reused unchanged.

## Relationship to task 002185 (Role/Worker → Bot merge) — LANDED

002185 has **merged into main**. Verified against the current code:
- `frontend/src/pages/HelixOrgBotDetail.tsx` exists (renamed from
  `HelixOrgWorkerDetail.tsx`); `HelixOrgWorkerDetail.tsx`/`...RoleDetail.tsx`
  are gone.
- `useHelixOrgBot(botId)` returns `BotDetailDTO` (`ApiBotDetailDTO`) and the
  page already reads `data.project_id` + `data.agent_app_id` (the **data
  dependency this feature needs is satisfied** — no API work required).
- The inline chat panel (`EmbeddedSessionView` + `RobustPromptInput`),
  `fetchExistingWorkerSession`, project/agent links, and `useRestartBotAgent`
  are all present.
- Bots have **no `kind`** — the old `worker.kind === 'ai'` gating is gone, so
  there is no AI/human branch to worry about.

There is therefore **no pending merge to coordinate with**. This is a clean
additive change to one existing file plus one new helper.

## Files touched (frontend only)
- `frontend/src/pages/HelixOrgBotDetail.tsx` — add Chat|Desktop toggle +
  `ExternalAgentDesktopViewer` branch beside the existing chat panel.
- `frontend/src/services/externalAgentDisplay.ts` — **new** shared
  `deriveDisplaySettings` helper.
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — refactor to use
  the shared helper (optional but recommended).

## Risks
- **Display-settings divergence** — mitigated by sharing one helper.
- **Session not yet provisioned** — handled by the existing GET-only resolve +
  empty state; never provision on page open.
