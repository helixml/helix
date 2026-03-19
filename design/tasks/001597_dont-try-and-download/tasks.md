# Implementation Tasks

- [ ] In `useSandboxState` (`ExternalAgentDesktopViewer.tsx`), add a `stopped` ref that is set to `true` when the fetched session state is `"stopped"` or `desired_state === "stopped"` (i.e. `sandboxState` maps to `"absent"`)
- [ ] Once `stopped` ref is true, skip further polling (clear the interval / return early from `fetchState`) so no more `GET /api/v1/sessions/{id}` calls are made for that card
- [ ] Reset the `stopped` ref if `sessionId` or `enabled` changes (i.e. if the user starts the desktop and a new session is created, the hook re-runs and polls again)
- [ ] Verify on the Kanban board that stopped tasks make at most 1 session request (the initial check) and then stop
- [ ] Verify that actively running desktops still poll at 3s as before
