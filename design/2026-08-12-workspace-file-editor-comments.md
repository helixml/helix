# Workspace file editing and review comments

## Context

The Workspace Inspector could browse and render files but could not make small edits or attach line-level feedback to the agent chat. T3 Code's implementation established the interaction model: select lines in the file, render a local annotation composer, and serialize accepted comments as `review_comment` blocks only when the user sends the chat message.

## Design

- Text files that are already allowed by the workspace browser are editable with Pierre's editor. Binary, ignored, and truncated files stay read-only.
- Saves are explicit through the toolbar or `Ctrl/Cmd+S`. The browser sends the hash of the opened content; the desktop bridge returns `409 Conflict` instead of overwriting an agent edit when the hash is stale.
- The desktop bridge writes through a same-directory temporary file and atomic rename while preserving file permissions.
- Accepted line comments live in the active task-session composer state. The visible chat draft remains clean; comments are converted to XML-like `review_comment` blocks at queue/send time and cleared only after the message is accepted.
- Comment draft text is local to the annotation component. Keeping it out of Pierre's annotation state prevents each keystroke from replacing the annotation node, losing textarea focus, and returning input to the file editor.
- Task creation remains an explicit toolbar action. The previous window-level bare-Enter shortcuts were unsafe for editors: shadow-DOM keyboard events are retargeted to the custom-element host, so checks against the inner `contenteditable` cannot reliably protect text input.

## Verification

- Desktop tests cover two consecutive saves, permission preservation, stale-content conflicts, and ignored/oversized file rejection.
- Server tests cover authorization of reads and writes.
- Frontend tests cover generated-client updates, comment serialization, queued prompts, stable comment focus, and multiline SpecTask descriptions.
- Live inner-Helix verification covered multiline task entry and character-by-character comment entry without modifying the underlying file.
