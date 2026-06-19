# Implementation Tasks: WidgetSync — Sync Widget Configurations Between Devices over WiFi

- [ ] Create `api/cmd/widgetsync-daemon/main.go` with HTTP server on port 7742 (`GET /config`, `POST /config`, `GET /events` SSE)
- [ ] Implement mDNS advertisement and peer discovery using `github.com/grandcat/zeroconf`
- [ ] Implement last-write-wins merge logic with `updatedAt` nanosecond timestamps
- [ ] Persist widget config to `~/.config/helix/widgetsync.json` with atomic file writes
- [ ] Add HTTP push to peers on config change; add 30-second polling fallback
- [ ] Add `useWidgetSync` React hook in `frontend/src/hooks/useWidgetSync.ts` (wraps daemon localhost REST + SSE)
- [ ] Wire `useWidgetSync` into existing widget components (`ViewModeToggle`, `SimpleTable`, `CardGrid`)
- [ ] Add WidgetSync section to Settings page: global toggle, per-type checkboxes, "Sync now" button, last-synced + peer count
- [ ] Show toast notification when a conflict is auto-resolved (last-write-wins applied)
- [ ] Add daemon startup/shutdown to the helix desktop app lifecycle (alongside settings-sync-daemon)
- [ ] Write unit tests for merge logic and HTTP handler
- [ ] Write integration test: two daemon instances exchange a config update over loopback
