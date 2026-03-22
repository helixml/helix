# Design: Fix Flaky First Paste

## Root Cause

**File**: `frontend/src/components/external-agent/DesktopStreamViewer.tsx`

The keydown handler is registered in a `useEffect` at line 3706 with dependencies `[isConnected, resetInputState]`:

```typescript
useEffect(() => {
  if (!isConnected || !containerRef.current) return;
  // ...
  const handleKeyDown = (event: KeyboardEvent) => {
    // ...
    if (isPasteKeystroke && sessionId) {  // <-- sessionId captured from outer scope
      event.preventDefault();
      // ... sync clipboard and forward Ctrl+V ...
    }
    // FALLTHROUGH: sends raw event via custom WebSocket protocol
    getInput()?.onKeyDown(event);
    event.preventDefault();
  };
  container.addEventListener("keydown", handleKeyDown);
}, [isConnected, resetInputState]);  // <-- sessionId NOT in deps!
```

**`sessionId` is missing from the dependency array.** When `isConnected` first becomes true, the effect runs and the closure captures `sessionId` at that moment. If the session data hasn't finished loading yet, `sessionId` is `undefined` in the closure.

When `sessionId` is falsy:
- `if (isPasteKeystroke && sessionId)` is false → paste block is skipped
- The raw Ctrl+V event falls through to `getInput()?.onKeyDown(event)` at line 4054
- `getInput()` returns a `StreamInput` from the custom WebSocket stream; depending on how it serializes the event, the ctrl modifier may be dropped, producing a literal "v"

Later the session loads and `sessionId` is set, but since `isConnected` didn't change, the effect doesn't re-run. However, a re-render triggered by other state changes can cause the effect to re-run naturally (or component unmounts/remounts) — explaining why subsequent presses often work.

## Fix

**Option A (recommended)**: Keep a ref for `sessionId` so the closure always reads the current value without needing to re-register the listener:

```typescript
const sessionIdRef = useRef(sessionId);
useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

// In handleKeyDown, replace `sessionId` with `sessionIdRef.current`
if (isPasteKeystroke && sessionIdRef.current) { ... }
apiClient.v1ExternalAgentsClipboardCreate(sessionIdRef.current, payload)
```

**Option B (simpler)**: Add `sessionId` to the `useEffect` deps array. This re-registers the listener whenever `sessionId` changes, which is acceptable overhead:

```typescript
}, [isConnected, resetInputState, sessionId]);
```

Option A is preferred — it avoids briefly removing and re-adding event listeners when `sessionId` changes, and is consistent with the pattern used for `getInput()` (which also reads via a ref).

## Secondary Issue to Investigate

The event listener is registered without `{ capture: true }`:
```typescript
container.addEventListener("keydown", handleKeyDown);  // bubbling only
```

If the VNC/noVNC input handler is registered in the capture phase on the same element, it would process the Ctrl+V **before** our paste handler even runs, potentially sending "v" to the remote regardless. Adding `{ capture: true }` would ensure our handler runs first. The keyup listener at line 4141 should also use capture for symmetry.

## Key Files

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — all changes here
  - Line 3706: `useEffect` with keyboard handler registration
  - Line 3892: `if (isPasteKeystroke && sessionId)` — the stale closure condition
  - Line 4151: deps array `[isConnected, resetInputState]` — missing `sessionId`
  - Line 4140: `container.addEventListener("keydown", handleKeyDown)` — missing capture option

## Codebase Patterns

- The component uses `streamRef` (a ref) to access the current VNC stream without stale closures — same pattern should apply to `sessionId`
- `helixApi.getApiClient()` is called inside the closure (fresh each time) — no stale closure issue there
- The copy handler (Ctrl+C, lines ~3762-3881) likely has the same `sessionId` stale closure bug
