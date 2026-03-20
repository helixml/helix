# Fix flaky first paste (Ctrl+V types "v" on first press)

## Summary

The first Ctrl+V paste in a desktop session would type a literal "v" instead of pasting. Subsequent presses worked fine.

**Root cause:** The keyboard event handler was registered in a `useEffect` with deps `[isConnected, resetInputState]`. The closure captured `sessionId` at registration time. If `sessionId` wasn't set yet when `isConnected` first became true (session still loading), the paste-intercept condition `if (isPasteKeystroke && sessionId)` evaluated to false, causing the raw key event to fall through to the VNC handler — which may drop the ctrl modifier and send a bare "v" to the remote.

A secondary risk: the keydown listener used bubbling phase, meaning a VNC handler in capture phase could process the event before our paste interceptor.

## Changes

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`
  - Add `sessionIdRef` (a ref kept in sync with the `sessionId` prop via a dedicated `useEffect`) so the keyboard handler closure always reads the current session ID without needing to be re-registered
  - Replace direct `sessionId` references inside the keyboard `useEffect` closure (copy and paste handlers) with `sessionIdRef.current`
  - Register keydown/keyup listeners with `{ capture: true }` so our clipboard interceptor runs before any VNC input handler
