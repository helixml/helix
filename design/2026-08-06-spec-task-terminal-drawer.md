# Spec task terminal drawer

## Goal

Make the running development container for a spec task accessible from both
spec-task routes as a resizable bottom terminal drawer. Browser refreshes must
reattach to the same shell and preserve the working directory and foreground
processes.

## Reference behavior

T3 Code's `ThreadTerminalDrawer` at commit
`e4abc31f1e3f930e521a5bb62b38a9c5b28d8fb1` uses a bottom panel with:

- a toolbar toggle and `Cmd/Ctrl+J` shortcut;
- a pointer-resizable height constrained to the viewport;
- per-thread persisted open, height, and selected-terminal state; and
- compact split, new-terminal, and close controls over the terminal surface;
- a terminal process that outlives the mounted browser surface.

Helix already provides the last two runtime primitives for user-created
sandboxes: xterm connects over an authenticated WebSocket, the API proxies the
PTY to Hydra, and the shell is attached through a named tmux session.

## Resource mapping

A spec task does not run in a user-created `types.Sandbox` row. Its active
Helix session identifies:

- the Hydra host in `Session.SandboxID`; and
- the development container in Hydra by `Session.ID`.

The existing organization sandbox route cannot be reused directly because it
loads a `types.Sandbox` row and uses that row's ID as the development-container
ID. Session terminal routes therefore authorize the session, dial
`hydra-<Session.SandboxID>`, and address the container by `Session.ID`.

## Design

1. Extract the common WebSocket/tmux bridge and tmux-session listing logic from
   the sandbox handlers. Both sandbox and session routes use the same helper.
2. Add `GET /api/v1/sessions/{id}/terminal`,
   `GET /api/v1/sessions/{id}/terminal/sessions`, and
   `DELETE /api/v1/sessions/{id}/terminal/sessions/{terminal_session}`.
   Opening or deleting a terminal requires session update access; listing
   sessions requires read access.
3. Extract a transport-independent persistent xterm component from
   `SandboxTerminal`. Thin sandbox and session wrappers supply their URL,
   session list, storage key, and unavailable-state message.
4. Mount a bottom drawer below `SpecTaskDetailContent`'s main content. Persist
   open state and height per task, constrain resizing to 180px–70% of the
   viewport, and refit xterm with a `ResizeObserver`.
5. Add a bottom-panel toggle to both desktop and compact task toolbars and bind
   `Cmd/Ctrl+J`. Switching the selected task thread remounts the terminal against
   that thread's session/container.
6. Start new task terminal sessions in `/home/retro/work`, where the development
   container mounts its checked-out repositories. Disable tmux's status line so
   terminal chrome is owned by the browser UI.
7. Persist terminal groups and panes per Helix session. Horizontal and vertical
   split actions create another independently persistent tmux session; the new
   terminal action creates a separate group; trash kills the active tmux session
   server-side before removing its pane.
8. Use the same neutral Lucide split, add, and trash controls as T3 Code. Task
   action icons share the same neutral color treatment. Task terminals use a
   compact `helix ~/work ❯` prompt instead of exposing the container hostname.
9. Keep standalone task content padded while allowing the terminal drawer to
   span the full task workspace width.
10. Store each terminal group as a nested split tree. Splitting replaces only
    the active pane with a two-pane node, so differently oriented splits can be
    composed without relaying out sibling terminals. Migrate the original flat
    version 1 layout in place and preserve its tmux session names.

## Lifecycle

The browser stores terminal groups, pane names, split direction, and the active
pane per Helix session. The API creates or attaches a named `helix-<name>` tmux
session inside that session's development container. Closing or refreshing the
page only closes the WebSocket/PTY client; tmux and its child processes remain.
Reopening the drawer or refreshing reuses the stored layout and reattaches each
visible pane. Trash explicitly kills the active tmux session through the API.
Exiting a shell with Ctrl+D removes its pane; exiting the last pane collapses the
drawer. Opening the drawer after all panes were closed creates a fresh terminal
session automatically.

Stopping or replacing the development container ends its tmux server. The UI
does not attempt to fake persistence across container replacement.

## Verification

- authorization and missing-Hydra-host handler tests;
- frontend state/shortcut tests and production build;
- Go server build/tests;
- live task page: open terminal, change directory, start a harmless process,
  refresh, and verify the same tmux session and shell state are restored;
- switch task thread and verify the terminal targets the selected session.
