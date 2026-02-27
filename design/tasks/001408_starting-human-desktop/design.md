# Design: Desktop Startup Progress Indicators

## Overview

Add granular progress messages during desktop session startup to replace the generic "Starting Desktop..." spinner. The backend already has a `status_message` field that's polled by the frontend—we just need to populate it at more stages.

## Current Architecture

### Startup Flow
1. **API receives resume/start request** → sets `external_agent_status = "starting"`
2. **Hydra creates container** → currently no status message
3. **Golden cache copy** → already sends "Unpacking build cache (X/Y GB)" via `updateSessionStatusMessage()`
4. **Container starts** → currently no status message
5. **Wait for desktop-bridge** → polls for up to 60 seconds, no status message
6. **Desktop ready** → clears status message, sets `external_agent_status = "running"`

### Existing Infrastructure
- `session.Metadata.StatusMessage` — transient string field in DB
- `updateSessionStatusMessage(ctx, sessionID, msg)` — helper to update it
- Frontend polls `/api/v1/sessions/{id}` every 3 seconds, reads `config.status_message`
- UI already displays `statusMessage || "Starting Desktop..."`

## Design

### Backend Changes (hydra_executor.go)

Add status messages at each stage in `StartDesktop()`:

```go
// 1. Before Hydra call
h.updateSessionStatusMessage(ctx, agent.SessionID, "Creating container...")

// 2. Golden cache progress (already exists)
msg := fmt.Sprintf("Unpacking build cache (%.1f/%.1f GB)", copiedGB, totalGB)

// 3. After container created, before bridge wait
h.updateSessionStatusMessage(ctx, agent.SessionID, "Starting desktop environment...")

// 4. Inside waitForDesktopBridge loop (every 10 attempts = 10 seconds)
h.updateSessionStatusMessage(ctx, agent.SessionID, "Connecting to desktop...")

// 5. On completion (already exists)
h.updateSessionStatusMessage(ctx, agent.SessionID, "")
```

### Frontend Changes (ExternalAgentDesktopViewer.tsx)

Add elapsed time display after 5 seconds:

```tsx
// In useSandboxState hook, add startTime tracking
const [startTime, setStartTime] = useState<number | null>(null);

// When isStarting becomes true, record start time
useEffect(() => {
  if (isStarting && !startTime) setStartTime(Date.now());
  if (!isStarting) setStartTime(null);
}, [isStarting]);

// Compute elapsed seconds for display
const elapsedSeconds = startTime ? Math.floor((Date.now() - startTime) / 1000) : 0;

// Display: "Starting desktop environment... (12s)" after 5 seconds
const displayMessage = statusMessage || "Starting Desktop...";
const withElapsed = elapsedSeconds >= 5 
  ? `${displayMessage} (${elapsedSeconds}s)` 
  : displayMessage;
```

## Key Decisions

1. **No percentage/progress bar** — Stages have unpredictable durations; discrete messages are more honest.

2. **Elapsed time only after 5s** — Avoids visual noise for fast startups; helps calibrate expectations for slow ones.

3. **Reuse existing polling** — Frontend already polls every 3 seconds; no new API endpoints needed.

4. **Messages clear automatically** — Backend clears `status_message` when startup completes or fails.

## Files to Modify

- `api/pkg/external-agent/hydra_executor.go` — Add status messages at startup stages
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — Add elapsed time display