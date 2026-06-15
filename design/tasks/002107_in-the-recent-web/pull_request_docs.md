# Document web service hosting persistence and runner pinning

## Summary
Adds a "Web Service Hosting" page explaining how a project web service's state
persists, since this is a common source of confusion (does my database survive
a deploy / reboot?).

## Changes
- New page `content/helix/using-helix/web-service-hosting/index.md`:
  - Write durable state to `/data` (`$HELIX_WEB_SERVICE_DATA_DIR`), with a
    worked `.helix/startup.sh` example (Postgres + SQLite).
  - Deploys are in-place / single-writer → brief restart-window of downtime;
    automatic rollback on a failed deploy.
  - Service is pinned to a runner: survives container/sandbox restart and runner
    reboot; fails loudly rather than relocating.
  - Warning: single-runner local disk — permanent runner loss loses the data;
    take your own backups. External Kubernetes for blue/green / scaling.
