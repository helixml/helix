# Design: Add Dedicated RevDial Connectivity Ping Endpoint

## Architecture

The ping follows the same path as the existing screenshot flow, but replaces the heavy image-capture step with a trivial HTTP response.

```
helix CLI
  └─ GET /api/v1/external-agents/{session-id}/ping
       └─ API server: handlePing()
            └─ connman.Dial("desktop-{session-id}")   ← RevDial tunnel
                 └─ GET http://localhost:9876/ping     ← inside container
                      └─ {"status":"ok"}
```

## Key Files

| File | Change |
|------|--------|
| `api/pkg/desktop/desktop.go` | Register `GET /ping` → `{"status":"ok","session_id":"..."}` |
| `api/pkg/server/external_agent_handlers.go` | Add `handlePing()`, register `GET /external-agents/{id}/ping` |
| `api/pkg/cli/spectask/spectask.go` | Add `newPingCommand()` cobra subcommand |

## Implementation Notes

**Desktop server (`/ping`):** A one-liner handler returning `application/json`. No auth needed — the endpoint is only reachable via RevDial from the API server, never from the public internet (same as existing sandbox handlers; see `api/pkg/hydra/sandbox_handlers.go` header comment).

**API handler (`handlePing`):** Mirrors `handleScreenshot` in `external_agent_handlers.go` up to the `connman.Dial` call. Instead of forwarding query params and buffering image data, it sends `GET /ping` over the RevDial connection, reads the response, and proxies the status code + body back to the caller.

**CLI (`spectask ping`):** Calls `GET /api/v1/external-agents/{session-id}/ping` with a short timeout (5 s), prints latency on success, exits non-zero on failure.

## Decision: no new struct types

The ping response is a trivial JSON literal. Defining a dedicated struct would be over-engineering; both the desktop handler and the API handler can write inline JSON (`{"status":"ok"}`).
