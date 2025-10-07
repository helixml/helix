# Wolf UI Configuration Notes

## What is Wolf UI?

Wolf UI is a Godot-based graphical interface that serves as a **lobby launcher/switcher**.

**How it works:**
1. User launches "Wolf UI" app in Moonlight → Streams wolf-ui container
2. Inside wolf-ui, user sees list of active lobbies
3. User selects a lobby and enters PIN
4. Wolf-ui calls `/api/v1/lobbies/join` with `lobby_id` and `moonlight_session_id`
5. **Wolf dynamically switches streams** - same Moonlight session now receives video/audio/input from the lobby
6. User is now streaming the lobby content (e.g., Zed agent desktop) instead of wolf-ui

**Key insight:** The Moonlight session stays connected, but Wolf redirects which lobby's streams it receives. This allows seamless lobby switching without reconnecting.

## Current Status

Wolf UI app appears in Moonlight but doesn't work because:

1. **Socket mount issue**: Wolf UI container needs access to `/var/run/wolf/wolf.sock`
2. **Current config**: Mounts host path `/var/run/wolf/wolf.sock` but socket only exists inside Wolf container
3. **Our setup**: Uses Docker `wolf-socket` volume, not host bind mount
4. **Wolf limitation**: Lobby/app runner configs don't support Docker volume mounts, only host paths

## Solutions

### Option 1: Fix wolf-ui app mount (Recommended for future)
Update wolf-ui app in config.toml to use the wolf-socket volume:
- Requires updating Wolf to support Docker volume mounts (not just host paths)
- OR: Expose socket via host bind mount instead of Docker volume

### Option 2: Remove wolf-ui app from Moonlight profile (Current workaround)
Users don't need Wolf UI because:
- Helix frontend shows all sessions/PDEs
- Users can join lobbies by entering PIN when Moonlight prompts
- Wolf API provides all needed functionality

To remove:
```bash
docker compose -f docker-compose.dev.yaml exec api curl -s -X POST \
  --unix-socket /var/run/wolf/wolf.sock \
  -H "Content-Type: application/json" \
  -d '{"id":"<wolf-ui-app-id>"}' \
  http://localhost/api/v1/apps/remove
```

### Option 3: Run wolf-ui as sidecar container
Add wolf-ui as a separate service in docker-compose.dev.yaml:
```yaml
wolf-ui:
  image: ghcr.io/games-on-whales/wolf-ui:main
  environment:
    - WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
  volumes:
    - wolf-socket:/var/run/wolf:rw
  network_mode: host
```

Then users could access wolf-ui via a dedicated Moonlight app or web interface.

## Recommendation

For now: **Ignore the Wolf UI app** - it's optional and not needed for Helix operation.

Users can:
- View sessions/PDEs in Helix frontend
- Connect via Moonlight by entering PIN when prompted
- No GUI launcher needed

Future: Consider Option 3 (sidecar) if rich lobby selection UI becomes important.
