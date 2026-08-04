# T3 Code-inspired agent chat redesign

**Status:** Visual/mechanics pass implemented on `feat/t3-chat-redesign`; durable attachment binding remains planned

**Date:** 2026-08-03

**Scope:** Helix org-chart agent chat and project spec-task chat

## Summary

Adopt T3 Code's chat hierarchy for Helix's two agent-chat surfaces:

- a quiet near-black transcript canvas;
- compact 14 px DM Sans message typography;
- right-aligned, softly raised user messages;
- unboxed assistant output;
- thumbnail-first image attachments in both the composer and sent message;
- one durable attachment model shared by direct org-chat sends and queued spec-task sends.

Use bundled DM Sans as the application-wide UI font so root DOM, MUI components, portals, and chat render the same face. Keep the T3 dark palette scoped to the shared chat surface, with Helix cyan retained for focus and primary actions. Do not implement sent-image previews using only browser blob URLs or sandbox paths embedded in visible message text: neither is durable message data.

## Reference and method

The catalogue is based on `pingdotgg/t3code` commit [`30c96228067bcd3a49e432ec898e52d4acb04297`](https://github.com/pingdotgg/t3code/commit/30c96228067bcd3a49e432ec898e52d4acb04297), dated 2026-08-02.

Primary source files:

- [`apps/web/src/index.css`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/apps/web/src/index.css)
- [`MessagesTimeline.tsx`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/apps/web/src/components/chat/MessagesTimeline.tsx)
- [`ChatComposer.tsx`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/apps/web/src/components/chat/ChatComposer.tsx)
- [`ComposerPromptEditor.tsx`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/apps/web/src/components/ComposerPromptEditor.tsx)
- [`ExpandedImageDialog.tsx`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/apps/web/src/components/chat/ExpandedImageDialog.tsx)
- [`orchestration.ts`](https://github.com/pingdotgg/t3code/blob/30c96228067bcd3a49e432ec898e52d4acb04297/packages/contracts/src/orchestration.ts)

The public hosted app had no connected environment, so a populated chat could not be inspected there. The visual measurements below are source-derived rather than estimates from a screenshot. The live dark shell did confirm the DM Sans stack and neutral dark theme.

## T3 Code catalogue

### Typography

| Element | T3 Code value |
|---|---|
| UI and chat sans | `DM Sans Variable`, `DM Sans`, Apple/system fallbacks |
| Monospace | `SF Mono`, `SFMono-Regular`, `JetBrains Mono`, Consolas, Liberation Mono, Menlo |
| User and assistant text | 14 px (`text-sm`) |
| Message line height | `1.625` (`leading-relaxed`), about 22.75 px at 14 px |
| Composer text | 14 px from the `sm` breakpoint; 16 px on narrow screens to avoid mobile input zoom |
| Timestamp and message actions | 12 px (`text-xs`) |
| Compact work/tool metadata | 10–12 px; long raw output uses 11 px mono |

T3 bundles DM Sans Variable and JetBrains Mono through Fontsource. Helix currently declares IBM Plex Sans but does not bundle it, so most clients fall through to Helvetica or Arial.

### Dark palette

T3 uses semantic translucent layers over a neutral-black base rather than many unrelated opaque grays.

| Token/use | T3 Code value |
|---|---|
| Workspace background | Tailwind `neutral-950`, `oklch(14.5% 0 0)` |
| Primary foreground | `neutral-100`, `oklch(97% 0 0)` |
| Card | 3% white mixed into the background |
| Popover | 6% white mixed into the background |
| Accent, muted, secondary | 4% white alpha |
| Border | 6% white alpha |
| Input border/surface | 8% white alpha |
| Muted foreground | approximately `#818181` in the current theme |
| User bubble | `accent`: 4% white over the dark background |
| Assistant body | primary foreground at 80% opacity |

The composer is a rounded glass layer: 22 px outer radius, a subtly lifted dark surface, a 5% white outline, a 3% inset highlight, and no dark-mode drop shadow.

### Transcript geometry

| Element | T3 Code value |
|---|---|
| Timeline and composer maximum width | 48 rem / 768 px (`max-w-3xl`) |
| Timeline horizontal inset | 12 px narrow, 20 px from `sm` |
| Normal turn spacing | 16 px below the row |
| User alignment | right |
| User bubble maximum width | 80% of timeline |
| User bubble chrome | 16 px radius, 12 px padding, no border |
| Assistant chrome | no bubble; 4 px horizontal and 2 px vertical inset |
| User metadata | right-aligned below bubble, hidden until hover/focus |

Long user messages collapse when they exceed either 600 characters or eight lines. The collapsed body is at most 176 px high with a bottom fade and an explicit “Show full message” control.

### Turn and interrupt mechanics

T3 gives a newly submitted turn a viewport-sized end spacer. Helix keeps the
streaming status in the normal transcript flow instead: reserving that height
on the whole interaction created a large blank region and visually displaced
the status row. Manual upward scrolling still disengages auto-scroll.

During an active turn, the composer swaps in a prominent destructive stop control: a red circular button with a white square. It is absent while idle. The control cancels the current turn without clearing the composer or changing the active session, so the next normal send continues in the same thread.

### Image attachment UX

T3 treats images as message data, not as paths inserted into message text.

Composer behavior:

- accepts pasted and dropped image files;
- validates image MIME types;
- allows up to eight images per turn;
- caps each transmitted image at 10 MiB;
- downscales oversized images instead of immediately rejecting them, preserving at most a 2048 px longest edge during re-encoding;
- shows each image as a 64 × 64 px, 8 px-radius, `object-cover` tile;
- places remove and persistence-warning actions over the tile;
- opens the same full-screen viewer used by sent messages when a tile is clicked.

Sent-message behavior:

- renders images before the user's text inside the bubble;
- uses a two-column grid, 8 px gap, maximum width 420 px;
- uses 8 px-radius tiles with a subtle border and dark background;
- caps displayed image height at 220 px and uses `object-cover`;
- retains an optimistic blob preview until the durable server preview URL is available, avoiding a visible flash;
- opens a 75% black lightbox with Escape-to-close, click-outside close, arrow-key navigation, previous/next buttons, filename, and position (`2/4`).

## Current Helix implementation

Both requested surfaces share the transcript renderer and composer primitives but duplicate their wiring:

- org chat: `frontend/src/components/helix-org/HelixOrgChatPanel.tsx`
- spec-task desktop and mobile chat: `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
- shared transcript: `frontend/src/components/session/EmbeddedSessionView.tsx`
- message row: `frontend/src/components/session/Interaction.tsx`
- message bubble: `frontend/src/components/session/InteractionContainer.tsx`
- image rendering: `frontend/src/components/session/InteractionInference.tsx`
- composer: `frontend/src/components/common/RobustPromptInput.tsx`

### Structured activity ordering

`response_entries` is an ordered transcript, not two independent collections.
The renderer keeps assistant progress text in its original position and collapses
only adjacent tool calls into a run. Each run shows its newest call and a
`+N previous tool calls` disclosure; later prose starts a new run. The last text
entry of a completed turn remains the final answer below the work disclosure.
While streaming, every visible prose or tool entry stays in the live timeline
because there is not yet a reliable final-answer boundary. The live activity
surface is intentionally compact: it keeps visible progress prose, collapses all
tool calls into one run whose newest call remains visible, and hides internal
thinking because the persistent `Working for …` status already communicates the
active state. Once the turn completes, internal thinking entries return to their
original positions inside the work transcript and split tool runs, so only
genuinely adjacent calls are collapsed together. The disclosure body renders as
Markdown; bold-only thought summaries are presented as one list rather than
showing literal Markdown punctuation. The bulb icon identifies thought rows, so
their headers show the first summary directly without a repeated `Thoughts`
label. The previous-tool-call disclosure uses the same high-contrast label color
and left icon alignment as the visible tool row, while its chevron uses the same
subtle icon color as the tool-kind icon. Expanding keeps the latest call and
disclosure fixed, inserting older calls below the disclosure so the cursor does
not have to chase a relocated collapse control. The redundant expanded `Tool
calls` header is omitted to keep activity-group spacing compact.

Primary assistant prose uses a high-contrast near-white foreground in dark mode,
including links. Reasoning summaries, command previews, durations, and other
activity metadata use two brighter gray tiers rather than opacity-composited
text: readable secondary labels and quieter tertiary details. Activity rows use
the shared application mono font so their metrics are consistent with the T3
Code-style work log.

Shell results use `Ran command` plus the command preview. MCP calls use
`Provider · tool` labels. This presentation is derived from the structured tool
metadata and terminal result shape; it does not change or discard raw output.

### Current visual differences

| Area | Helix today | T3 direction |
|---|---|---|
| Sans font | IBM Plex declaration without a bundled font; practical fallback is Helvetica/Arial | Bundled DM Sans Variable |
| Chat background | T3 `neutral-950` (`#0a0a0a`) | exact scoped dark-mode canvas |
| Transcript width | 700 px | 768 px |
| User width | fit-content up to nearly the full 700 px | maximum 80% |
| User bubble | 16 px radius, 1 px `#33373a` border, 16 px horizontal/4 px vertical padding | borderless 4% white surface, 12 px padding |
| Assistant | unboxed, but shares old typography and spacing | unboxed with quieter 80% text |
| Message text | 0.9 rem markdown, about 14.4 px | 14 px with consistent relaxed line height |
| Sent images | vertically repeated 150 × 150 previews | compact two-column gallery in the user bubble |
| Image viewer | image-only backdrop; no filename, navigation, close button, or keyboard handling | accessible gallery lightbox |
| Composer preview | chip with a 20 × 20 icon-sized crop and filename | 64 × 64 visual tile tray |

### Functional attachment gap

`RobustPromptInput` supports an upload callback, paste, drag/drop, and attachment chips, but neither target surface passes `onFileUpload` or `onImagePaste`. Attachments are therefore disabled in both target composers.

The spec-task page has a separate upload action that writes files to `~/work/incoming`; it does not associate the file with a chat message or render it in message history. Where `RobustPromptInput` uploads are enabled elsewhere, uploaded sandbox paths are prepended to the text. That is useful transport for a coding agent, but it is not sufficient UI state:

- the path becomes visible message content;
- a browser blob preview disappears after reload;
- the sandbox file is not a durable, authorized preview source;
- queued spec-task sends and direct org-chat sends take different code paths;
- a retry must retain exactly the same attachment association.

## Proposed Helix design

### Visual tokens

Add chat-scoped semantic tokens rather than changing the global MUI palette.

| Token | Dark value | Use |
|---|---|---|
| `chat.canvas` | application `background.default` (`#0a0a0a` in dark mode) | transcript and composer dock; the literal is defined once as `DARK_APP_BACKGROUND` in `themeTokens.ts` |
| `chat.text` | `#f5f5f5` | user message text |
| `chat.assistantText` | `rgba(245,245,245,0.80)` | assistant markdown; the opacity is applied at the message container so nested Markdown inherits it consistently |
| `chat.mutedText` | `#818181` | timestamps and secondary metadata |
| `chat.userBubble` | `rgba(255,255,255,0.04)` | user bubble |
| `chat.raised` | `rgba(255,255,255,0.03)` | composer surface |
| `chat.border` | `rgba(255,255,255,0.06)` | passive separators |
| `chat.inputBorder` | `rgba(255,255,255,0.08)` | composer and attachment tiles |
| `chat.focus` | Helix cyan `#00d5ff` | focus ring and primary chat actions |

Light mode retains the existing Helix light palette in the first change. The new tokens must still define a legible light equivalent so the components do not contain mode checks.

Bundle `@fontsource-variable/dm-sans` and `@fontsource-variable/jetbrains-mono`. Apply DM Sans through the root MUI theme and `CssBaseline`; plain form controls inherit the root face, while deliberate monospace overrides remain intact. Override MUI's forced grayscale font smoothing with the browser default (`auto`) to match T3's rendering rather than making DM Sans artificially thin on macOS/Chromium.

### Shared component boundary

Introduce one composed `AgentChat` component for both surfaces. Page-specific headers, thread selectors, and desktop/spec tabs remain outside it.

```text
HelixOrgChatPanel ─┐
                   ├─ AgentChat
SpecTaskDetail ────┘    ├─ EmbeddedSessionView
                        │    └─ Interaction
                        │         ├─ UserMessage
                        │         │    └─ ChatImageGrid
                        │         └─ AssistantMessage
                        ├─ SessionPromptQueue (configured by queue mode)
                        └─ RobustPromptInput
                             └─ ChatAttachmentTray

AgentChat root ── one ImageLightbox instance for composer and transcript images
```

Suggested `AgentChat` inputs:

- `sessionId`
- `projectId`
- optional `specTaskId`
- `placeholder`
- `disabled`
- `onWillSend`
- `onCancel`
- `isAgentBusy`
- `showPromptQueue`
- `enableInteractionDebugCopy`

The component owns scroll-to-bottom coordination, upload integration, attachment lightbox state, and consistent composer padding. It does not own page navigation or session/thread selection.

### Message presentation

- Set transcript and composer content to a shared 768 px maximum width with 12 px mobile and 20 px desktop gutters.
- Render user bubbles at `max-width: 80%`, 16 px radius, 12 px padding, no border.
- Render assistant output full-width without a bubble; use 14 px/1.625 text at 80% foreground.
- Prefer `interaction.display_message` for the visible user text. Keep transport-only sandbox paths in `prompt_message` so agents can read files without exposing those paths as the user's prose.
- Move timestamp, copy, edit, and debug actions into a 12 px row below the user bubble. Reveal it on hover and `focus-within`; keep it reachable by keyboard and visible on touch layouts.
- Add T3-style long-user-message disclosure using the 600-character/eight-line threshold and a 176 px faded collapsed state.
- Preserve the current system-prefix, fork, activity, tool-call, and streaming components. Restyle their surfaces using chat tokens rather than removing them.

### Turn mechanics

- Keep the newest interaction in normal document flow while it is `waiting`.
- Continue using the existing scroll-to-bottom operation without adding a
  second viewport-sized layout allocation.
- Keep the streaming activity summary adjacent to the rendered response so an
  invisible spacer cannot push it away from the transcript.
- Respect the existing manual-scroll unlock. Do not force the viewport back to the active turn after the operator scrolls up.
- Derive busy state inside `AgentChat` from the newest interaction so org chat and both spec-task layouts show the same red interrupt control.
- After interrupting, keep the composer enabled and preserve session identity; the next send is the required lifecycle seam test.

### Attachment presentation

Composer:

- Add a visible attachment button; keep paste and drag/drop.
- Initial scope is images only. Do not advertise PDFs or arbitrary files in this tray until they have a designed sent-message representation.
- Show 64 × 64 px tiles above the editor, wrapping with an 8 px gap.
- Tile states: uploading progress, uploaded, failed with retry, and remove.
- Clicking a tile opens the shared lightbox.
- Allow eight images per message and 10 MiB per normalized image. Re-encode oversized images to a maximum 2048 px longest edge. Reject unreadable images and source files too large to decode safely.

Sent user message:

- Render image attachments before text in a two-column grid capped at 420 px.
- Use 8 px gaps/radii, subtle token border, and 220 px maximum tile height.
- Preserve the original filename for accessible labels and the lightbox caption.
- For attachment-only messages, render the gallery without an empty text block.

Lightbox:

- one portal-backed dialog shared across the chat;
- 75% black backdrop;
- close button and click-outside close;
- Escape, Left, and Right keys;
- previous/next buttons only for multi-image messages;
- contained image up to 86 vh × 92 vw;
- filename and current/total count;
- focus trap and focus restoration to the opening thumbnail.

### Durable attachment model

Use a generic session-chat attachment primitive; do not create separate org-chat and spec-task attachment implementations.

Proposed typed record:

```go
type ChatAttachment struct {
    ID             string
    SessionID      string
    OwnerID        string
    PromptHistoryID string
    InteractionID  string
    Name           string
    MIMEType       string
    SizeBytes      int64
    FilestorePath  string
    SandboxPath    string
    CreatedAt      time.Time
}
```

The association IDs are empty while the attachment belongs to a draft. Binding attachments to a prompt must be transactional and idempotent. Exact GORM tags and cleanup fields belong in implementation, not in this UI catalogue.

Data flow:

1. Composer uploads an image through a generated-client `POST /api/v1/sessions/{session_id}/chat-attachments` endpoint.
2. The server authorizes write access to the session, validates and normalizes the image, stores a durable preview in the Helix filestore, copies the agent-readable file into `~/work/incoming`, and returns typed metadata.
3. The composer queues/sends visible text plus attachment IDs. Local draft state keeps IDs and preview URLs, not base64 payloads.
4. The server binds the IDs to the `PromptHistoryEntry`. Add an `AttachmentIDs` JSONB field to both the sync and session-scoped queue paths.
5. Queue dispatch builds transport text from `SandboxPath` plus the user's message, while storing the clean user text in `Interaction.DisplayMessage`.
6. The created interaction receives structured attachment metadata in `PromptMessageContent` or a typed interaction attachment field. Store stable filestore keys, not expiring signed URLs and not base64 image bodies in Postgres.
7. `EmbeddedSessionView` resolves authorized preview URLs and renders `ChatImageGrid` identically after polling, WebSocket updates, retry, and reload.

For direct org-chat dispatch, extend the send request with attachment IDs and run the same server binding/interaction construction use case. For queued spec-task dispatch, `PromptHistoryEntry` carries those IDs until the message is claimed. Both paths must call the same attachment-binding and transport-text builder.

Unbound draft attachments should be garbage-collected after a bounded interval. Removing a tile may delete an unbound attachment immediately; attachments bound to prompts/interactions remain immutable history.

### Security and correctness constraints

- Resolve and authorize the session before accepting, binding, or serving an attachment.
- Attachment ownership and session ID must match the prompt and interaction.
- Ignore client-supplied filestore or sandbox paths; the server owns both.
- Sanitize filenames and generate collision-free storage names.
- Validate decoded image content, not only MIME type or extension.
- Do not expose container filesystem paths as preview URLs.
- Retry and queue reordering must retain attachment IDs without re-uploading or duplicating files.
- Deleting or clearing a draft must not delete an attachment already bound to an interaction.

## Implementation plan

### Transcript turn navigator

The shared transcript includes a desktop-only turn navigator modeled on T3 Code's minimap. Each visible user turn gets an evenly spaced marker in the reserved left gutter. Hovering the rail expands the nearest marker and shows one line of user text plus up to three lines of the final assistant prose. Clicking, Enter, or Space smooth-scrolls to that turn and pauses auto-follow; Arrow keys and Home/End move between previews. Markers for turns intersecting the viewport use the foreground color.

The navigator is absent on coarse pointers and when fewer than two turns exist. Its collapsed hit area stays inside the 36 px transcript gutter so it does not block message selection. The transcript root and every intervening flex item use `min-width: 0`; horizontal overflow is clipped at the transcript scroller so narrow split panes retain their 20 px right content margin.

### 1. Shared visual foundation

- Add bundled DM Sans Variable and JetBrains Mono.
- Add typed chat theme tokens and a scoped chat root.
- Restyle `EmbeddedSessionView`, `InteractionContainer`, `InteractionInference`, and markdown typography.
- Add `UserMessageBody` disclosure and hover/focus metadata treatment.
- Extract `ImageLightbox`, initially replacing the current image-only overlay.

### 2. Durable attachment primitive

- Add `ChatAttachment`, AutoMigrate registration, store interface, and store tests.
- Add upload/bind/read endpoints with Swagger annotations, then run `./stack update_openapi`.
- Implement a frontend React Query service using the generated API client.
- Extend prompt-history sync and direct session send types with attachment IDs.
- Centralize interaction creation and agent transport-text construction so both queue paths behave identically.

### 3. Shared composer and message gallery

- Replace attachment chips with `ChatAttachmentTray` tiles.
- Add picker, paste, drop, validation, progress, retry, and removal.
- Add `ChatImageGrid` to user messages and connect it to the shared lightbox.
- Introduce `AgentChat` and replace the duplicated org/spec desktop/spec mobile wiring.
- Remove the target surfaces' obsolete raw-fetch upload path once every upload entry point uses the generated client.

### 4. Verification

- Frontend unit tests: bubble geometry classes/styles, collapse thresholds, attachment-only messages, tile state transitions, lightbox keyboard behavior, and clean visible text versus transport paths.
- Go tests: authorization, image validation, transactional binding, session mismatch rejection, queue retry idempotency, direct-send/queued-send parity, and cleanup of unbound drafts.
- Build checks: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`.
- Live inner-Helix desktop and mobile-width checks in both dark and light mode.
- Org-chat E2E: start an agent, attach two images, send, verify agent receipt, reload, reopen both images in the lightbox, then send a normal text-only follow-up.
- Spec-task E2E against a live connected Zed: attach two images to a queued prompt, verify one interaction and one delivery, reload while queued and after sent, retry a failed upload, then send the immediately following normal prompt.

## Acceptance criteria

- Org-chart chat and spec-task chat render through the same `AgentChat` composition.
- Dark chat uses the documented scoped font, sizes, palette, widths, and message hierarchy.
- User messages are right-aligned, at most 80% wide, and visually distinct without a heavy border.
- Assistant content remains unboxed and readable.
- Pasted, dropped, and picked images show 64 px composer previews with explicit upload state.
- Sent images remain visible and zoomable after reload and across devices.
- Multi-image lightbox navigation works with mouse, touch, and keyboard.
- The visible user message never includes transport-only sandbox paths.
- Direct org sends and queued spec-task sends preserve the same attachment metadata and retry semantics.
- No raw frontend request is added where a generated API client method can be used.
- Existing light mode, system prefixes, tool calls, prompt queueing, interrupt mode, auto-scroll, and live streaming still work.
- Desktop chats expose a keyboard-accessible turn navigator with T3-style hover previews and manual navigation pauses auto-follow.

## Explicit non-goals

- Replacing the global Helix application theme or typography.
- Copying T3's provider/model controls, prompt stash, or review-annotation system.
- General document/PDF attachment cards in the first pass.
- Sending authenticated preview URLs directly to the external agent. The agent receives server-built workspace paths; the UI receives authorized filestore previews.
