# Stale agent list after a bot runtime/model edit

Task: helix-specs 003600 ("Address stale agent list"). Reported: "I've updated
the bot to use opencode // glm 5.3 but the agent list still shows what the
agent started with."

## Symptom

An org bot session shows the harness + model the session STARTED with in every
agent surface, even after the bot was updated:

- the composer's agent picker (`CodeAgentConfigPicker`) in the session view,
- the desktop's coding agent (the settings-sync daemon's `/zed-config` source
  of truth), so new threads keep spawning on the old agent/model,
- `GET /sessions/{id}/execution-config`.

The 2026-07-13 fix (`design/2026-07-13-org-bot-runtime-switch-stale-agent.md`)
added `reconcileSessionAgentWithApp`, which repairs the session's *runtime*
binding before the next user turn. It did not cover the *model* half, and it
left a stale session-level `Metadata.CodeAgentConfig` snapshot in place.

## Root cause

A session-level coding-identity snapshot (`Session.Metadata.CodeAgentConfig`,
written by the session composer's `PATCH /sessions/{id}/execution-config`)
**overrides the app in every consumer**:

- `sessionExecutionConfigSurface` (session_execution_config_handlers.go):
  a non-nil snapshot is returned instead of resolving the app.
- `getZedConfig` (zed_config_handlers.go): the snapshot is applied over the app
  before the desktop's Zed settings are generated.
- `getAgentNameForSession`: the snapshot app wins when picking the agent for a
  new thread.

The snapshot is not always staleness — a composer PATCH on an App-backed
session deliberately stores a deviation, and the deviation is the feature. It
becomes stale when the app changes AFTER it was written; nothing told the
session apart:

- `reconcileSessionAgentWithApp` compared only the flat runtime, so a
  model-only bot edit (runtime unchanged) was invisible to it.
- `SyncAgentProfile` (bot activation / Apply-config) synced runtime, agent
  name and `ModelName` from the app but left the snapshot untouched — after a
  container restart `/zed-config` still applied the old snapshot.
- `switchAgentInPlaceForNextTurn` repointed the runtime but left any snapshot,
  so an app-driven switch left contradictory state behind.

Verified live (inner Helix): app edited to model B while the session kept a
snapshot with model A → `GET /sessions/{id}/execution-config` reported A
("what the agent started with"); the reconcile log showed the drift was only
visible once config identity was compared.

## Fix

App-driven identity changes now adopt the app's live configuration, and a
point-in-time snapshot yields to the app only when the app actually changed
since the snapshot was written.

1. `agentSwitchOptions.adoptAppIdentity` — `switchAgentInPlaceForNextTurn`
   clears `Metadata.CodeAgentConfig` + `CodeAgentOverrides` when the switch is
   app-driven (the `switch-agent` endpoint, `switchAgentInPlace`, the
   reconcile path). The session-driven composer PATCH path deliberately does
   NOT set it: there the snapshot is the change being applied.
2. `reconcileSessionAgentWithApp` now also fires on config drift:
   `MaterializeCodeAgentConfig(app)` is compared against the stored snapshot
   (`sessionCodeAgentIdentityDiffers` — runtime, credential type, provider
   ref, model). Drifted snapshots reconcile exactly like runtime drifts:
   thread cleared, fork_seed seeded, next turn starts on the app's config.
3. Drift is gated on WHEN the snapshot was written
   (`sessionCodeAgentSnapshotStale`): a snapshot written AFTER the app's last
   CODING-IDENTITY change is a deliberate session-level deviation from the
   composer PATCH and must survive reconciliation — differing from the app
   alone is not staleness. The app-side clock is a dedicated
   `App.CodeAgentConfigAt`, NOT `app.Updated`: the latter moves on every
   app-row write (tool/MCP edits via `UpdateAppConfig`, skill enable, avatar
   upload, legacy provider-ref heal), and any of those would wrongly expire a
   deliberate deviation and fork a new thread. The identity clock moves only
   when the assistant's runtime/credential/provider/model/effort actually
   changes (`stampAppCodeAgentConfigAt`, wired into `updateAgent`,
   `applyProject`, the org-MCP `UpdateAppConfig` and `ApplyAgentDefaults`).
   Prompt edits deliberately do NOT move it — they arm the restart-required
   banner but are not identity changes. Snapshots predating the
   `CodeAgentConfigAt` field (zero time) are treated as stale when they
   contradict the app, so pre-existing bot sessions get the fix without
   migration; a zero APP timestamp (no recorded identity edit) never expires a
   snapshot on its own.
4. `applySessionCodeAgentExecutionConfig` records `CodeAgentConfigAt` whenever
   it writes the snapshot (rolled back with the rest on failure).
5. `SyncAgentProfile` drops a drifted snapshot before the bot's next
   activation clears the ACP thread — under the same timing gate, so a
   session-level deviation chosen after the bot's last coding-identity edit
   survives re-activation.
6. Frontend: `useGetSessionExecutionConfig` polls every 15s so the composer's
   picker converges after a server-side reconciliation without a page reload
   (the desktop daemon already re-polls `/zed-config` every 30s).

Reasoning effort and goose recipes are intentionally NOT part of the drift
predicate: they are tuning that follows the app on the next config render, not
identity, and normalizing them (`""` vs `"none"`) would produce false drift.

## Verification

- Focused Go tests: `TestSwitchAgentInPlace_AdoptsAppIdentity`,
  `TestReconcileSessionAgentWithApp_ModelDriftReconciles`,
  `TestReconcileSessionAgentWithApp_ComposerDeviationKeptWhenAppUnchanged`,
  `TestReconcileSessionAgentWithApp_MatchingConfigIsNoop`,
  `TestUpdateAppPromptEditDoesNotMoveCodeAgentConfigAt`,
  `TestUpdateAppModelEditMovesCodeAgentConfigAt`,
  `TestInProcSpawnerClient_SyncAgentProfileDropsDriftedCodeAgentSnapshot`,
  `TestInProcSpawnerClient_SyncAgentProfileKeepsDeviationNewerThanApp`,
  `TestInProcSpawnerClient_SyncAgentProfileKeepsMatchingCodeAgentSnapshot`.
- Full `go test ./pkg/server/` passes (several consecutive green runs; one run
  aborted with a timer-runtime panic and no failing test — not reproducible
  since, presumed an unrelated timing flake).
- `yarn build` (frontend) passes.
- Live inner-Helix checks (API level), one coding-agent app edited
  `glm-5.3-flash → glm-5.3` through the real `PUT /agents/{id}` (which stamped
  `App.CodeAgentConfigAt`):
  - Stale case — session snapshot (`glm-5.3-flash`) written 1s BEFORE the app
    edit: the next message logged `config_drift=true runtime_drift=false`
    (`app_identity_updated_at` newer than `snapshot_at`), cleared the
    snapshot, set `AgentSwitchedAt`, cleared the thread pointer, and
    `GET /execution-config` then reported `glm-5.3`.
  - Deviation case — session snapshot (`glm-5.3-flash`) written AFTER the app
    edit: the next message kept the snapshot (`agent_switched_at` zero, no
    thread replacement) and `GET /execution-config` still reported
    `glm-5.3-flash`.
  Test sessions/app deleted afterwards; no containers leaked.

WARNING: not yet tested end-to-end with a LIVE connected desktop (no real Zed
turn was driven through the reconciled thread). The reconcile/switch lifecycle
itself is covered by the existing live-verified suites
(design/2026-07-13-org-bot-runtime-switch-stale-agent.md) and only the
snapshot-clearing behavior is new.
