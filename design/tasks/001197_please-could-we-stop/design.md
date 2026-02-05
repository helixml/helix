# Design: Stop Screenshot Requests for Stopped Sessions

## Overview

Stop screenshot polling when a session is known to be stopped, eliminating console spam from failed requests.

## Current Behavior

1. `DesktopStreamViewer.tsx` polls for screenshots in "screenshot" quality mode (low bandwidth mode)
2. `ScreenshotViewer.tsx` polls for screenshots in screenshot mode (used by Kanban cards)
3. `ExternalAgentDesktopViewer.tsx` fetches a static screenshot when showing the "paused" overlay

These components continue polling/fetching even when the session is stopped, resulting in repeated console warnings:
- `[DesktopStreamViewer] Screenshot fetch failed: 503`
- `[DesktopStreamViewer] Screenshot fetch error: ...`

## Root Cause

The screenshot polling effects don't check if the session is stopped before making requests. The `ScreenshotViewer` does have `sessionUnavailable` state that stops polling after a 503/404, but:
1. It still logs warnings before stopping
2. `DesktopStreamViewer` doesn't have this protection at all
3. The paused overlay in `ExternalAgentDesktopViewer` still makes a single request on render

## Solution

### Approach: Pass Stopped State to Viewers

Add a `sessionStopped` prop to the viewer components and skip polling when true.

### Key Files

1. **`ExternalAgentDesktopViewer.tsx`**: Already has `isPaused` from `useSandboxState`. Pass this to child viewers.

2. **`DesktopStreamViewer.tsx`**: Add `sessionStopped` prop. In the screenshot polling effect, return early if stopped.

3. **`ScreenshotViewer.tsx`**: Add `sessionStopped` prop. Skip polling when stopped.

### Code Changes

**DesktopStreamViewer.tsx** - screenshot polling effect (around L1695):
```typescript
// Add to props interface
sessionStopped?: boolean;

// In the screenshot polling useEffect
useEffect(() => {
  // Skip polling entirely if session is stopped
  if (sessionStopped) {
    return;
  }
  // ... existing polling logic
}, [shouldPollScreenshots, isConnected, sessionId, sessionStopped, ...]);
```

**ScreenshotViewer.tsx** - auto-refresh effect:
```typescript
// Add to props interface
sessionStopped?: boolean;

// In the auto-refresh useEffect (around L178)
useEffect(() => {
  if (!autoRefresh || streamingMode !== 'screenshot' || sessionUnavailable || sessionStopped) return;
  // ... existing logic
}, [..., sessionStopped]);
```

**ExternalAgentDesktopViewer.tsx** - pass prop to children:
```typescript
// In stream mode render
<DesktopStreamViewer
  sessionId={sessionId}
  sessionStopped={isPaused}
  // ... other props
/>

// In screenshot mode render  
<ScreenshotViewer
  sessionId={sessionId}
  sessionStopped={isPaused}
  // ... other props
/>
```

### Paused Overlay Static Screenshot

The paused overlay in `ExternalAgentDesktopViewer` fetches a screenshot to show a dimmed preview. This is intentional - it provides visual context for what the desktop looked like. Keep this single fetch (not a polling loop).

However, add `onError` handling to hide the image gracefully if it fails (which it already does with `e.currentTarget.style.display = 'none'`).

## Alternatives Considered

1. **Unmount viewers when stopped**: Would work but breaks the "keep mounted for fullscreen" requirement in stream mode.

2. **Check HTTP status in each request**: Already partially done in `ScreenshotViewer`, but still logs before stopping. Checking props is cleaner.

3. **Global context for session state**: Over-engineering for this simple fix.

## Testing

1. Start a spec task with a desktop session
2. Stop the session via the stop button
3. Verify no screenshot warnings appear in browser console
4. Resume the session
5. Verify screenshots work again (in screenshot mode)