# Remove host port exposure from VectorChord in production compose

## Summary
The `vectorchord-kodit` service in `docker-compose.yaml` exposed PostgreSQL port 5432 on a random host port via `ports: - "5432"`. This is unnecessary since the only consumer (`helix` service) connects via Docker internal DNS (`vectorchord-kodit:5432`). Removing the port mapping eliminates an unneeded attack surface.

## Changes
- Removed `ports:` block from `vectorchord-kodit` service in `docker-compose.yaml`
