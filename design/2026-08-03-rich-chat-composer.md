# Rich chat composer

## T3 Code reference

Inspected `pingdotgg/t3code` at commit
`30c96228067bcd3a49e432ec898e52d4acb04297` (2026-08-02).

The desktop/web composer uses:

- a 768px (`max-w-3xl`) centered glass surface with a 22px outer radius and
  20px inner radius;
- 16px horizontal/vertical inset on desktop and 12px on compact layouts;
- DM Sans Variable, 14px desktop text, 16px mobile text, relaxed line height,
  a 70px minimum editor height, and a 200px maximum height;
- a subtle card-colored glass fill, 5–8% border, small light-mode shadow, and
  no dark-mode drop shadow;
- a bottom toolbar of muted 28px controls and a circular 32px send/stop action;
- 64×64 image thumbnails with an 8px radius, zoom preview, upload processing,
  and a small overlaid remove button;
- whole-composer drag/drop highlighting and clipboard image capture.

T3 currently accepts images only: at most eight per message and 10 MB per
image after compression. It does not accept pasted PDFs or arbitrary files.

## Helix data flow

Org-chart chat and spec-task chat both render `AgentChat`, which renders
`RobustPromptInput`. The composer uploads each file through the generated
`v1ExternalAgentsUploadCreate` client. The API streams the multipart body over
RevDial to the desktop bridge, which stores a collision-safe file under
`/home/retro/work/incoming/`. That directory is inside the same agent desktop,
so the coding agent can inspect images, PDFs, archives, and source files with
its normal filesystem tools.

The queued prompt carries a short attachment manifest containing the returned
absolute paths. This is required because prompt history and the external-agent
message protocol currently transport text, not structured file parts. The
uploaded file remains the source of truth; no base64 payload is placed in the
prompt or database. On read, the shared interaction renderer separates that
trailing manifest from the visible user text and renders the corresponding
image preview or file card. The agent still receives the complete manifest.

Historical previews use an authenticated same-origin endpoint that accepts
only a direct filename from the incoming directory. The API authorizes access
to the session and proxies the response over RevDial; the desktop bridge
independently rejects traversal, nested paths, symlinks outside the incoming
directory, and non-regular files.

## Helix behavior

- Paste (`Cmd+V` / `Ctrl+V`), drag/drop, file picker, and mobile camera all use
  one validation and upload pipeline.
- Clipboard `DataTransfer.files` is used rather than filtering clipboard items
  to images, allowing copied PDFs and other files to work.
- Images get an immediate local thumbnail and lightbox. Other files get a
  compact name/type/size card. All previews live inside the composer surface.
- Sent messages retain those previews in chat history without showing the
  agent-facing workspace manifest or absolute path.
- Queued prompts are rendered as a muted extension of the composer instead of
  a separate colored status card. During generation, the primary action is a
  single red stop button rather than simultaneous stop and disabled-send
  controls.
- Stop waits for the external agent's cancellation acknowledgement, shows an
  in-progress state, and refreshes the latest interaction before becoming
  clickable again. A turn cancelled while the desktop is still booting is
  marked interrupted before dispatch, so it cannot be picked up later.
- Pending uploads disable send. Failed uploads remain visible and must be
  retried or removed, preventing silent attachment loss.
- Uploading to a stopped external-agent session starts its desktop and waits
  for the desktop bridge before streaming the file. This preserves the normal
  paste-then-send flow after idle shutdowns.
- Up to ten files and 500 MB per file are accepted, matching the desktop
  bridge's upload ceiling.
- `open_file_manager=false` is forwarded through the API proxy, so pasting into
  chat does not steal focus by opening the desktop file manager.
- Multipart bodies are streamed through the API rather than read entirely into
  API memory.

## Cancellation verification

The CLI E2E creates a live spec-task sandbox, waits for a connected Zed thread
with an in-flight interaction, calls the same cancellation endpoint as the chat
button, and requires the interaction to become `interrupted`. It then sends a
normal follow-up and requires it to complete, covering the lifecycle seam after
cancellation.
