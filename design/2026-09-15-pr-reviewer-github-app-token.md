# PR reviewer GitHub credential — it is already the GitHub App

**Date:** 2026-09-15
**Task:** 003232 — "on meta Helix, I notice that PR reviewer uses a secret that holds the GH token… can't we just use the gh app?"

## Finding: the premise is outdated

The meta Helix PR review flow already authenticates via the org's GitHub App. Verified live on meta.helix.ml, org `helix`:

1. `b-pr-coordinator` ("PR Review Coordinator") has exactly one worker-secret binding:
   `GH_TOKEN` → `source_kind: connected_account`, `export_key: github_app/installation_token`,
   account = the org's `github_app` ServiceConnection `helixml-bot`
   (`https://github.com/apps/helixml-bot`, installed on the `helixml` GitHub org).
2. Dispatched `pr-review-*` spec tasks call `get_secret GH_TOKEN` through the DelegatedCaller
   (`api/pkg/server/mcp_backend_spectask.go:119`), which resolves against the coordinator's
   bindings (`api/pkg/server/helix_org.go:874-907`) → `MintInstallationCredential` mints a
   **fresh ~1h installation token per call**. Nothing is stored or injected at boot (boot-time
   `GH_TOKEN` injection was removed in spec 002092 / PR #2586).
3. Live proof: the Sep 13–15 reviews on `helixml/helix` PRs (e.g. #3222) are authored by
   `helixml-bot[bot]` — the App identity. A static PAT would post as a human account.
4. No stored PAT exists anywhere in the flow: `transport.github` on the org holds only
   `webhook_secret`; the coordinator's project has zero project secrets; no other worker in
   the org has any binding; trigger/processor configs carry no token.

So the answer to "can't we just use the gh app?" is: **it already does.** The `GH_TOKEN`
"secret" is a binding that mints a short-lived App installation token on every `get_secret`.

## Why it looked like a separate token secret (root causes fixed here)

- The coordinator prompt said "find a granted GitHub token whose name contains `github`" —
  a stale instruction (the binding is named `GH_TOKEN`; it never matches) that reads like a
  stored-PAT model. Fixed in the bot content.
- The GitHub trigger panel rendered "GitHub token and webhook signing secret — Organization
  Settings (transport.github)" (`api/pkg/org/domain/transport/github.go:216-220`), telling
  every reader a GH token secret must be stored. Relabelled to webhook-secret-first with the
  App as the outbound identity; `transport.github.token` documented as an ops override only
  (it still wins in `Transport.Token()` for deployments that pin a PAT deliberately).

## Changes

1. **meta Helix (durable config, no Go code):** `b-pr-coordinator` content PATCH + restart —
   the two auth passages and the fallback now name `GH_TOKEN` exactly and state it resolves
   to a fresh `helixml-bot[bot]` installation token minted per call (re-fetch after 401/403).
   Verified: content round-trip matches, bot back `running`.
2. **Repo one-liner:** `Secrets` label on the github transport descriptor
   (`api/pkg/org/domain/transport/github.go`) — "GitHub webhook signing secret (outbound
   GitHub calls use the org's GitHub App; the transport.github token field is an ops override)".

## Options assessed and rejected

- Expose the credential source in `list_secrets` descriptors — contradicts the deliberate
  design ("Values and backend source details are never returned").
- Drop `transport.github.token` / the override path — still a legitimate ops escape hatch;
  removing it is a behaviour change beyond this task's scope.

## Verification

- `go build ./pkg/server/ ./pkg/org/...` and `go test ./pkg/org/domain/transport/ -count=1` — pass.
- meta Helix: `GET /api/v1/orgs/helix/bots/b-pr-coordinator` content diff shows exactly the
  three intended passages changed; `helix org bots restart b-pr-coordinator --org helix` →
  bot `running` (activation `a-bfce3707`). The credential path itself was NOT changed —
  binding, minting, and review posting behaviour are untouched (reviews already post as
  `helixml-bot[bot]`).
