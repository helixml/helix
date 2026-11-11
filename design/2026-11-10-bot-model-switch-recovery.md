# Bot model-switch recovery

Date: 2026-11-10
Status: implemented and locally validated; production rollout pending

## Incident

Autonomous bots could remain stuck after their provider or model was changed.
The UI showed a generic activation failure, while later turns could continue
using the old model or remain blocked behind a failed activation.

## Root causes

- Zed's process-global OpenCode connection cache was keyed by project and
  agent ID. Since every OpenCode configuration used the same agent ID, a
  cached process and its environment survived a model switch.
- The settings-sync daemon acknowledged an agent configuration before Zed had
  observed the rewritten settings. A watcher/callback race could therefore
  deliver a new thread to a process with the old configuration. A failed sync
  could also send the applied callback.
- Provider model discovery was not authoritative. A configured model missing
  from a successful upstream catalogue could be fabricated back into the
  enabled list. On a cold cache, validation checked provider existence without
  checking selected model availability, even when upstream discovery worked.
- Failed activation prompts were marked sent, while stale provider-recovery
  metadata could survive a provider switch. The original failed activation
  could remain stuck, and its old error could be misattributed to the current
  provider.

## Fix and guards

- Zed now includes the complete custom command/environment or registry
  environment in connection identity. A changed launch identity creates a new
  process; unchanged settings still deduplicate. Generation checks prevent an
  older failed connection from evicting a newer one. UI entries use the same
  identity and invalidate on change. New thread creation reloads settings
  from disk before connecting, closing the watcher/callback race, while
  existing active threads retain their connections.
- The settings-sync daemon only acknowledges a successfully applied agent
  configuration. A failed sync leaves the API restart fallback responsible
  for recovery.
- Successful `/v1/models` discovery is authoritative: the effective catalogue
  intersects the configured whitelist and does not synthesize missing IDs.
  Explicit lists remain valid for endpoints that do not expose discovery when
  no prior successful catalogue exists. A prior successful catalogue is kept
  as degraded cache data rather than being replaced by a fabricated list.
  Saving app or session configuration validates the selected provider model
  using fresh-enough discovery, including on a cold cache.
- Recovery requeues only the exact latest activation prompt when its latest
  interaction is still an error and the failure has explicit provider-failure
  evidence or matching recovery metadata. Deleted, cancelled, completed, and
  newer work is left alone. The original error interaction remains unchanged
  even after a successful retry; the separate activation outcome row is
  settled by the existing durable retry after real completion.
  The latest-activation lookup includes deleted entries and uses the literal
  activation prefix, so a newest cancelled activation blocks an older failure.
  Provider-recovery metadata is cleared with compare-and-swap guards,
  preserving a concurrently newer recovery. Global provider aliases are
  matched only for the corresponding synthetic global endpoint; raw and ID
  forms remain distinct for other providers.
- Self-review found that the agent-config-applied callback could read a session
  snapshot and later save stale recovery metadata over a concurrent recovery
  clear. `RecordSessionAgentConfigApplied` now updates only the applied and
  handoff timestamps, leaving recovery metadata untouched.

## Validation

Reported local validation after the final edits:

- Zed `agent_servers`: 37 passed.
- Zed `external_websocket_sync`: 82 passed, 5 ignored.
- Affected Zed crate checks passed.
- Full settings-sync-daemon tests passed.
- Helix stale-snapshot handler regression, MemoryStore tests, and the isolated
  PostgreSQL recovery suite (2 tests) passed.
- Full affected Helix server, external-agent, MemoryStore, and
  settings-sync-daemon tests passed:
  `go test ./api/pkg/server ./api/pkg/external-agent ./api/pkg/store/memorystore ./api/cmd/settings-sync-daemon -count=1`.
- The focused PostgreSQL suite passed after the final CAS split. The server
  mock race unit passed separately.

Zed is committed locally at
`73027feda055e6ca8bf79b5e51a5bee664bcb6da`, and Helix
`sandbox-versions.txt` pins that exact SHA. No production deployment or live
bot configuration changed. Live connected-bot verification was not run.
