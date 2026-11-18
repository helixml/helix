# Session sandbox resource history

The Usage tab of a spec task or bot session shows how its desktop container used CPU, memory, I/O and disk, so sandboxes can be sized from evidence. The same data is at `GET /api/v1/sessions/{id}/resources?hours=1..720` and `helix session resources [session-id]`; inside a sandbox the CLI defaults to `$HELIX_SESSION_ID`, so an agent can check its own sandbox.

## Collection

The sandbox heartbeat, already walking every `helix.session_id` container each 30 seconds for OOM kills, reads the container's cgroup v2 counters: CPU usage, quota and throttling, memory current/peak/limit, anon and file memory, `memory.high`, `memory.max` and OOM events, I/O bytes and operations, CPU/memory/I/O pressure stall totals, and process count. Counters are cumulative; the API derives rates from consecutive samples and skips a pair whose counters went backwards (the container restarted).

- A desktop's init process lives in a nested cgroup (`…/<id>/desktop/display`). Counters, including the OOM kill count, are read from the ancestor cgroup named after the container, which covers the whole container.
- On stacked block devices (device mapper over a partition) the kernel charges reads at every layer and writes only at the top. Only devices with no holders, on themselves or their partitions, are counted.
- Each distinct filesystem behind the container's bind and volume mounts is reported once (grouped by fsid). A filesystem mounted at a path naming the session — its ZFS volume or quota disk — is the session's own; anything else is a shared host volume, which shows headroom but not per-session use. Per-session use on shared volumes would need a directory walk, too costly to run every 30 seconds on hosts that are often I/O bound.

## Storage

The API stamps samples on receipt and keeps only those from the sandbox the session is placed on (`sessions.sandbox_id`), so a runner cannot write another sandbox's history. Samples are buffered and written every five minutes as an immutable Parquet chunk under `<filestore>/.session-resources/<day>/<session>/`. When a session-day reaches 24 chunks they are merged into one. Readers merge files with the unflushed buffer and drop duplicate timestamps, which also covers a merge interrupted between writing its output and removing its inputs. Days older than 30 are removed. File names carry the API node (`HELIX_HOST_HISTORY_NODE`), so API hosts sharing a filestore never write the same file; a node's unflushed samples are visible only to that node for up to five minutes.

## Reading

The response has a summary (CPU p50/p95/max against the limit, maximum and peak memory against the limit, share of time throttled and stalled on CPU, memory and I/O, I/O totals, `memory.high` hits, OOM kills), up to 360 chart points (rates averaged over time, gauges at their maximum, events summed) and the latest filesystems.
