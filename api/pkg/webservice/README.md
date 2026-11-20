# Persistent web-service runner recovery

Web-service `/data` is persistent and runner-local. `HostDeviceID` records the
runner that owns that data. The pin is immutable after it is established. Deploy
and recovery refuse to use a missing or different runner, and a pin conflict is
reported as HTTP 409.

## Pre-release inventory

Run this read-only query against the control-plane PostgreSQL database before a
release that can recreate web-service sandboxes:

```sql
SELECT
  p.id AS project_id,
  p.name AS project_name,
  s.active_sandbox_id,
  s.host_device_id AS data_host,
  sb.host_device_id AS sandbox_host,
  sb.status AS sandbox_status,
  sb.deleted_at AS sandbox_deleted_at
FROM project_web_service_states s
JOIN projects p ON p.id = s.project_id AND p.deleted_at IS NULL
LEFT JOIN sandboxes sb ON sb.id = s.active_sandbox_id
WHERE s.enabled
  AND (
    NULLIF(s.host_device_id, '') IS NULL
    OR sb.id IS NULL
    OR sb.deleted_at IS NOT NULL
    OR NULLIF(sb.host_device_id, '') IS NULL
    OR s.host_device_id <> sb.host_device_id
  )
ORDER BY p.id;
```

An empty result means the recorded data runner and active sandbox runner agree
for enabled projects. A row requires operator review before deployment or
recovery. A healthy sandbox on another runner is not proof that its `/data`
contains the original data.

## Recovery

If the original data is still on the recorded runner, select or create a
persistent web-service sandbox for the same project on that runner. Verify that
it mounts the project's existing `/data`, then activate it with `POST
/api/v1/projects/{id}/web-service/active-sandbox` and redeploy. If the recorded
runner is unavailable, wait for or recover that runner; do not clear the pin or
repin the project just because another runner is online.

If the data was deliberately migrated, first take a complete, consistent
snapshot of `/data` on the source and target, including all database data and
logs, metadata and stored files. Use the database engine's supported online
backup API, or quiesce the service before copying the data and its WAL state.
Verify database integrity, foreign keys, file hashes, tenant scope and the
exact source and target runners. Keep the source snapshot and obtain a
reviewed operator change before reconciling the project's pin. The change must
be targeted to that project; never clear or rewrite all pins.

Before the final synchronization and pin change, fence writes on the source.
An online backup can reduce downtime, but it does not replace the final write
fence. Verify the target before allowing traffic. If the mismatch is not
explained by a reviewed migration, leave the pin unchanged and investigate the
original runner and its data.

The health monitor reports deferred recovery when a pin is missing or differs
from the active sandbox. That is intentional: refusal preserves the chance to
recover the original data instead of silently starting an empty service.
