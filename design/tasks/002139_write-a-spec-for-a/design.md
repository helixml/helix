# Design: WidgetSync — Sync Widget Configurations Between Devices over WiFi

## Architecture

WidgetSync runs as a lightweight peer-to-peer sync service using mDNS for device discovery and HTTP for config exchange. No central server is required.

```
Device A                          Device B
┌─────────────────────┐           ┌─────────────────────┐
│  Frontend (React)   │           │  Frontend (React)   │
│  widget config      │           │  widget config      │
│        │            │           │        │            │
│  WidgetSync hook    │           │  WidgetSync hook    │
└────────┬────────────┘           └────────┬────────────┘
         │ REST/SSE                         │ REST/SSE
┌────────▼────────────┐  mDNS/LAN  ┌───────▼─────────────┐
│  widgetsync-daemon  │◄──────────►│  widgetsync-daemon  │
│  (Go, port 7742)    │  HTTP sync  │  (Go, port 7742)    │
└─────────────────────┘           └─────────────────────┘
```

## Components

### 1. widgetsync-daemon (Go)
A small HTTP server (port 7742) alongside the existing settings-sync-daemon pattern in `api/cmd/`. Responsibilities:
- Advertise itself via mDNS (`_widgetsync._tcp.local`)
- Discover peers on the same LAN
- Expose `GET /config` and `POST /config` endpoints
- Push config deltas to peers via HTTP POST on change
- Persist last-known configs to a local JSON file (`~/.config/helix/widgetsync.json`)

### 2. Frontend hook (`useWidgetSync`)
A React hook wrapping the daemon's local REST API:
- Reads/writes widget config through the daemon's localhost endpoint
- Subscribes to SSE stream for incoming sync events
- Exposes `syncNow()`, `lastSynced`, and `status` to UI components

### 3. Settings UI
New section in Settings → "WidgetSync":
- Toggle sync on/off globally
- Per-widget-type enable/disable checkboxes
- "Sync now" button
- Last-synced timestamp + active peers count

## Key Decisions

**mDNS over a cloud relay** — satisfies the requirement that no data leaves the LAN. Uses the standard `github.com/grandcat/zeroconf` library (already used elsewhere in the Go ecosystem).

**Last-write-wins with vector timestamps** — each config entry carries a `updatedAt` (RFC3339 nanosecond). On conflict, the newer timestamp wins. Simple and sufficient for single-user multi-device scenarios.

**HTTP push, not pull** — the daemon pushes diffs to known peers immediately on change, falling back to a 30-second polling pull for peers that missed a push. Keeps latency low without requiring a persistent connection.

**Daemon reuses existing pattern** — mirrors `api/cmd/settings-sync-daemon/main.go` structure so the implementation is familiar and the same Dockerfile/systemd unit patterns apply.

## Data Model

```json
{
  "version": 1,
  "deviceId": "uuid-v4",
  "widgets": {
    "viewMode": { "value": "grid", "updatedAt": "2026-06-19T10:00:00.000Z" },
    "tableColumns": { "value": {...}, "updatedAt": "2026-06-19T09:55:00.000Z" }
  }
}
```

## Constraints

- Daemon must start/stop cleanly alongside the helix desktop app.
- mDNS discovery scope is limited to the local subnet (TTL=1).
- Widget config payload must stay under 64 KB (no binary assets, only JSON prefs).
