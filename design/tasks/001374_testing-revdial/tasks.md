# Implementation Tasks

## Unit Tests for revdial Package

- [ ] Create `api/pkg/revdial/revdial_test.go`
- [ ] Test Dialer/Listener handshake using `net.Pipe()` in-memory connections
- [ ] Test control message protocol (keep-alive, conn-ready, pickup-failed)
- [ ] Test Dialer.Dial() returns connection from Listener
- [ ] Test Listener timeout when server goes silent (60s controlReadTimeout)
- [ ] Test Dialer.Close() properly cleans up and unregisters

## CLI Connectivity Test

- [ ] Add `test-revdial` subcommand to `api/pkg/cli/spectask/`
- [ ] Accept session ID or sandbox ID as argument
- [ ] Query connman to check if sandbox has active RevDial connection
- [ ] Attempt to dial through RevDial and measure round-trip latency
- [ ] Display clear success/failure message with diagnostics on error

## Health Check Endpoint

- [ ] Add `GET /api/v1/sandboxes/{id}/revdial-health` endpoint
- [ ] Return connection status (connected/disconnected/grace-period)
- [ ] Include `connected_since` and `last_activity` timestamps
- [ ] Optional `?ping=true` parameter to verify tunnel responsiveness

## Documentation

- [ ] Add RevDial troubleshooting section to `docs/content/helix/private-deployment/`
- [ ] Document common failure modes (proxy timeouts, TLS issues, NAT)
- [ ] Add example `test-revdial` usage to CLAUDE.md quick reference