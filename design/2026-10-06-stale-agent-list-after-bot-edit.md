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

Meanwhile the bot/app update paths never refresh that snapshot:

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

App-driven identity changes now adopt the app's live configuration; the
point-in-time snapshot is dropped when it would contradict it.

1. `agentSwitchOptions.adoptAppIdentity` — `switchAgentInPlaceForNextTurn`
   clears `Metadata.CodeAgentConfig` + `CodeAgentOverrides` when the switch is
   app-driven (the `switch-agent` endpoint, `switchAgentInPlace`, the
   reconcile path). The session-driven composer PATCH path deliberately does
   NOT set it: there the snapshot is the change being applied.
2. `reconcileSessionAgentWithApp` now also fires on config drift:
   `MaterializeCodeAgentConfig(app)` is compared against the stored snapshot
   (`sessionCodeAgentIdentityDiffers` — runtime, credential type, provider
   ref, model). A drifted snapshot reconciles exactly like a runtime drift:
   thread cleared, fork_seed seeded, next turn starts on the app's config.
3. `SyncAgentProfile` drops a drifted snapshot before the bot's next
   activation clears the ACP thread, so the restarted desktop resolves its
   config from the app.
4. Frontend: `useGetSessionExecutionConfig` polls every 15s so the composer's
   picker converges after a server-side reconciliation without a page reload
   (the desktop daemon already re-polls `/zed-config` every 30s).

Reasoning effort / goose recipes are intentionally NOT part of the drift
predicate: they are tuning that follows the app on the next config render, not
identity, and normalizing them (`""` vs `"none"`) would produce false drift.

## Verification

- Focused Go tests: `TestSwitchAgentInPlace_AdoptsAppIdentity`,
  `TestReconcileSessionAgentWithApp_ModelDriftReconciles`,
  `TestReconcileSessionAgentWithApp_MatchingConfigIsNoop`,
  `TestInProcSpawnerClient_SyncAgentProfileDropsDriftedCodeAgentSnapshot`,
  `TestInProcSpawnerClient_SyncAgentProfileKeepsMatchingCodeAgentSnapshot`.
- Full `go test ./pkg/server/` passes (includes all switch/phase/execution-config suites).
- `yarn build` (frontend) passes.
- Live inner-Helix check (API level): app-backed zed_external session with a
  stored snapshot (model A) + app edited to model B:
  - before: `GET /execution-config` → A (stale snapshot wins);
  - `POST /sessions/{id}/messages` → reconcile logged
    `config_drift=true runtime_drift=false`, snapshot cleared,
    `AgentSwitchedAt` set, thread pointer cleared, fork_seed marker written;
  - after: `GET /execution-config` → B (app's live model).
  Test session/app deleted afterwards; no containers leaked.

WARNING: not yet tested end-to-end with a LIVE connected desktop (no real Zed
turn was driven through the reconciled thread). The reconcile/switch lifecycle
itself is covered by the existing live-verified suites
(design/2026-07-13-org-bot-runtime-switch-stale-agent.md) and only the
snapshot-clearing behavior is new.
