# Requirements: Fix dark-on-dark text in message queue header in light mode

## Problem

In the prompt input's message queue header, the text "Message queue (saved locally)"
(and its sibling strings "Editing - paused from here" and "Offline - saved locally,
will send when connected") renders as dark text on a dark background when the app
is in **light mode**, making it effectively unreadable.

The header `Box` has its background set to `primary.dark` / `info.dark` /
`warning.dark` (palette tokens that resolve to dark shades in both modes), but the
nested `<Typography>` and lucide icons inherit the default body text color — which
is dark in light mode.

## User stories

- **As a user on a light-themed Helix instance**, I want the message-queue
  header text to be legible so I can see what state my queue is in without
  switching to dark mode.

## Acceptance criteria

- In **light mode**, the queue header label and its leading icon (`Cloud` /
  `CloudOff` / `CirclePause`) are clearly readable against the dark
  `primary.dark` / `warning.dark` / `info.dark` background.
- The fix applies to all three states:
  - online + not editing → `"Message queue (saved locally)"` on `primary.dark`
  - offline → `"Offline - saved locally, will send when connected"` on `warning.dark`
  - editing → `"Editing - paused from here"` on `info.dark`
- The existing **dark mode** appearance is unchanged (the bug is light-mode-only,
  so the dark-mode regression risk must be verified, not assumed).
- The count `<Chip>` next to the label remains legible in both modes.

## Out of scope

- No restyling of the queue items themselves (the rows under the header).
- No change to the header background colors — only the foreground color is wrong.
- No change to the offline/online detection logic.
