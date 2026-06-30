# Add Desktop view to the Bot detail page

## Summary

The bot detail page (`HelixOrgBotDetail.tsx`) already showed the bot's
"Project Desktop" session as an inline **chat** transcript, but not its actual
desktop. This adds a **Chat | Desktop** toggle to that session panel so an
operator can watch and drive the bot's live desktop without leaving the page —
reusing the exact `ExternalAgentDesktopViewer` widget the spec-task detail
page uses (also used by `TeamDesktopPage`, `SandboxDesktopTab`, etc.).

Frontend-only — no backend changes. The Desktop view binds to the same
exploratory session the chat already resolves, and reuses the existing
external-agent WebSocket stream/input endpoints.

## Changes

- **New** `frontend/src/services/externalAgentDisplay.ts`: shared
  `deriveDisplaySettings(app?)` helper (resolution presets 1080p/4k/5k +
  explicit `display_width`/`display_height`/`display_refresh_rate`, fallback
  1920×1080×60) — extracted from `SpecTaskDetailContent.tsx` so both pages feed
  the viewer identical settings. Unit-tested (6 cases).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`: refactored to use
  the shared helper (behaviour unchanged).
- `frontend/src/pages/HelixOrgBotDetail.tsx`: added a `Chat | Desktop`
  `ToggleButtonGroup` to the session panel; the Desktop branch renders
  `ExternalAgentDesktopViewer` (`mode="stream"`) bound to the bot's resolved
  session, with display settings derived from the bot's `agent_app_id`. Empty
  state ("No desktop yet") when no session is resolved; the Chat panel is
  unchanged.

## Testing

- `tsc --noEmit` clean; `deriveDisplaySettings` unit test green.
- End-to-end in the inner Helix: created a bot, opened its detail page,
  confirmed the toggle — Chat shows the transcript and **Desktop streams the
  bot's live GNOME desktop**.

## Screenshots

![Chat view](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002186_we-need-to-add-desktop/screenshots/01-bot-detail-chat.png)
![Desktop view — live stream](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002186_we-need-to-add-desktop/screenshots/03-bot-detail-desktop.png)
