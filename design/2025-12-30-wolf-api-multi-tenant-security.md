# Wolf API Multi-Tenant Security Roadmap

**Date:** 2025-12-30
**Status:** Roadmap item
**Priority:** Medium (security hardening)

## Current State

Wolf exposes a Unix socket API at `/var/run/wolf/wolf.sock` that is bind-mounted into desktop containers. This allows containers to communicate with Wolf for operations like:

- Setting PipeWire node ID for video capture (`POST /api/v1/lobbies/set-pipewire-node-id`)
- Joining/leaving lobbies
- Session management

### Security Issue

Currently, **any container with access to the Wolf socket has full API access**. A malicious or buggy container could:

1. Stop other users' lobbies (`POST /api/v1/lobbies/stop`)
2. Join other users' lobbies without authorization
3. Set PipeWire node IDs for other lobbies (hijack video streams)
4. View all active sessions and their metadata

This is a multi-tenant isolation violation.

## Proposed Solutions

### Option 1: Per-Lobby Sockets (Recommended)

Create a separate scoped socket for each lobby:

```
/var/run/wolf/lobby-{lobby_id}.sock
```

Each socket only exposes endpoints relevant to that lobby:
- `POST /set-pipewire-node-id` (no lobby_id param needed - implicit)
- `GET /status`

**Pros:**
- Natural isolation - container can only access its own socket
- Simple to implement
- No token management

**Cons:**
- More file descriptors
- Need to clean up sockets on lobby termination

### Option 2: Token-Based Scoping

Pass a unique secret token to each container via environment variable:

```
WOLF_API_TOKEN=<random-uuid>
```

Container must include token in API requests. Wolf validates token matches the lobby/session.

**Pros:**
- Single socket for all containers
- Familiar auth pattern

**Cons:**
- Token could leak (env vars visible in /proc)
- More complex validation logic

### Option 3: Separate Read-Only Socket

Create two sockets:
- `/var/run/wolf/wolf.sock` - Full API (only for Wolf UI, trusted containers)
- `/var/run/wolf/wolf-limited.sock` - Limited API for desktop containers

**Pros:**
- Clear separation of privilege levels

**Cons:**
- Doesn't solve per-tenant isolation

## Recommendation

**Option 1 (Per-Lobby Sockets)** is recommended because:
1. Provides natural isolation without token management
2. Simpler to reason about security boundaries
3. File system permissions can provide additional security layer
4. Easy to audit which container has access to which socket

## Implementation Notes

When implementing per-lobby sockets:

1. Create socket in `setup_lobbies_handlers` when lobby is created
2. Mount only that lobby's socket into the container
3. Remove socket when lobby is stopped
4. Update container env: `WOLF_SOCKET_PATH=/var/run/wolf/lobby-{id}.sock`

## Related

- PipeWire socket sharing implementation (commit `57389b1` on `feature/pipewire-screencast-gnome49`)
- Wolf API endpoint: `endpoint_LobbySetPipeWireNodeId` in `endpoints.cpp`
