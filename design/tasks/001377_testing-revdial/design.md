# Design: Testing RevDial Connectivity

## Architecture Overview

```
┌─────────────┐     WebSocket      ┌─────────────┐     RevDial      ┌──────────────────┐
│   Browser   │ ◄─────────────────► │  Helix API  │ ◄───────────────► │ Sandbox Container │
│  or CLI     │                     │  (connman)  │                   │ (Hydra + desktop) │
└─────────────┘                     └─────────────┘                   └──────────────────┘
```

**Key Components:**
- `api/pkg/revdial/` - RevDial client and server implementation
- `api/pkg/connman/` - Connection manager with grace period support
- `api/cmd/hydra/` - Sandbox process that establishes RevDial connection
- `api/pkg/cli/spectask/` - CLI commands for testing

## How RevDial Works

1. **Sandbox → API**: Hydra (in sandbox) opens WebSocket to `/api/v1/revdial?runnerid=hydra-{sandbox_id}`
2. **Control channel**: JSON messages over WebSocket (`keep-alive`, `conn-ready`, `pickup-failed`)
3. **Data connections**: When API needs to dial sandbox, it sends `conn-ready` → sandbox opens new WebSocket for data
4. **connman**: Stores `revdial.Dialer` per runner ID, allows `Dial(ctx, key)` to reach sandbox services

## Testing Strategy

### Manual Testing (CLI)

```bash
# Start a session (creates sandbox with RevDial)
helix spectask start --project $HELIX_PROJECT -n "test"

# Wait for sandbox to connect (~10-15 seconds)

# Test screenshot (verifies full pipeline)
helix spectask screenshot ses_xxx
```

### Automated Testing

Use existing `helix spectask test --session <id> --desktop` which runs:
1. `testScreenshot()` - GET `/api/v1/external-agents/{id}/screenshot`
2. `testDesktopTool()` - Tests `list_windows`, `read_screen`, `type_text`

### What the Tests Validate

| Test | What It Validates |
|------|-------------------|
| Screenshot | RevDial → desktop-bridge → GStreamer → JPEG pipeline |
| list_windows | RevDial → desktop-bridge → D-Bus → GNOME Shell |
| type_text | RevDial → desktop-bridge → D-Bus → input injection |

## Key Design Decisions

1. **Screenshot as primary test**: Screenshot exercises the most complex path (video capture) and is a good proxy for overall health
2. **15-second timeout**: Matches existing CLI behavior; long enough for slow containers, short enough to fail fast
3. **Grace period (30s)**: connman queues Dial() calls during brief disconnections; tests should retry within this window

## Debugging Patterns

**Check RevDial connection state:**
```bash
docker compose -f docker-compose.dev.yaml logs api | grep -E "revdial|connman"
```

**Check sandbox side:**
```bash
docker compose exec -T sandbox-nvidia docker logs $(docker compose exec -T sandbox-nvidia docker ps -q --filter name=ubuntu-external | head -1) 2>&1 | grep -i revdial
```

## Existing Code to Reuse

- `api/pkg/cli/spectask/test_cmd.go` - `testScreenshot()`, `testDesktopTool()` functions
- `api/pkg/cli/spectask/spectask.go` - `newScreenshotCommand()` 
- `api/pkg/connman/connman.go` - `Stats()` method for connection health

## Out of Scope

- Load testing / stress testing RevDial
- Automated reconnection testing (would require killing/restarting Hydra)
- Network partition simulation