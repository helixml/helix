# helix-org: add `mint_credential` MCP tool for Worker credential refresh

## Summary

A Worker's `GH_TOKEN` is minted as a GitHub App installation token
(~1h TTL) and pushed into the desktop env **once at container boot**;
Docker env vars are immutable so sessions that outlive the TTL silently
lose every `git`/`gh`/auth-`curl` call mid-task. This PR closes the gap
with a generic `mint_credential` MCP tool the agent can call (and re-call
on 401/403) to pull a fresh credential from the same per-provider
identity resolver that already drives the boot-time injection.

The tool is built provider-agnostic from day one — adding Slack (or any
other transport) is a new file in that transport's package plus one
registration line in `helix_org.go`, with **zero edits to the tool
itself**.

## Why a new MCP tool (recorded exception to "keep MCP surface small")

`CLAUDE.md` bans per-action MCP wrappers like `publish_to_blog` /
`fetch_url`. `mint_credential` is a different category: a generic
*primitive* that makes the shell tools usable on long-running sessions.
The exception is recorded inline in `CLAUDE.md`, in
`mint_credential.go`'s doc comment, and in
`design/tasks/002092_helix-org-mintcredential/design.md` §2.

## Changes

- **New domain interface** `org/domain/credential` — `Provider` +
  `Credential{Token, ExpiresAt, Usage}`. Providers live with their
  transports; the tool dispatches across a name-keyed registry wired in
  helix-org's bootstrap.
- **New MCP tool** `org/application/tools/mint_credential.go`. Args:
  `{provider}`. Returns `{token, expires_at, usage}`. `orgID` is read from
  `inv.Caller.OrganizationID()` — never from args, mirroring
  `create_stream`. A forged `org_id` arg is ignored (regression test).
- **GitHub `CredentialProvider`** in
  `org/infrastructure/transports/github/credential_provider.go` — thin
  wrapper over the existing identity resolver. The token in / token out
  path is unchanged; we just expose it through a second surface.
- **Resolver widening:** sibling `MintInstallationCredential` in
  `agent/skill/github/client.go` returns both the token and the
  ghinstallation-reported expiry; `OrgGitHubIdentity` and the `mintFn`
  seam grow an `ExpiresAt` field. `MintInstallationToken` stays as a
  thin wrapper for the one caller that only needs the string.
- **Wiring** in `server/helix_org.go` builds the provider registry
  reusing the same per-org identity resolver that used to feed the
  GitHub `SecretInjector`. The github `SecretInjector` itself is
  **removed** (the file is deleted) — with every Worker holding
  `mint_credential` in its baseline tool set, the boot-time `GH_TOKEN`
  env var was redundant and only taught the agent to expect a working
  credential that silently expired mid-session. The generic
  `secretInjectors` slice + spawner iteration stay (still useful for
  future non-credential secrets); the slice is empty today.
- **Role / prompt:** `MintCredentialName` added to `BaseReadTools` so
  every Role (owner included, plus every Worker hired now or in the
  future, plus all pre-existing Roles backfilled by `RoleReconciler` at
  API start) automatically gets the tool. `defaults_test.go`,
  `reconciler_test.go`, and `create_role_test.go` golden lists updated.
  `templates/owner_role.md` gains a "Long-running credentials" section
  telling the owner to put the mint → export → retry-on-401/403 guidance
  in any Worker Role whose Worker runs `gh`/`git`/auth `curl` — the tool
  is automatic but the prompt signal for *when* to reach for it is not.
- **Tests:**
  - 8 in `mint_credential_test.go`: happy path, unknown provider, missing
    provider arg, missing OrgID, **forged-`org_id` regression**, provider
    error propagation, zero-ExpiresAt JSON omission, description content.
  - 5 in `credential_provider_test.go`: Name, happy path, empty-token
    error, resolver-error propagation, nil-resolver guard.
  - 4 updated in `helix_org_github_app_test.go` for the new `mintFn` shape.
- **CLAUDE.md** addendum on the "Keep the MCP surface small" bullet
  records the exception.

## Out of scope

- Slack `CredentialProvider` — ships with the Slack transport when it
  lands.
- Git credential helper for transparent automatic refresh — only worth
  building if the prompt-based recovery path proves unreliable for `git`
  in practice.

## Test plan

- [x] `go build ./api/...` clean.
- [x] `go test ./api/pkg/org/...` clean (all packages green).
- [x] Inner-Helix API container Air-rebuilds with the new wiring and
      serves cleanly.
- [ ] **Manual:** install Helix GitHub App on a test org, hire a Worker
      with `mint_credential` in its Role, force expiry (suspend the App
      or overwrite `GH_TOKEN`), confirm the agent re-mints and retries.
      Steps in `design/tasks/002092_helix-org-mintcredential/VERIFICATION.md`;
      the resulting `E2E-RUN.md` closes acceptance criterion AC9. Cannot
      be synthesised in-session without real GitHub App credentials.
