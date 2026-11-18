# Session sandbox resource history

The Usage tab of a spec task or bot session shows how its desktop container used CPU, memory, I/O and disk, so sandboxes can be sized from evidence. The same data is at `GET /api/v1/sessions/{id}/resources?hours=1..720` and `helix session resources [session-id]`; inside a sandbox the CLI defaults to `$HELIX_SESSION_ID`, so an agent can check its own sandbox.

## Collection

The sandbox heartbeat, already walking every `helix.session_id` container each 30 seconds for OOM kills, reads the container's cgroup v2 counters: CPU usage, quota and throttling, memory current/peak/limit, anon and file memory, `memory.high`, `memory.max` and OOM events, I/O bytes and operations, CPU/memory/I/O pressure stall totals, and process count. Counters are cumulative; the API derives rates from consecutive samples and skips a pair whose counters went backwards (the container restarted).

- A desktop's init process lives in a nested cgroup (`…/<id>/desktop/display`). Counters, including the OOM kill count, are read from the ancestor cgroup named after the container, which covers the whole container.
- On stacked block devices (device mapper over a partition) the kernel charges reads at every layer and writes only at the top. Only devices with no holders, on themselves or their partitions, are counted.
- Each distinct filesystem behind the container's bind and volume mounts is reported once (grouped by fsid). A filesystem mounted at a path naming the session — its ZFS volume or quota disk — is the session's own; anything else is a shared host volume, which shows headroom but not per-session use. Per-session use on shared volumes would need a directory walk, too costly to run every 30 seconds on hosts that are often I/O bound.

## Storage

The API stamps samples on receipt and keeps only those from the sandbox the session is placed on (`sessions.sandbox_id`), so a runner cannot write another sandbox's history. Samples are buffered and written every five minutes as an immutable Parquet chunk under `<filestore>/.session-resources/<day>/<session>/`. When a session-day reaches 24 chunks they are merged into one. Readers merge files with the unflushed buffer and drop duplicate timestamps, which also covers a merge interrupted between writing its output and removing its inputs. Days older than 30 are removed. File names carry the API node (`HELIX_HOST_HISTORY_NODE`), so API hosts sharing a filestore never write the same file; a node's unflushed samples are visible only to that node for up to five minutes.

## Lifecycle

History follows the task, not the desktop. Archiving a task (which stops its desktop) keeps it, subject to the 30-day retention. Deleting a task removes the history of every session it ran — the planning session even when its row is already gone, and each session listed under the task. Deleting a project does the same for all of its tasks, then for the project's own sessions (exploratory session, project chats), before the project is soft-deleted. A step that fails fails the delete so it can be retried.

Deletion leaves a tombstone per session in `.deleted/`, kept for the retention period. Flushes and new samples skip tombstoned sessions on every API node, so samples buffered on another node, or reported by a desktop still running after its task was deleted, cannot recreate the history. Ingestion also refuses a session whose spec task or project no longer exists, which covers a desktop outliving its tombstone.

## Reading

The response has a summary (CPU p50/p95/max against the limit, maximum and peak memory against the limit, share of time throttled and stalled on CPU, memory and I/O, I/O totals, `memory.high` hits, OOM kills), up to 360 chart points (rates averaged over time, gauges at their maximum, events summed) and the latest filesystems.
