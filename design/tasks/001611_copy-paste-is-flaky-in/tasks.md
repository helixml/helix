# Implementation Tasks

- [ ] In `DesktopStreamViewer.tsx`, add a `sessionIdRef` ref that stays up-to-date: `const sessionIdRef = useRef(sessionId)` + `useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId])`
- [ ] Replace all uses of `sessionId` inside the keyboard `useEffect` closure (lines ~3892, ~3996) with `sessionIdRef.current`
- [ ] Check the copy (Ctrl+C) handler in the same `useEffect` for the same stale-closure issue and apply the same fix
- [ ] Change `container.addEventListener("keydown", handleKeyDown)` to use `{ capture: true }` so our paste interceptor runs before any VNC handler
- [ ] Change `container.addEventListener("keyup", handleKeyUp)` similarly to `{ capture: true }` (update the `removeEventListener` calls to match)
- [ ] Test: open a desktop session, immediately press Ctrl+V on first focus — confirm clipboard content is pasted, no spurious "v" appears
