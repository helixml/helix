# Design: Port Exposure UI

## Architecture

Add a port exposure dialog accessible from the `DesktopStreamViewer` toolbar. Uses existing API endpoints and generated TypeScript client.

## Component Structure

```
DesktopStreamViewer.tsx (existing)
  └── Toolbar (existing)
       └── [New] Port exposure button (icon: Wifi or Share)
            └── PortExposureDialog.tsx (new component)
                 ├── List of exposed ports (from GET /expose)
                 ├── Form to expose new port
                 └── Delete buttons for each port
```

## Key Decisions

### 1. Location: Desktop Stream Toolbar
Port exposure is session-specific and only relevant when viewing the desktop stream. Adding it to the existing toolbar keeps related controls together.

### 2. Dialog vs Drawer
Use a simple `Dialog` (MUI) rather than a slide-out drawer. This matches existing patterns in the codebase (e.g., `ClaudeSubscription.tsx` uses dialogs for connect/delete flows).

### 3. React Query for State
Use React Query (`useQuery`/`useMutation`) for fetching and mutating exposed ports. This provides:
- Automatic refetching
- Loading/error states
- Cache invalidation after mutations

### 4. Generated API Client
Use the existing generated methods from `api.ts`:
- `api.v1SessionsExposeDetail(sessionId)` - List ports
- `api.v1SessionsExposeCreate(sessionId, request)` - Expose port
- `api.v1SessionsExposeDelete(sessionId, port)` - Unexpose port

## UI Flow

1. User clicks port icon in toolbar → Dialog opens
2. Dialog shows list of currently exposed ports (if any)
3. User enters port number (required) and optional name
4. User clicks "Expose" → API call → List refreshes → URL shown
5. User can copy URL with clipboard button
6. User can click delete icon to unexpose a port

## Data Types (Already Generated)

```typescript
interface ServerExposePortRequest {
  port?: number;
  protocol?: string;  // defaults to "http"
  name?: string;
}

interface ServerExposedPort {
  port?: number;
  url?: string;
  name?: string;
  status?: string;
  created_at?: string;
}
```

## File Changes

| File | Change |
|------|--------|
| `frontend/src/components/external-agent/PortExposureDialog.tsx` | New component |
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Add button + dialog state |