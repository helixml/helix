# Wolf UI Configuration Notes

## What is Wolf UI?

Wolf UI is a Godot-based graphical interface for managing Wolf lobbies and profiles. It provides a nicer lobby selection experience than the default Moonlight app list.

## Current Status

Wolf UI app appears in the Moonlight client but doesn't work because:

1. **Socket mount issue**: Wolf UI needs access to `/var/run/wolf/wolf.sock`
2. **Current config**: Mounts host path `/var/run/wolf/wolf.sock` but socket only exists inside Wolf container
3. **Our setup**: Uses Docker `wolf-socket` volume, not host bind mount

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
