# Implementation Tasks: Add `mint_credential` MCP Tool for Worker Credential Refresh

## Domain layer

- [~] Create `api/pkg/org/domain/credential/credential.go` with the `Provider` interface (`Name()`, `Mint(ctx, orgID) (Credential, error)`) and the `Credential` struct (`Token`, `ExpiresAt`, `Usage`).
- [~] Add a short package doc comment explaining that `Provider` implementations live with their transports under `infrastructure/transports/<name>/`.

## Resolver widening (GitHub identity)

- [ ] Widen `MintInstallationToken` (`api/pkg/agent/skill/github/client.go:127`) so the github transport can surface `expires_at` from the installation-token response — either return a small struct or expose a sibling function. Keep the existing string-returning function or its callers compiling (no churn outside the github transport).
- [ ] Widen `newOrgGitHubIdentityResolver` (`api/pkg/server/helix_org_github.go`) so its return shape includes `Token` and `ExpiresAt`. Keep the existing `gitHubTokenResolver` projection (returns just the token string) for the `SecretInjector` callers.
- [ ] Update unit tests around the resolver to assert the widened shape (no behaviour change for `Token`; new `ExpiresAt` assertion).

## GitHub `CredentialProvider`

- [ ] Add `api/pkg/org/infrastructure/transports/github/credential_provider.go` implementing `credential.Provider`. Constructor accepts the widened identity resolver. `Mint` returns `{Token, ExpiresAt, Usage: "export GH_TOKEN=<token>"}` and returns a descriptive error when the resolver returns an empty token.
- [ ] Add `credential_provider_test.go` covering the happy path and the empty-token error path. Use the existing test fixtures in `secret_injector_test.go` as a model.

## `mint_credential` MCP tool

- [ ] Add `MintCredential` to `Deps` (in `api/pkg/org/application/tools/builtins.go`): new field `CredentialProviders map[string]credential.Provider`. Default to an empty map in `DefaultDeps` so existing tests keep compiling.
- [ ] Add `api/pkg/org/application/tools/mint_credential.go` implementing `tool.Tool` (Name / Description / InputSchema / Invoke). Description is the exact text from `design.md` §3.3 (load-bearing — drives the mint→export→retry recovery loop). Schema has a single required `provider` string field; no `org_id` field exists in the schema.
- [ ] Read `orgID` from `inv.Caller.OrganizationID()` only — never from args. Return `mint_credential: caller has no OrgID` if empty (mirrors `create_stream`).
- [ ] Dispatch to the named provider; on unknown provider, return an error listing the registered provider names.
- [ ] Register `&MintCredential{deps: deps, providers: deps.CredentialProviders}` in `RegisterBuiltins`.
- [ ] Add `mint_credential_test.go` covering: happy path with a fake provider; unknown provider returns a listing error; missing OrgID returns the canonical error; a forged `org_id` field in raw args is ignored and the caller's OrgID is used; provider errors propagate with the provider name in the wrap.

## Wiring

- [ ] In `api/pkg/server/helix_org.go` near the existing `secretInjectors` block (`helix_org.go:283`), build `credentialProviders := map[string]credential.Provider{"github": githubtransport.NewCredentialProvider(identityResolver)}` and pass it through to `tools.RegisterBuiltins` via `deps.CredentialProviders`.
- [ ] Confirm `tools.DefaultDeps` and any other callsites of `RegisterBuiltins` are updated.

## Role / prompt integration

- [ ] Add `mint_credential` to `Role.Tools` for the Role(s) used by inner-Helix Worker sessions (identify exact role(s) during implementation; likely the default Worker role and any role that has `gh`/`git` in its environment).
- [ ] Append a short paragraph to the corresponding Role prompt body matching the tool description's final paragraph (mint → export → on 401/403, re-mint, re-export, retry). Prompt edit only — no Go logic.

## Verification

- [ ] `go build ./api/pkg/org/... ./api/pkg/server/...` is clean.
- [ ] All new and modified unit tests pass: `CGO_ENABLED=1 go test ./api/pkg/org/...`.
- [ ] End-to-end in the inner Helix at `http://localhost:8080`: hire a Worker with `mint_credential` + shell tools; force `GH_TOKEN` expiry; confirm the agent re-mints, re-exports, and the `git`/`gh` retry succeeds. Document the exact reproduction steps in a follow-up note in this task directory.

## Docs / changelog

- [ ] Add a short note to `CLAUDE.md` under "helix-org design philosophy" (or alongside it) recording the recorded exception: a generic *credential-minting primitive* is allowed under the "keep MCP surface small" rule; per-action wrappers (`publish_to_blog`, `fetch_url`) remain banned.
- [ ] In the `mint_credential.go` doc comment, link back to this design document and explicitly note the MCP-surface exception so future reviewers do not have to re-derive the rationale.

## Out-of-scope (do not implement in this task)

- [ ] Slack `CredentialProvider` — ships with the Slack transport when it lands.
- [ ] Git credential helper for transparent automatic refresh — only build if AC7/AC8 prompting proves unreliable in practice.
- [ ] Any change that re-issues `GH_TOKEN` into a running container's env (the immutability of Docker env is what motivated this design — we don't fight it).
