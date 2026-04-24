# Requirements: Remove VectorChord Host Port Exposure

## User Story

As a **system administrator**, I want the production docker-compose file to **not expose the VectorChord (PostgreSQL) container on the host network**, so that the database is only reachable by other containers on the internal Docker network and is not accessible from outside the host.

## Acceptance Criteria

- [ ] The `vectorchord-kodit` service in `docker-compose.yaml` does **not** have a `ports:` directive
- [ ] Internal container-to-container communication via `vectorchord-kodit:5432` continues to work (no change to `KODIT_DB_URL`)
- [ ] The development compose file (`docker-compose.dev.yaml`) is unaffected (its port mapping is already commented out)
