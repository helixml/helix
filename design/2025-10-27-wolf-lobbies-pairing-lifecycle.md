# Wolf Lobbies Mode: Pairing Lifecycle and Multi-Client Architecture

**Date**: 2025-10-27
**Status**: Root cause identified, fix in progress
**Context**: Investigating HostNotPaired error in lobbies mode while apps mode works correctly

## Problem Statement

When creating external agent sessions in **lobbies mode**, moonlight-web returns `HostNotPaired` error immediately when user tries to connect to stream. However, in **apps mode**, pairing works automatically on first session creation. Both modes use the same Wolf and moonlight-web instances with identical environment variables (`MOONLIGHT_INTERNAL_PAIRING_PIN=1234`).

## Architecture Overview

### Moonlight-Web Pairing Model

Moonlight-web manages a **single Wolf host** (host_id=0) with pairing state persisted in `/app/server/data.json`:

```json
{
  "hosts": [
    {
      "address": "wolf",
      "http_port": 47989,
      "unique_id": null,
      "cache": {...},
      "paired": {
        "client_private_key": "-----BEGIN PRIVATE KEY-----...",
        "client_certificate": "-----BEGIN CERTIFICATE-----...",
        "server_certificate": "-----BEGIN CERTIFICATE-----..."
      }
    }
  ]
}
```

**Key insight**: There is only ONE host object in moonlight-web, regardless of mode (apps or lobbies). The pairing state is global for all streaming sessions to Wolf.

### Pairing State Lifecycle

1. **On moonlight-web startup** (`data.rs:102-156`):
   - Load hosts from `data.json`
   - For each host, call `set_pair_state()` to restore pairing info
   - If `paired` section exists, parse PEM certificates and call `host.set_pairing_info()`
   - Call `host.verify_paired()` to validate pairing with Wolf

2. **When stream connection is requested** (`stream.rs:189`):
   - Check `if host.is_paired() == PairStatus::NotPaired`
   - If not paired, send `HostNotPaired` error and close connection
   - **This check happens BEFORE any session/lobby logic**

3. **Pairing API endpoint** (`/api/pair`):
   - Called explicitly by Helix API to establish pairing
   - Generates new client auth (certificate pair)
   - Uses `MOONLIGHT_INTERNAL_PAIRING_PIN` env var (1234) if set
   - Calls `host.moonlight.pair()` with Wolf
   - On success, triggers `file_writer` to save pairing state to `data.json`

### Apps Mode vs Lobbies Mode

#### Apps Mode (wolf_executor_apps.go)

**Pairing Flow**:
```go
func (w *WolfExecutor) StartZedAgent(...) {
    // Line 177-193: Auto-pair Wolf with moonlight-web
    if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
        log.Warn().Err(err).Msg("Auto-pairing failed")
    }

    // Then create Wolf app...
}
```

**ensureWolfPaired** (line 1197):
1. Calls `POST /api/pair` on moonlight-web
2. Reads PIN from NDJSON response stream
3. Wolf auto-accepts because `MOONLIGHT_INTERNAL_PAIRING_PIN=1234` is set
4. Pairing completes and is saved to `data.json`
5. All subsequent streaming connections work because `host.is_paired()` returns `Paired`

#### Lobbies Mode (wolf_executor.go)

**Current Flow** (BROKEN):
```go
func (w *WolfExecutor) StartZedAgent(...) {
    // Line 287-436: Create lobby directly
    lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)

    // ❌ NO PAIRING CALL - HostNotPaired error occurs!
}
```

**Why it's broken**:
- `ensureWolfPaired()` is **never called** in lobbies mode
- Moonlight-web never pairs with Wolf
- `data.json` has no `paired` section
- `host.is_paired()` returns `NotPaired`
- Stream connection fails immediately with `HostNotPaired` error

### Multi-Client Confusion (Clarified)

**Initial hypothesis** (INCORRECT): Apps mode creates per-session clients, lobbies mode uses a different client model.

**Reality**:
- Both modes use the **same single Wolf host** in moonlight-web
- There is NO per-session client creation
- Pairing is global for the Wolf instance, not per-session
- The difference is that **apps mode triggers pairing, lobbies mode doesn't**

### Why Fresh Startup Works

When both Wolf and moonlight-web start from clean state (no `config.toml`, no `data.json`):
1. Wolf generates new server certificate
2. Moonlight-web has empty `data.json` (no pairing info)
3. First stream attempt triggers pairing protocol
4. Auto-pairing PIN (1234) works
5. Both services save state

**But this only works if pairing is triggered!** In lobbies mode, nothing triggers it.

## Root Cause

**The bug**: `wolf_executor.go` (lobbies mode) never calls `ensureWolfPaired()`.

**Location**: `api/pkg/external-agent/wolf_executor.go:287` (StartZedAgent function)

**Impact**:
- External agent sessions in lobbies mode cannot be streamed
- Personal dev environments in lobbies mode cannot be streamed
- Users see HostNotPaired error in UI immediately

## Solution

Add pairing call to lobbies mode executor, matching apps mode pattern:

```go
func (w *WolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
    // ... existing code ...

    // AUTO-PAIR: Ensure Wolf is paired with moonlight-web before creating lobby
    moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
    if moonlightWebURL == "" {
        moonlightWebURL = "http://moonlight-web:8080"
    }
    credentials := os.Getenv("MOONLIGHT_CREDENTIALS")

    if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
        log.Warn().
            Err(err).
            Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
    }

    // Then create lobby...
    lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)
    // ...
}
```

**Same fix needed in**:
- `StartZedAgent` (external agents) - line 287
- `CreatePersonalDevEnvironmentWithDisplay` (PDEs) - line 675
- `recreateLobbyForPDE` (PDE reconciliation) - line 1456

## Code References

**Moonlight-web pairing check**: `/home/luke/pm/moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs:189`

**Apps mode pairing**: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor_apps.go:190`

**Lobbies mode (missing pairing)**: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go:287`

**ensureWolfPaired implementation**: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor_apps.go:1197`

## Testing Plan

1. Stop services: `./stack stop`
2. Clean pairing state (already automatic in `./stack start`)
3. Start services: `./stack start`
4. Create external agent session in lobbies mode
5. Click "Live Stream" button
6. Verify Wolf UI loads and shows available lobby
7. Enter PIN and verify streaming works

## Historical Context

This bug was introduced when migrating from apps mode to lobbies mode. The `ensureWolfPaired()` function exists in `wolf_executor_apps.go` but was not copied to `wolf_executor.go` (lobbies implementation).

The auto-cleanup added to `./stack start` masks this issue temporarily - if both services start fresh, the first manual pairing trigger (via browser or API call) establishes pairing. But this is fragile and doesn't work reliably when Wolf restarts independently.
