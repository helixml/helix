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
   The same staleness rule covers composer deviations stored as
   `CodeAgentOverrides` (no snapshot): they gate on their own write time
   (`CodeAgentOverridesAt`, recorded by `persistSessionCodeAgentConfig`),
   because even an override pinning the app's current values blocks later app
   edits from reaching `/zed-config`. Overrides are cleared by the existing
   `adoptAppIdentity` path; `applySessionCodeAgentExecutionConfig` resets
   their timestamp when a snapshot replaces them (rollback restores both).
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
   Prompt edits and effort-only edits deliberately do NOT move it — they arm
   the restart-required banner (prompt) or count as tuning (effort) but are
   not identity changes; the identity tuple deliberately mirrors the fields
   the staleness predicate compares. `updateAgent` also carries the stored
   clock across the full-row save (API PUTs omit it — a full-row save would
   otherwise reset it to zero and disarm the gate). Snapshots predating the
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
  `TestReconcileSessionAgentWithApp_LegacySnapshotReconciles` (zero-time
  snapshot, the pre-existing-session branch),
  `TestReconcileSessionAgentWithApp_ComposerDeviationKeptWhenAppUnchanged`,
  `TestReconcileSessionAgentWithApp_StaleOverridesReconcile`,
  `TestReconcileSessionAgentWithApp_FreshOverridesKept`,
  `TestReconcileSessionAgentWithApp_MatchingConfigIsNoop`,
  `TestUpdateAppPromptEditDoesNotMoveCodeAgentConfigAt`,
  `TestUpdateAppModelEditMovesCodeAgentConfigAt`,
  `TestUpdateAppEffortEditDoesNotMoveCodeAgentConfigAt`,
  `TestUpdateAppPreservesCodeAgentConfigAtOnIdentityUnchangedSave`,
  `TestInProcSpawnerClient_SyncAgentProfileDropsDriftedCodeAgentSnapshot`,
  `TestInProcSpawnerClient_SyncAgentProfileDropsLegacySnapshot`,
  `TestInProcSpawnerClient_SyncAgentProfileKeepsDeviationNewerThanApp`,
  `TestInProcSpawnerClient_SyncAgentProfileDropsStaleOverrides`,
  `TestInProcSpawnerClient_SyncAgentProfileKeepsMatchingCodeAgentSnapshot`.
- Full `go test ./pkg/server/` passes (multiple consecutive green runs). One
  earlier run aborted with `panic: Fail in goroutine after
  TestWebSocketSyncSuite has completed`: a pre-existing race from the
  trailing-edge DB flush timer (main, 6ff541aa6, 2026-07-03) firing into a
  finished test's gomock controller under load. Not reproducible on demand,
  the suite passes in isolation, and CI (which runs the same suite) has been
  green throughout. A proper fix belongs to the test harness, not this PR.
- `yarn build` (frontend) passes.
- Live inner-Helix checks (API level), against real coding-agent apps edited
  through `PUT /agents/{id}`:
  - Stale snapshot case — snapshot written 1s BEFORE the app's model edit:
    the next message logged `deviation_stale=true runtime_drift=false`
    (`app_identity_updated_at` newer than `snapshot_at`), cleared the
    snapshot, set `AgentSwitchedAt`, cleared the thread pointer, and
    `GET /execution-config` then reported the app's model.
  - Fresh deviation case — snapshot written AFTER the app's last identity
    edit: the next message kept the snapshot (`agent_switched_at` zero, no
    thread replacement) and `GET /execution-config` still reported the
    deviating model.
  - Overrides cases — an effort-only app edit did NOT expire overrides
    (kept, no thread replacement); a model identity edit after the overrides
    fired `deviation_stale=true` (`overrides_at` older than
    `app_identity_updated_at`), cleared them, replaced the thread, and the
    picker followed the app's model.
  Test sessions/apps deleted afterwards; no containers leaked.

WARNING: not yet tested end-to-end with a LIVE connected desktop (no real Zed
turn was driven through the reconciled thread). The reconcile/switch lifecycle
itself is covered by the existing live-verified suites
(design/2026-07-13-org-bot-runtime-switch-stale-agent.md) and only the
snapshot-clearing behavior is new.
