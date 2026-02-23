# Design: Testing RevDial Connectivity

## Overview

Add a `helix spectask revdial <session-id>` command to verify RevDial tunnel connectivity for a given session.

## Architecture

```
CLI Command                    API Server                    Sandbox Container
    │                              │                              │
    │ GET /api/v1/sessions/{id}    │                              │
    │──────────────────────────────>│                              │
    │ (verify session exists)       │                              │
    │<──────────────────────────────│                              │
    │                              │                              │
    │ GET /external-agents/{id}/screenshot                        │
    │──────────────────────────────>│                              │
    │                              │ Dial via RevDial tunnel       │
    │                              │──────────────────────────────>│
    │                              │<──────────────────────────────│
    │<──────────────────────────────│ (screenshot bytes)           │
    │                              │                              │
```

## Key Decisions

### Reuse Existing Screenshot Endpoint
The screenshot endpoint (`/api/v1/external-agents/{id}/screenshot`) already tests the full RevDial path:
- API receives request
- API looks up desktop container's runner ID (`desktop-{session_id}`)
- API calls `connman.Dial()` to get a connection through the RevDial tunnel
- Request is proxied to the desktop container's screenshot server

If screenshot works, RevDial works. No new endpoints needed.

### Connection Status via Session Lookup
The session object includes `sandbox_status` which indicates if the container is running. Combined with a successful screenshot, this confirms RevDial connectivity.

## Implementation

Location: `helix/api/pkg/cli/spectask/revdial_cmd.go`

```go
func newRevDialCommand() *cobra.Command {
    // 1. Parse session ID arg
    // 2. GET session to verify it exists and is running
    // 3. GET screenshot to test RevDial tunnel
    // 4. Report results (human or JSON based on --json flag)
}
```

## Patterns Found

- **Existing test pattern**: `testScreenshot()` in `test_cmd.go` already tests screenshot endpoint
- **CLI pattern**: All spectask commands use `getAPIURL()` and `getToken()` helpers
- **Output pattern**: `--json` flag with `TestResult` struct for machine output

## Constraints

- Requires a running session with an active desktop container
- Session must have RevDial connected (hydra → API tunnel established)
- Timeout should be reasonable (10-15s) to account for cold start scenarios