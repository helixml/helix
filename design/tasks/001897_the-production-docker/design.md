# Design: Remove VectorChord Host Port Exposure

## Current State

In `docker-compose.yaml` (production), the `vectorchord-kodit` service has:

```yaml
ports:
  - "5432"
```

This tells Docker to map container port 5432 to a **random host port**, making the PostgreSQL instance reachable from outside the Docker network. This is a security concern — the database should only be accessible to other services on the internal `helix_default` network.

The development file (`docker-compose.dev.yaml`) already has its port mapping commented out.

## Change

Remove the `ports:` block entirely from the `vectorchord-kodit` service in `docker-compose.yaml`.

No other changes are needed:
- The `KODIT_DB_URL` environment variable in the `helix` service already uses Docker internal DNS (`vectorchord-kodit:5432`), which works without host port mapping.
- No other service or external component references this port via the host.
- The Helm chart (`charts/helix-controlplane`) uses its own service definitions and is unaffected.

## Codebase Notes

- **Production compose:** `docker-compose.yaml`, lines 124–125
- **Dev compose:** `docker-compose.dev.yaml` — port already commented out (lines 201–202)
- **Internal DB URL:** `postgresql://postgres:...@vectorchord-kodit:5432/kodit` — uses Docker DNS, unaffected by this change
