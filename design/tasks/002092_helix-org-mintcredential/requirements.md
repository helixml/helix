# Requirements: Add `mint_credential` MCP Tool for Worker Credential Refresh

## Background

GitHub credentials are minted as GitHub App installation tokens (~1h TTL) and
injected into a Worker's desktop **once**, at container boot, as the `GH_TOKEN`
env var. There is no refresh mechanism, so any session that outlives ~1h ends
up with an expired token and every `git`/`gh`/authenticated `curl` call fails
mid-task. Re-activation does not help an already-running desktop because the
secret is only read into the env at boot.

The bug is the **lack of refresh**, not the TTL. Slack will hit the same shape
of problem as soon as its transport lands.

## User Stories

### Story 1: Worker self-refreshes a GitHub credential mid-session

**As** a long-running AI Worker performing `git`/`gh` operations,
**I want** to call `mint_credential` to obtain a fresh short-lived token for my
org's GitHub identity,
**so that** I can recover from auth failures without my session being killed or
needing operator intervention.

### Story 2: Worker recovers from a 401/403 transparently

**As** a Worker whose existing `GH_TOKEN` has just expired mid-task,
**I want** my Role prompt to tell me that 401/403 errors mean *"call
`mint_credential` again, re-export the result, retry"*,
**so that** silent mid-session token expiry becomes a self-healing retry rather
than a task-killing failure.

### Story 3: Operator adds a new provider without editing the core

**As** a developer adding a Slack (or other) transport,
**I want** to implement one small `CredentialProvider` interface alongside my
transport code and register it next to the existing `SecretInjector`
registration,
**so that** the agent gains the ability to mint Slack credentials with **zero
edits** to the `mint_credential` tool itself.

### Story 4: Tenant isolation is preserved

**As** the platform operator,
**I want** the agent to be able to select only the *provider*, never the *org*,
**so that** a Worker cannot mint another org's credential even if it tries.

## Acceptance Criteria

### AC1: Tool surface

- A new MCP tool `mint_credential` is registered in `application/tools/`
  builtins and discoverable on Workers whose Role lists it.
- Args (JSON Schema, strict):
  - `provider` (string, required): one of the registered provider names
    (initially `"github"`).
- Returns a JSON object with:
  - `token` (string): the minted credential.
  - `expires_at` (RFC3339 timestamp): when the credential expires.
  - `usage` (string): a hint, e.g. `"Authorization: Bearer <token>"` or
    `"export GH_TOKEN=<token>"`.

### AC2: Tenant scoping (from caller, never the args)

- `orgID` is read off `inv.Caller.OrganizationID()`, identical to
  `create_stream` (`tools/create_stream.go:103`).
- The schema does **not** expose `org_id`. Even if an agent supplies one in
  the raw JSON, it is ignored. (Verified by a regression test.)

### AC3: Authorization model

- Having `mint_credential` in a Role *is* the authorization. The tool does not
  re-check the caller's scope — consistent with every other built-in tool
  (`tool.go:71`, "tool appearing in the caller's Role.Tools is the entire
  authorisation").

### AC4: Unknown provider error

- Calling with a `provider` that is not registered returns a clear, agent-
  readable error (`unknown provider %q; available: github, ...`). The tool
  does not panic.

### AC5: GitHub provider works end-to-end

- The GitHub `CredentialProvider` is implemented as a thin wrapper over the
  existing `newOrgGitHubIdentityResolver` /
  `github.MintInstallationToken` pipeline (the same code path the
  `SecretInjector` already uses).
- A Worker in an org that has a Helix GitHub App installation can call
  `mint_credential {provider: "github"}` and receive a fresh installation
  token. `expires_at` is approximately one hour in the future.
- A Worker in an org that has **no** GitHub identity wired up receives a
  descriptive error rather than an empty token.

### AC6: Provider registry is open-set

- Providers are registered through a small `CredentialProvider` interface
  defined in `org/domain/`. The `mint_credential` tool holds a
  `map[string]CredentialProvider` and dispatches by name.
- Adding a provider requires:
  - one new file in the relevant transport's `infrastructure/transports/<name>/`
    directory, implementing the interface, and
  - one registration line near the existing transport-wiring block in
    `helix_org.go`.
- **No edits to the tool itself** are needed to add a provider. This is
  asserted by a test that registers a fake provider and confirms the tool
  surfaces it.

### AC7: Tool description drives social-enforcement refresh

- The tool's `Description()` reads to the effect of §3.1 in the design doc:
  - explains it returns a short-lived (~1h) credential;
  - tells the agent to call it and `export` the result **immediately before**
    `git`/`gh`/authenticated `curl` work;
  - explicitly says *"if a command fails with 401/403, your token has
    expired — call `mint_credential` again, re-export, and retry"*.
- The instruction is **not** "mint at activation" — the `SecretInjector`
  already covers boot; the prompt is for the long-running-session gap.

### AC8: Role / prompt integration

- The relevant Role(s) used by Workers in the inner-Helix end-to-end test have
  `mint_credential` added to `Role.Tools`.
- The Role prompt body includes the §3.1 guidance (mint → export → retry on
  401/403). Prompt edits are pure text — no Go logic added to fulfil this.

### AC9: End-to-end recovery test

- In the inner Helix, a long-running Worker session has its `GH_TOKEN`
  forcibly expired (e.g. overwritten with a deliberately bad value or the
  installation token deleted/revoked).
- The next `git push` fails with a 401/403, the agent calls
  `mint_credential`, re-exports `GH_TOKEN`, and the retry succeeds.
- The test is reproducible and documented in the task notes.

## Out of Scope

- **Slack provider.** The interface and tool are built provider-agnostic
  with Slack in mind, but the Slack `CredentialProvider` ships with the Slack
  transport, not this task.
- **Git credential helper** (transparent automatic refresh for `git`). Only
  worth building if the prompt-based recovery in AC7/AC8 proves unreliable
  in practice for `git` specifically.
- **Long-lived PAT for a shared bot account.** Explicitly rejected in the
  design doc §2.1 — breaks per-org tenant isolation and increases the blast
  radius of a leak.
- **Re-issuing the boot-time `GH_TOKEN` env var** on a running container. The
  immutability of a Docker env var for the container's life is what motivated
  this whole change; we are *not* fighting it, we are giving the agent a
  pull-on-demand path.
