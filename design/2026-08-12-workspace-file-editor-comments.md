# Workspace file editing and review comments

## Context

The Workspace Inspector could browse and render files but could not make small edits or attach line-level feedback to the agent chat. T3 Code's implementation established the interaction model: select lines in the file, render a local annotation composer, and serialize accepted comments as `review_comment` blocks only when the user sends the chat message.

## Design

- Text files that are already allowed by the workspace browser are editable with Pierre's editor. Binary, ignored, and truncated files stay read-only.
- Saves are explicit through the toolbar or `Ctrl/Cmd+S`. The browser sends the hash of the opened content; the desktop bridge returns `409 Conflict` instead of overwriting an agent edit when the hash is stale.
- The desktop bridge writes through a same-directory temporary file and atomic rename while preserving file permissions.
- Accepted line comments live in the active task-session composer state. The visible chat draft remains clean; comments are converted to XML-like `review_comment` blocks at queue/send time and cleared only after the message is accepted.
- Comment draft text is local to the annotation component. Keeping it out of Pierre's annotation state prevents each keystroke from replacing the annotation node, losing textarea focus, and returning input to the file editor.
- Task creation uses the toolbar or `Ctrl/Cmd+Enter` while the project board is active. The listener runs in the capture phase so nested UI cannot swallow the shortcut, but still inspects the full event path to avoid firing from editable controls. The previous window-level bare-Enter shortcuts were unsafe for editors: shadow-DOM keyboard events are retargeted to the custom-element host, so checks against the inner `contenteditable` cannot reliably protect text input. `Ctrl/Cmd+T` is not used because browsers reserve it for opening a tab.
- Desktops started from an image built before the write route return `405 Method Not Allowed`. The editor keeps the draft and explains that the user must copy it, then stop and start the desktop to load the current image.

## Verification

- Desktop tests cover two consecutive saves, permission preservation, stale-content conflicts, and ignored/oversized file rejection.
- Server tests cover authorization of reads and writes.
- Frontend tests cover generated-client updates, comment serialization, queued prompts, stable comment focus, and multiline SpecTask descriptions.
- Live inner-Helix verification covered multiline task entry and character-by-character comment entry without modifying the underlying file.
- A current `helix-ubuntu:9f8ee8` desktop completed a write followed by a restoring write through the authenticated workspace-file API, both with HTTP 200. The board shortcut opened the new-task form, while the same event on the Audit view did nothing.
