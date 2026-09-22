# Sandbox desktop image metadata recovery

The sandbox image contains an immutable copy of desktop `.version` and `.ref`
files in `/opt/images-seed`. On boot, missing runtime metadata in `/opt/images`
is restored from that seed. If the seed is unavailable, startup may adopt one
unambiguous non-`latest` tag from the nested Docker store or `registry:5000`.
It still fails closed when no exact version can be determined or loaded.

## Production deployment check

Official production deployments use the metadata bundled in the sandbox image.
They must not bind-mount a source checkout onto `/opt/images`.

```bash
docker inspect <sandbox-container> \
  --format '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{println}}{{end}}'
```

If `/opt/images` is listed and its source is a Git worktree, remove that mount
from the deployment definition and recreate the sandbox container. The
development compose file intentionally keeps the bind mount for image-build hot
reload; startup recovery protects it from an empty directory but does not make
that development mount appropriate for production.

## Recovery drill

Run this only on a lab host with no active sessions. The drill moves metadata to
a backup rather than deleting it.

```bash
mkdir -p /tmp/helix-image-metadata-backup
mv sandbox-images/helix-*.version sandbox-images/helix-*.ref \
  /tmp/helix-image-metadata-backup/ 2>/dev/null || true
docker restart <sandbox-container>
docker logs <sandbox-container> 2>&1 | grep -E \
  'Desktop image metadata|Restored desktop image metadata|helix-ubuntu.*available'
docker inspect <sandbox-container> --format '{{.State.Health.Status}}'
docker exec <sandbox-container> test -s /opt/images/helix-ubuntu.version
```

Expected result: the next boot reports the `/opt/images` mount source, restores
the pointer from `/opt/images-seed` (or identifies the store/registry fallback
used), loads `helix-ubuntu:<version>`, and becomes healthy within that restart.
The `/data` workspace volume is not modified.

Restore the lab checkout after the drill if it is still used for development:

```bash
mv /tmp/helix-image-metadata-backup/helix-* sandbox-images/ 2>/dev/null || true
```

If startup still fails, the fatal diagnostic lists the runtime metadata source,
seed presence, nested Docker lookup, and local-registry endpoint. A failure after
all four checks means the desktop image itself is unavailable; restore or publish
the expected tag before restarting rather than inventing a version pointer.
