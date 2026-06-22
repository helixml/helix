# Implementation Tasks: Add Dedicated RevDial Connectivity Ping Endpoint

- [ ] Add `GET /ping` handler in `api/pkg/desktop/desktop.go` that returns `{"status":"ok"}` with `Content-Type: application/json`
- [ ] Register the `/ping` route on the desktop HTTP server (port 9876) alongside existing routes
- [ ] Add `handlePing()` in `api/pkg/server/external_agent_handlers.go` that dials the RevDial tunnel and proxies `/ping` from the desktop server
- [ ] Register `GET /api/v1/external-agents/{id}/ping` route in the server router (same auth middleware as screenshot)
- [ ] Add `newPingCommand()` cobra subcommand in `api/pkg/cli/spectask/spectask.go` with a 5 s timeout that prints latency on success and exits non-zero on failure
- [ ] Register `ping` under the `spectask` root command
- [ ] Write a unit test for `handlePing()` that verifies `503` when no RevDial connection exists
