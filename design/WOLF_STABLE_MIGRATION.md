# Wolf Stable Branch Migration Plan

**Date:** 2025-10-13
**Context:** Reverting from wolf-ui (lobbies) back to stable (apps) due to complexity and reliability issues

## Current Problem

The wolf-ui branch with lobbies introduces significant complexity:
- Keepalive sessions failing with moonlight-web integration
- PIN management complexity across API restarts
- Multiple failure modes (pairing, PIN validation, WebSocket timing)
- Additional reconciliation loops needed
- Hard to debug multi-layer interactions

## Migration Strategy

### Wolf Repository Changes

**Current State:**
- Branch: `wolf-ui-working` (fork of upstream wolf-ui)
- Docker image: `ghcr.io/games-on-whales/wolf:wolf-ui`
- Introduced in Helix: commit `a9815fe09` (Oct 7, 2025)

**Target State:**
- Branch: `stable` with cherry-picked moonlight-web support
- Docker image: Build from local Wolf repo with custom tag
- Simpler apps/sessions model instead of lobbies

**Critical Commits to Preserve** (all by Luke Marsden):

From `wolf-ui-working` branch (not on stable):
1. `84d4c01` - "Add Phase 5 HTTP support for Moonlight pairing protocol"
   - Enables moonlight-web pairing to work
   - CRITICAL for moonlight-web integration

2. `57321eb` - "Add MOONLIGHT_INTERNAL_PAIRING_PIN auto-pairing support"
   - Reads `MOONLIGHT_INTERNAL_PAIRING_PIN` env var
   - Auto-fulfills pairing without manual PIN entry
   - CRITICAL for automated moonlight-web pairing

3. `c19557a` - "only auto-pair for docker local clients i.e moonlight-web-stream"
   - Security: Only auto-pairs for Docker internal IPs
   - Prevents auto-pairing for external clients

4. `82eecca` - "Add system memory debugging endpoint for Agent Sandboxes dashboard"
   - `/api/v1/system/memory` endpoint
   - Provides GStreamer buffer memory stats
   - Used by Agent Sandboxes dashboard

**Commits to SKIP** (wolf-ui hang fixes, not needed on stable):
- `1500016` - Prevent auto-leave on pause (lobby-specific)
- `a0acb44`, `ca2ad24`, `cf0f4af`, `9cb7bcf` - Lobby rejoin hang fixes
- `15eb3a9`, `29da4d0` - Diagnostic logging for wolf-ui hangs
- `307c3de`, `45339fe` - HTTP Phase 5 reverts (cancelled out)

**Commits to PORT MANUALLY** (after core migration stable):
- `82eecca` - Memory debugging endpoint
  - PORT NEEDED: Adapt from lobbies to apps model
  - Read /proc/self/status for RSS (unchanged)
  - Iterate apps instead of lobbies
  - Track per-app memory instead of per-lobby
  - Keep StreamSession tracking (unchanged)

### Helix Repository Changes

**Files to Modify:**

1. **docker-compose.dev.yaml**
   - Change Wolf image from ghcr.io/games-on-whales/wolf:wolf-ui → custom build
   - Add build context pointing to ../wolf

2. **api/pkg/wolf/client.go**
   - Remove lobby-specific types: `CreateLobbyRequest`, `StopLobbyRequest`, `JoinLobbyRequest`, `Lobby`
   - Remove methods: `CreateLobby()`, `StopLobby()`, `JoinLobby()`, `ListLobbies()`
   - Keep app methods: `AddApp()`, `RemoveApp()`, `ListApps()`
   - Keep session methods: `CreateSession()`, `StopSession()`

3. **api/pkg/external-agent/wolf_executor.go**
   - Remove all lobby code: `CreateLobby()` calls, keepalive sessions, reconciliation
   - Restore app-based approach: `AddApp()` → `CreateSession()` when client connects
   - Remove `generateLobbyPIN()`, keepalive WebSocket code
   - Simplify to: Create app → Wait for Moonlight client → Session auto-created by Wolf
   - Remove PIN fields from ZedSession struct

4. **api/pkg/types/types.go**
   - Remove `WolfLobbyID`, `WolfLobbyPIN` from types
   - Keep `WolfAppID` and `WolfSessionID` for app-based model

5. **frontend components**
   - Remove PIN display from SessionToolbar
   - Remove keepalive status indicators
   - Simplify streaming setup UI

**Mode Togglability: IMPLEMENTATION APPROACH**

Add environment variable: `WOLF_MODE=lobbies|apps` (default: `apps`)

**Architecture:**
```
Executor Interface (unchanged)
      ↑
      ├─→ LobbyWolfExecutor (current implementation, rename from WolfExecutor)
      │   - CreateLobby(), StopLobby(), JoinLobby()
      │   - Keepalive sessions via moonlight-web
      │   - PIN-based multi-tenancy
      │   - Complex but feature-rich
      │
      └─→ AppWolfExecutor (new, simpler implementation)
          - AddApp(), RemoveApp()
          - No keepalive needed (apps are stable)
          - No PINs (simpler security model)
          - Simpler, more reliable
```

**Factory Function:**
```go
func NewWolfExecutor(...) Executor {
    mode := os.Getenv("WOLF_MODE")
    if mode == "" {
        mode = "apps" // Default to simpler, more stable apps model
    }

    switch mode {
    case "lobbies":
        return NewLobbyWolfExecutor(...)
    case "apps":
        return NewAppWolfExecutor(...)
    default:
        log.Fatal().Str("wolf_mode", mode).Msg("Invalid WOLF_MODE")
    }
}
```

**Benefits:**
- Test both approaches in production with simple env var change
- Gradual migration: run both in parallel, compare reliability
- Easy rollback: just flip env var
- No code deletion - keep lobbies code for reference
- Clean abstraction: both implement same Executor interface

## Implementation Steps

### Step 1: Wolf Repository Migration (~/pm/wolf)

```bash
cd /home/luke/pm/wolf

# Save current branch for reference
git branch wolf-ui-working-backup

# Switch to stable and pull latest
git fetch upstream
git checkout stable
git pull upstream stable

# Create new branch for our moonlight-web patches
git checkout -b stable-moonlight-web

# Cherry-pick critical commits in order
git cherry-pick 84d4c01  # Phase 5 HTTP support
git cherry-pick 57321eb  # Auto-pairing PIN
git cherry-pick c19557a  # Docker-only auto-pair
git cherry-pick 82eecca  # Memory debugging endpoint

# Resolve any conflicts, test build
docker build -t wolf:stable-moonlight-web .
```

### Step 2: Update Helix Docker Compose

```yaml
# docker-compose.dev.yaml
services:
  wolf:
    build:
      context: ../wolf
      dockerfile: Dockerfile
    image: wolf:stable-moonlight-web
    # ... rest unchanged
```

### Step 3: Revert Helix Code to App-Based Model

**Before (Lobbies):**
```go
// Create lobby (container starts immediately)
lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)
session := &ZedSession{
    WolfLobbyID: lobbyResp.LobbyID,
    WolfLobbyPIN: lobbyPINString,
}
go w.startKeepaliveSession(ctx, sessionID, lobbyID, pin)
```

**After (Apps):**
```go
// Add app to Wolf (container managed by Wolf's auto_start)
app := w.createSwayWolfApp(config)
err := w.wolfClient.AddApp(ctx, app)
session := &ZedSession{
    WolfAppID: wolfAppID,
}
// No keepalive needed - apps are simple and stable
```

### Step 4: Database Migration

Personal Dev Environments already store both `WolfAppID` and `WolfLobbyID`. Migration:
- Keep existing PDE records
- Use `WolfAppID` for app-based interaction
- Ignore `WolfLobbyID` and `WolfLobbyPIN` fields (backward compatible)
- No schema changes needed

### Step 5: Testing Checklist

- [ ] Wolf builds successfully on stable-moonlight-web branch
- [ ] Wolf container starts and serves API via Unix socket
- [ ] Moonlight-web can pair with Wolf (auto-pairing works)
- [ ] External agent sessions create Wolf apps
- [ ] Moonlight client can connect and stream
- [ ] Personal Dev Environments work
- [ ] Agent Sandboxes dashboard shows memory data
- [ ] Screenshot server works
- [ ] No lobby-related errors in logs

## Rollback Plan

If migration fails:
```bash
# Wolf
cd /home/luke/pm/wolf
git checkout wolf-ui-working

# Helix
cd /home/luke/pm/helix
git revert <migration-commits>
docker compose -f docker-compose.dev.yaml build wolf
docker compose -f docker-compose.dev.yaml down wolf && docker compose -f docker-compose.dev.yaml up -d wolf
```

## Benefits of Stable Branch

1. **Simpler architecture**: No lobbies, PINs, or keepalive sessions
2. **More reliable**: Stable branch is battle-tested
3. **Easier debugging**: Fewer layers of abstraction
4. **Wolf manages lifecycle**: `auto_start_containers=true` handles containers
5. **No pairing certificate issues**: More stable between restarts
6. **Moonlight-web still supported**: Via cherry-picked commits

## Timeline Estimate

- Wolf branch migration: 30 minutes
- Helix code reversion: 2-3 hours
- Testing and fixes: 2-3 hours
- **Total: 5-7 hours**

Much simpler than maintaining lobbies long-term.

## Known Wolf Stable Limitations

- No Wolf UI app (lobby switcher) - fine, we don't need it
- No multi-user lobbies - we'll implement session sharing differently if needed
- Single session per app - acceptable for our use case

## Decision Points

1. **Make it togglable?**
   - Yes - env var `WOLF_MODE=apps` (default) or `lobbies`
   - Allows A/B testing and gradual rollout

2. **Keep both executors?**
   - Initially yes - `AppWolfExecutor` and `LobbyWolfExecutor`
   - Remove LobbyWolfExecutor after stable period

3. **Database schema?**
   - No changes - backward compatible
   - Existing lobbies references ignored on apps mode

## Next Steps

1. Execute Wolf repository migration
2. Build and test Wolf stable-moonlight-web image
3. Update Helix docker-compose
4. Revert Helix code to app-based model
5. Test thoroughly
6. Monitor production for 1 week
7. Remove lobbies code if stable
